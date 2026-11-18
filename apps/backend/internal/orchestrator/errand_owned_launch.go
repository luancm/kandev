package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/mcpmode"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type ownedErrandLaunchStore interface {
	GetErrand(context.Context, string) (*models.Errand, error)
	ReserveErrandExecutionGeneration(context.Context, string, uint64) (*models.ErrandExecutionDescriptor, error)
	BindErrandExecution(context.Context, string, uint64, string) (bool, error)
	GetErrandExecutionDescriptor(context.Context, string) (*models.ErrandExecutionDescriptor, error)
	GetErrandTurn(context.Context, string, string) (*models.Turn, error)
	CreateErrandTurn(context.Context, string, *models.Turn) error
	GetErrandMessage(context.Context, string, string) (*models.Message, error)
	CreateErrandMessage(context.Context, string, *models.Message) error
	GetAgentDeliverySubmissionMetadata(context.Context, string) (*models.AgentDeliverySubmission, error)
	ReadOwnedErrandFirstTerminal(context.Context, string) (models.ErrandTeardownTarget, models.DeliverySubmissionState, error)
}

type ownedErrandAdmission struct {
	service     *Service
	request     errands.LaunchRequest
	owner       runtimeapi.ExecutionOwner
	store       ownedErrandLaunchStore
	attachment  *ownedErrandWorkspaceAttachment
	predecessor *models.ErrandTeardownTarget
}

func (a ownedErrandAdmission) AdmitExecution(ctx context.Context, owner runtimeapi.ExecutionOwner) error {
	if owner != a.owner || a.service.isCancelInFlight(owner.ErrandID) {
		return models.ErrInvalidErrand
	}
	if err := a.checkLaunch(ctx); err != nil {
		return err
	}
	current, err := a.store.GetErrandExecutionDescriptor(ctx, owner.ErrandID)
	if err != nil {
		return err
	}
	if current.TaskID != owner.TaskID || current.Generation != owner.Generation {
		return models.ErrInvalidErrand
	}
	return a.verifyWorkspaceAttachment(ctx)
}

func (a ownedErrandAdmission) checkLaunch(ctx context.Context) error {
	if a.predecessor == nil {
		return a.service.validateErrandLaunchReservation(ctx, a.request, false)
	}
	row, err := a.store.GetErrand(ctx, a.request.ErrandID)
	if err != nil {
		return err
	}
	if row.TaskID != a.request.TaskID || row.ParentSessionID != a.request.ParentSessionID ||
		row.Lifecycle != models.ErrandLifecycleLaunched || row.TeardownIntent == nil ||
		row.TeardownIntent.Purpose != models.ErrandTeardownPurposeCompletion || row.TeardownIntent.State != models.ErrandTeardownIntentCompleted ||
		row.TeardownIntent.Target != *a.predecessor || a.predecessor.OwnerGeneration+1 != a.owner.Generation {
		return db.ErrErrandSettlementPending
	}
	return nil
}

func (a *ErrandAdapter) launchOwnedErrand(ctx context.Context, req errands.LaunchRequest) (errands.LaunchResult, error) {
	blocked := errands.LaunchResult{Disposition: errands.LaunchBlocked}
	guard, err := a.svc.lockCancelInFlightGuardWithContext(ctx, "errand-launch:"+req.ErrandID)
	if err != nil {
		return blocked, err
	}
	defer guard.release()
	if err := a.svc.validateErrandLaunchReservation(ctx, req, true); err != nil {
		return blocked, err
	}
	store, ok := a.svc.repo.(ownedErrandLaunchStore)
	if !ok {
		return blocked, errors.New("errand launch dependencies are unavailable")
	}
	if current, err := store.GetErrandExecutionDescriptor(ctx, req.ErrandID); !errors.Is(err, sql.ErrNoRows) {
		if err != nil {
			return blocked, fmt.Errorf("read errand launch evidence: %w", err)
		}
		return recoverOwnedErrandLaunch(ctx, store, req, current)
	}
	info, err := a.ownedErrandWorkspace(ctx, req)
	if err != nil {
		return blocked, err
	}
	if a.svc.sessionCeiling != nil {
		decision := a.svc.sessionCeiling.admit(ctx, admissionRequest{
			taskID: req.TaskID, sessionID: models.ErrandExecutionIdentity(req.ErrandID),
			origin: launchOriginAutomatic, seam: "errand_start",
		})
		if !decision.admitted {
			return errands.LaunchResult{Disposition: errands.LaunchQueued}, nil
		}
		defer a.svc.sessionCeiling.release(decision.reservationKey)
	}
	attachment, err := a.loadOwnedErrandWorkspaceAttachment(ctx, info)
	if err != nil {
		return blocked, err
	}
	if err := prepareOwnedErrandFirstTurn(ctx, store, req); err != nil {
		return blocked, err
	}
	descriptor, err := store.ReserveErrandExecutionGeneration(ctx, req.ErrandID, 0)
	if err != nil {
		return blocked, fmt.Errorf("reserve errand generation: %w", err)
	}
	launch := ownedErrandLaunchRequest(req, info, attachment)
	if err := a.startOwnedErrandExecution(ctx, store, req, launch, attachment, descriptor.Generation); err != nil {
		return blocked, err
	}
	return errands.LaunchResult{
		Disposition: errands.LaunchAdmitted, SessionID: req.ErrandID,
		TurnID: req.TurnID, SubmissionID: req.SubmissionID,
	}, nil
}

func recoverOwnedErrandLaunch(ctx context.Context, store ownedErrandLaunchStore, req errands.LaunchRequest, current *models.ErrandExecutionDescriptor) (errands.LaunchResult, error) {
	blocked := errands.LaunchResult{Disposition: errands.LaunchBlocked}
	if current == nil || current.ErrandID != req.ErrandID || current.TaskID != req.TaskID ||
		current.Generation == 0 || current.ExecutionID == "" {
		return blocked, models.ErrInvalidErrand
	}
	result := errands.LaunchResult{SessionID: req.ErrandID, TurnID: req.TurnID, SubmissionID: req.SubmissionID}
	if current.ActiveSubmissionID == req.SubmissionID && current.ActiveTurnID == req.TurnID && current.PromptGeneration != 0 {
		stored, err := store.GetAgentDeliverySubmissionMetadata(ctx, req.SubmissionID)
		if err != nil && !errors.Is(err, repoerrors.ErrAgentDeliverySubmissionNotFound) {
			return blocked, fmt.Errorf("read initial errand submission: %w", err)
		}
		if acceptedOwnedErrandLaunch(stored, current, req) {
			result.Disposition = errands.LaunchAdmitted
			return result, nil
		}
	}
	target, _, err := store.ReadOwnedErrandFirstTerminal(ctx, req.ErrandID)
	if errors.Is(err, sql.ErrNoRows) {
		return blocked, errors.New("errand launch acceptance remains unresolved")
	}
	if err != nil {
		return blocked, fmt.Errorf("read initial errand terminal receipt: %w", err)
	}
	if !matchesOwnedLaunchTerminal(target, current, req) {
		return blocked, models.ErrInvalidErrand
	}
	result.Disposition = errands.LaunchSettled
	return result, nil
}

func matchesOwnedLaunchTerminal(target models.ErrandTeardownTarget, current *models.ErrandExecutionDescriptor, req errands.LaunchRequest) bool {
	return target.TaskID == req.TaskID && target.ErrandID == req.ErrandID && target.ExecutionID == current.ExecutionID &&
		target.OwnerGeneration == current.Generation && target.TurnID == req.TurnID && target.SubmissionID == req.SubmissionID
}

func acceptedOwnedErrandLaunch(stored *models.AgentDeliverySubmission, current *models.ErrandExecutionDescriptor, req errands.LaunchRequest) bool {
	return stored != nil && stored.ID == req.SubmissionID && stored.State == models.DeliverySubmissionAccepted &&
		stored.SessionID == models.ErrandExecutionIdentity(req.ErrandID) && stored.DispatchAttemptID == current.ExecutionID &&
		stored.IncarnationID != "" && stored.HarnessGeneration > 0 && stored.OwnerGeneration == stored.HarnessGeneration
}

func (a *ErrandAdapter) ownedErrandWorkspace(ctx context.Context, req errands.LaunchRequest) (*runtimeapi.WorkspaceInfo, error) {
	if a.workspace == nil {
		return nil, errors.New("errand workspace reader is unavailable")
	}
	info, err := a.workspace(ctx, req.TaskID, req.ParentSessionID)
	if err != nil {
		return nil, fmt.Errorf("resolve errand workspace: %w", err)
	}
	if info == nil || info.TaskID != req.TaskID || info.SessionID != req.ParentSessionID ||
		info.WorkspaceID == "" || info.WorkspacePath == "" || info.ExecutorType == "" ||
		info.TaskArchived || info.WorkspaceOwnerArchived {
		return nil, errors.New("errand parent has no attachable workspace")
	}
	if !supportsOwnedErrandWorkspace(info.ExecutorType) {
		return nil, &errands.ErrLaunchRejected{Err: fmt.Errorf("%w: executor does not support errand workspace attachment", models.ErrWorkspaceReuseUnsafe)}
	}
	return info, nil
}

func prepareOwnedErrandFirstTurn(ctx context.Context, store ownedErrandLaunchStore, req errands.LaunchRequest) error {
	_, err := store.GetErrandTurn(ctx, req.ErrandID, req.TurnID)
	if errors.Is(err, sql.ErrNoRows) {
		err = store.CreateErrandTurn(ctx, req.ErrandID, &models.Turn{
			ID: req.TurnID, TaskID: req.TaskID, ExecutionProfileID: req.AgentProfileID,
			Metadata: map[string]any{"sender_session_id": req.ParentSessionID},
		})
	}
	if err != nil {
		return fmt.Errorf("reserve errand turn: %w", err)
	}
	// The reserved submission gives the first transcript entry a stable
	// identity across retries before execution reservation.
	message, err := store.GetErrandMessage(ctx, req.ErrandID, req.SubmissionID)
	if errors.Is(err, sql.ErrNoRows) {
		return store.CreateErrandMessage(ctx, req.ErrandID, &models.Message{
			ID: req.SubmissionID, TurnID: req.TurnID, TaskID: req.TaskID,
			AuthorType: models.MessageAuthorUser, Content: req.Objective,
			Metadata: map[string]any{"sender_session_id": req.ParentSessionID},
		})
	}
	if err != nil {
		return err
	}
	if message.TurnID != req.TurnID || message.Content != req.Objective ||
		message.AuthorType != models.MessageAuthorUser || message.Metadata["sender_session_id"] != req.ParentSessionID {
		return models.ErrInvalidErrand
	}
	return nil
}

func (a *ErrandAdapter) startOwnedErrandExecution(
	ctx context.Context, store ownedErrandLaunchStore, req errands.LaunchRequest,
	launch *runtimeapi.LaunchRequest, attachment *ownedErrandWorkspaceAttachment, generation uint64,
) error {
	owner := runtimeapi.ExecutionOwner{
		Kind: runtimeapi.ExecutionOwnerErrand, TaskID: req.TaskID, WorkspaceID: launch.WorkspaceID,
		ErrandID: req.ErrandID, Generation: generation, AgentProfileID: req.AgentProfileID,
	}
	admission := ownedErrandAdmission{service: a.svc, request: req, owner: owner, store: store, attachment: attachment, predecessor: launch.ErrandPredecessor}
	launch.OnExecutionAllocated = func(ctx context.Context, executionID string) error {
		if executionID != launch.CandidateExecutionID {
			return models.ErrInvalidErrand
		}
		bound, err := store.BindErrandExecution(ctx, req.ErrandID, generation, executionID)
		if err != nil {
			return err
		}
		if !bound {
			return errors.New("errand execution reservation changed")
		}
		return nil
	}
	ref, err := a.runtime.Launch(ctx, runtimeapi.LaunchSpec{
		Owner: owner, OwnerAdmission: admission, AgentProfileID: req.AgentProfileID,
		ExecutorID: launch.ExecutorType, Workspace: runtimeapi.WorkspaceRef{Path: launch.WorkspacePath},
		Prompt: ownedErrandRuntimePrompt(ctx, req, launch), PriorACPSession: launch.RequiredNativeConversationID, McpMode: mcpmode.Errand,
		Metadata: map[string]any{"launch_request": launch},
	})
	if err != nil {
		return fmt.Errorf("launch errand execution: %w", err)
	}
	if ref.ID != launch.CandidateExecutionID || ref.SessionID != "" {
		return models.ErrInvalidErrand
	}
	execution, err := a.runtime.GetExecution(ctx, ref.ID)
	if err != nil {
		return err
	}
	if execution == nil || execution.ID != ref.ID || execution.Owner != owner || execution.TaskID != "" || execution.SessionID != "" {
		return models.ErrInvalidErrand
	}
	if err := a.runtime.StartExecution(ctx, execution.ID); err != nil {
		return fmt.Errorf("start errand execution: %w", err)
	}
	return nil
}

func ownedErrandRuntimePrompt(ctx context.Context, req errands.LaunchRequest, launch *runtimeapi.LaunchRequest) string {
	if launch.ErrandPredecessor != nil && errands.ParentPromptOriginFromContext(ctx).SenderSessionID != "" {
		return req.Objective
	}
	prompt, _ := applyErrandOriginContext(req.Objective, "", &ErrandLaunch{ParentSessionID: req.ParentSessionID, Title: req.Title})
	return prompt
}

func ownedErrandLaunchRequest(req errands.LaunchRequest, info *runtimeapi.WorkspaceInfo, attachment *ownedErrandWorkspaceAttachment) *runtimeapi.LaunchRequest {
	var environment *models.TaskEnvironment
	var executorConfig map[string]string
	if attachment != nil {
		environment, executorConfig = attachment.environment, maps.Clone(attachment.executorConfig)
	}
	launch := runtimeapi.ErrandWorkspaceLaunch(info, environment)
	launch.ExecutorConfig = executorConfig
	launch.ErrandMCPSelection = &mcpconfig.SelectionSnapshot{
		DefinitionIDs:         slices.Clone(req.MCPServerIDs),
		ExcludedDefinitionIDs: slices.Clone(req.MCPExcludedServerIDs),
	}
	launch.CandidateExecutionID = uuid.NewString()
	launch.StartAgent, launch.SuppressBootStatusMessage = true, true
	launch.TurnID, launch.InitialDeliverySubmissionID = req.TurnID, req.SubmissionID
	launch.DeliveryIncarnationID, launch.DeliveryStreamID = uuid.NewString(), uuid.NewString()
	launch.DeliveryHarnessGeneration = 1
	return launch
}
