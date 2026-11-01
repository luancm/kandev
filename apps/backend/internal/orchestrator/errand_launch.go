package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type errandLaunchReservationKey struct{}

// ErrandLaunch describes the errand an IntentStart launch serves. The errands
// service fills it from its own durable row, never from caller-supplied text.
type ErrandLaunch struct {
	ErrandID        string
	ParentSessionID string
	Title           string
	// SubmissionID is the errand's reserved first-prompt submission identity.
	SubmissionID         string
	MCPExcludedServerIDs []string
}

// errandStartOrigin classifies errand launches as automatic: the parent's agent
// chose to start the work, so the session ceiling queues the errand instead of
// letting it override the ceiling like a user action.
func errandStartOrigin(errand *ErrandLaunch) launchOrigin {
	if errand == nil {
		return ""
	}
	return launchOriginAutomatic
}

// LaunchErrandSession starts one errand session on the task's shared workspace
// with the explicit profile and the reserved identity in req. A ceiling refusal
// returns ErrSessionCeilingRefused and creates nothing.
func (s *Service) LaunchErrandSession(ctx context.Context, req errands.LaunchRequest) (*LaunchSessionResponse, error) {
	result, response, err := s.launchErrand(ctx, req)
	if result.Disposition == errands.LaunchQueued && err == nil {
		return nil, ErrSessionCeilingRefused
	}
	return response, err
}

// Both entry points own this guard once. Its namespace serializes complete
// launch attempts while the session's own admission guard remains available
// to cancellation at creation, registration and dispatch boundaries.
func (s *Service) launchErrand(ctx context.Context, req errands.LaunchRequest) (errands.LaunchResult, *LaunchSessionResponse, error) {
	blocked := errands.LaunchResult{Disposition: errands.LaunchBlocked}
	if err := s.authorizeTask(ctx, req.TaskID); err != nil {
		return blocked, nil, err
	}
	if !validErrandLaunchRequest(req) {
		return blocked, nil, errors.New("errand launch requires an errand, task, parent, profile, objective, and reserved identity")
	}
	launchGuard, err := s.lockCancelInFlightGuardWithContext(ctx, "errand-launch:"+req.ErrandID)
	if err != nil {
		return blocked, nil, err
	}
	defer launchGuard.release()
	evidence, state, err := s.inspectErrandLaunch(ctx, req)
	if err != nil {
		return blocked, nil, s.blockReservedErrandLaunch(ctx, req, evidence, err)
	}
	if state == errandLaunchRecoveryAdmitted || state == errandLaunchRecoverySettled {
		return recoveredErrandLaunch(evidence, state)
	}
	var response *LaunchSessionResponse
	var launchErr error
	if state == errandLaunchRecoveryPrepared {
		response, launchErr = s.continueReservedErrandLaunch(ctx, req, evidence.Session)
	} else {
		response, launchErr = s.dispatchNewErrandLaunch(ctx, req)
	}
	if errors.Is(launchErr, ErrSessionCeilingRefused) {
		return errands.LaunchResult{Disposition: errands.LaunchQueued}, nil, nil
	}
	if launchErr == nil && response != nil {
		return errands.LaunchResult{Disposition: errands.LaunchAdmitted, SessionID: req.ErrandID,
			TurnID: req.TurnID, SubmissionID: req.SubmissionID}, response, nil
	}
	// A failed return is not proof of non-admission. Read fresh durable and
	// runtime evidence before deciding whether bookkeeping may adopt it.
	evidence, state, inspectErr := s.inspectErrandLaunch(ctx, req)
	if inspectErr == nil && (state == errandLaunchRecoveryAdmitted || state == errandLaunchRecoverySettled) {
		return recoveredErrandLaunch(evidence, state)
	}
	return blocked, nil, s.blockReservedErrandLaunch(ctx, req, evidence, errors.Join(launchErr, inspectErr))
}

func validErrandLaunchRequest(req errands.LaunchRequest) bool {
	return req.ErrandID != "" && req.TaskID != "" && req.ParentSessionID != "" && req.AgentProfileID != "" &&
		req.TurnID != "" && req.SubmissionID != "" && strings.TrimSpace(req.Objective) != ""
}

func (s *Service) dispatchNewErrandLaunch(ctx context.Context, req errands.LaunchRequest) (*LaunchSessionResponse, error) {
	ctx = context.WithValue(ctx, errandLaunchReservationKey{}, req)
	ctx = models.WithReservedLaunchIdentity(ctx, models.ReservedLaunchIdentity{
		TaskID: req.TaskID, SessionID: req.ErrandID, TurnID: req.TurnID, SubmissionID: req.SubmissionID,
	})
	mcpServerIDs, err := s.enabledErrandMCPServers(ctx, req)
	if err != nil {
		return nil, err
	}
	return s.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:          req.TaskID,
		Intent:          IntentStart,
		AgentProfileID:  req.AgentProfileID,
		ProfileExplicit: true,
		Prompt:          req.Objective,
		MCPServerIDs:    mcpServerIDs,
		Errand: &ErrandLaunch{
			ErrandID: req.ErrandID, ParentSessionID: req.ParentSessionID,
			Title: req.Title, SubmissionID: req.SubmissionID,
			MCPExcludedServerIDs: req.MCPExcludedServerIDs,
		},
	})
}

func (s *Service) validateErrandLaunchReservation(ctx context.Context, req errands.LaunchRequest, queuedOnly bool) error {
	store, ok := s.repo.(interface {
		GetErrand(context.Context, string) (*models.Errand, error)
	})
	if !ok {
		return errors.New("errand launch reservation store is unavailable")
	}
	row, err := store.GetErrand(ctx, req.ErrandID)
	if err != nil {
		return fmt.Errorf("read errand launch reservation: %w", err)
	}
	if row == nil || row.TaskID != req.TaskID || row.ParentSessionID != req.ParentSessionID ||
		row.FirstTurnID != req.TurnID || row.SubmissionID != req.SubmissionID ||
		row.AgentProfileID != req.AgentProfileID {
		return models.ErrInvalidErrand
	}
	if row.Lifecycle == models.ErrandLifecycleCancelled || (queuedOnly && row.Lifecycle != models.ErrandLifecycleQueued) {
		return errors.New("errand launch reservation is no longer runnable")
	}
	return s.validateErrandLaunchOwner(ctx, req)
}

func (s *Service) validateErrandLaunchOwner(ctx context.Context, req errands.LaunchRequest) error {
	task, err := s.repo.GetTask(ctx, req.TaskID)
	if err != nil {
		return fmt.Errorf("read errand task lifecycle: %w", err)
	}
	if task == nil || task.ID != req.TaskID || task.ArchivedAt != nil {
		return errors.New("errand task is no longer runnable")
	}
	parent, err := s.repo.GetTaskSession(ctx, req.ParentSessionID)
	if err != nil {
		return fmt.Errorf("read errand parent lifecycle: %w", err)
	}
	if parent == nil || parent.ID != req.ParentSessionID || parent.TaskID != req.TaskID ||
		parent.Role != "" || parent.ParentSessionID != "" || parent.IsPassthrough || isTerminalSessionState(parent.State) {
		return errors.New("errand parent is no longer runnable")
	}
	return nil
}

func (s *Service) lockErrandLaunchAdmission(ctx context.Context, req errands.LaunchRequest, queuedOnly bool) (*lockedCancelInFlightGuard, error) {
	guard, err := s.lockCancelInFlightGuardWithContext(ctx, req.ErrandID)
	if err != nil {
		return nil, err
	}
	if s.isCancelInFlight(req.ErrandID) {
		guard.release()
		return nil, ErrCancelInFlight
	}
	if err := s.validateErrandLaunchReservation(ctx, req, queuedOnly); err != nil {
		guard.release()
		return nil, err
	}
	return guard, nil
}

func (s *Service) guardErrandSessionCreation(ctx context.Context) (func(), error) {
	req, ok := ctx.Value(errandLaunchReservationKey{}).(errands.LaunchRequest)
	if !ok {
		return func() {}, nil
	}
	guard, err := s.lockErrandLaunchAdmission(ctx, req, true)
	if err != nil {
		return nil, err
	}
	return guard.release, nil
}

// enabledErrandMCPServers drops snapshot definitions that were disabled or
// removed after the parent selected them. It returns nil when none remain, so
// the session keeps its default scope.
func (s *Service) enabledErrandMCPServers(ctx context.Context, req errands.LaunchRequest) ([]string, error) {
	if len(req.MCPServerIDs) == 0 || s.mcpSelectionReader == nil {
		return nil, nil
	}
	task, err := s.repo.GetTask(ctx, req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("load task for errand MCP selections: %w", err)
	}
	ids := slices.DeleteFunc(slices.Clone(req.MCPServerIDs), func(id string) bool {
		return slices.Contains(req.MCPExcludedServerIDs, id)
	})
	if len(ids) == 0 {
		return nil, nil
	}
	enabled, err := s.mcpSelectionReader.EnabledDefinitionIDs(ctx, task.WorkspaceID, ids)
	if err != nil {
		return nil, fmt.Errorf("resolve errand MCP selections: %w", err)
	}
	if len(enabled) == 0 {
		return nil, nil
	}
	return enabled, nil
}

// deferredLaunchClaimForStart claims the task's start-later intent for an
// ordinary start. An errand start owns no such intent, so it holds an empty
// claim that releases and consumes nothing.
func (s *Service) deferredLaunchClaimForStart(ctx context.Context, taskID string, opts startTaskOptions) *deferredLaunchClaim {
	if opts.Errand != nil {
		return &deferredLaunchClaim{svc: s, taskID: taskID}
	}
	return s.claimDeferredLaunchForStart(ctx, taskID)
}

// withErrandSessionContext makes the executor stamp the errand role and parent
// on the session row it inserts.
func withErrandSessionContext(ctx context.Context, errand *ErrandLaunch) context.Context {
	if errand == nil {
		return ctx
	}
	return executor.WithErrandSession(ctx, errand.ParentSessionID)
}

// configureErrandInitialLaunch binds the errand's reserved submission identity
// to the first prompt.
func (s *Service) configureErrandInitialLaunch(
	ctx context.Context, errand *ErrandLaunch, session *models.TaskSession, launchOptions *executor.LaunchOptions,
) error {
	if errand == nil {
		return nil
	}
	if session == nil {
		return models.ErrInvalidErrand
	}
	req := errands.LaunchRequest{ErrandID: session.ID, TaskID: session.TaskID, ParentSessionID: session.ParentSessionID,
		AgentProfileID: session.AgentProfileID, TurnID: launchOptions.TurnID, SubmissionID: errand.SubmissionID}
	guard, err := s.lockErrandLaunchAdmission(ctx, req, false)
	if err != nil {
		return err
	}
	defer guard.release()
	s.configureInitialDeliveryAdmission(session, startCreatedSessionOptions{deliverySubmissionID: errand.SubmissionID, errandParentOrigin: errands.ParentPromptOrigin{SenderSessionID: errand.ParentSessionID, SubmissionID: errand.SubmissionID}}, launchOptions)
	if err := s.configureReservedErrandTurn(ctx, session, launchOptions); err != nil {
		return err
	}
	s.fenceErrandInitialLaunch(session, launchOptions)
	return nil
}

// Initial dispatch retains admission ownership until its acceptance/failure
// callback, so cancellation cannot complete between validation and native I/O.
// SQL finishes before native I/O, and publication releases the guard first.
func (s *Service) fenceErrandInitialLaunch(session *models.TaskSession, opts *executor.LaunchOptions) {
	req := errands.LaunchRequest{ErrandID: session.ID, TaskID: session.TaskID, ParentSessionID: session.ParentSessionID,
		AgentProfileID: session.AgentProfileID, TurnID: opts.TurnID, SubmissionID: opts.DeliverySubmissionID}
	beforeStart, beforeDispatch := opts.BeforeAgentStart, opts.BeforeInitialPromptDispatch
	accepted, failed := opts.OnInitialPromptAccepted, opts.OnInitialPromptFailed
	var dispatchGuard atomic.Pointer[lockedCancelInFlightGuard]
	release := func() {
		if guard := dispatchGuard.Swap(nil); guard != nil {
			guard.release()
		}
	}
	opts.BeforeAgentStart = func(ctx context.Context, executionID, prompt string) error {
		guard, err := s.lockErrandLaunchAdmission(ctx, req, false)
		if err != nil {
			return err
		}
		defer guard.release()
		return beforeStart(ctx, executionID, prompt)
	}
	opts.BeforeInitialPromptDispatch = func(ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment) error {
		guard, err := s.lockErrandLaunchAdmission(ctx, req, false)
		if err != nil {
			return err
		}
		dispatchGuard.Store(guard)
		if err := beforeDispatch(ctx, executionID, prompt, attachments); err != nil {
			release()
			return err
		}
		return nil
	}
	opts.OnInitialPromptAccepted = func(executionID string) {
		err := s.validateErrandLaunchReservation(context.Background(), req, false)
		release()
		if err != nil {
			s.logger.Warn("cancelled initial errand acceptance retained for teardown", zap.String("session_id", session.ID), zap.Error(err))
			return
		}
		if accepted != nil {
			accepted(executionID)
		}
	}
	opts.OnInitialPromptFailed = func() {
		release()
		if failed != nil {
			failed()
		}
	}
}

// errandOriginContent renders the trusted first-turn block of an errand from
// config/prompts/errand.md: identity, objective, and the result contract.
func errandOriginContent(errand *ErrandLaunch) string {
	if errand == nil || sysprompt.StripTags(errand.ParentSessionID) == "" {
		return ""
	}
	return sysprompt.FormatErrandContext(errand.ParentSessionID, errand.Title)
}

// applyErrandOriginContext prepends the errand block to the first turn and
// returns the content the first-turn injector must whitelist. It reuses the
// trusted-content slot of spawner attribution, which an errand never has.
func applyErrandOriginContext(prompt, spawnContext string, errand *ErrandLaunch) (string, string) {
	content := errandOriginContent(errand)
	if content == "" {
		return prompt, spawnContext
	}
	return sysprompt.Wrap(content) + "\n\n" + prompt, content
}

func (s *Service) continueReservedErrandLaunch(ctx context.Context, req errands.LaunchRequest, session *models.TaskSession) (*LaunchSessionResponse, error) {
	if err := s.checkSessionRecoveryBlock(ctx, req.ErrandID); err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, errandLaunchReservationKey{}, req)
	ctx = models.WithReservedLaunchIdentity(ctx, models.ReservedLaunchIdentity{
		TaskID: req.TaskID, SessionID: req.ErrandID, TurnID: req.TurnID, SubmissionID: req.SubmissionID,
	})
	row, err := s.repo.(errandRowReader).GetErrand(ctx, req.ErrandID)
	if err != nil {
		return nil, err
	}
	prompt, planMode, attachments := row.Objective, false, []v1.MessageAttachment(nil)
	initial, found, err := models.LoadInitialPromptSubmission(session.Metadata)
	if err != nil {
		return nil, err
	}
	if found {
		if initial.InlineAttachmentCount > 0 {
			return nil, errors.New("reserved initial attachment snapshot is incomplete")
		}
		prompt, planMode, attachments = initial.Content, initial.PlanMode, initial.Attachments
	}
	options := startCreatedSessionOptions{skipTaskDescriptionFallback: true, preserveDirectPrompt: true,
		promptReferencesPrepared: true, deliverySubmissionID: req.SubmissionID,
		errandParentOrigin: errands.ParentPromptOrigin{SenderSessionID: req.ParentSessionID, SubmissionID: req.SubmissionID}}
	options.beforeProviderAdmission, options.onInitialPromptAccepted, err = s.initialSubmissionDispatchCallbacks(
		ctx, req.TaskID, req.ErrandID, prompt, planMode, attachments,
	)
	if err != nil {
		return nil, err
	}
	execution, err := s.startCreatedSession(ctx, req.TaskID, req.ErrandID, session.AgentProfileID, prompt, false, planMode, true,
		attachments, nil, "", options)
	if err != nil {
		return nil, err
	}
	if execution == nil {
		return nil, errors.New("reserved errand continuation returned no admission")
	}
	return executionToLaunchResponse(req.TaskID, execution), nil
}

// Reserved CREATED launches keep capacity refusal in the errand FIFO; ordinary
// CREATED launches retain their existing session/task deferral behavior.
func (s *Service) admitCreatedSessionLaunch(ctx context.Context, session *models.TaskSession, origin launchOrigin, payload map[string]interface{}) (*sessionKeyedCeilingReservation, bool, error) {
	req, reserved := ctx.Value(errandLaunchReservationKey{}).(errands.LaunchRequest)
	if !reserved {
		return s.admitOrDeferSeam2(ctx, session.TaskID, session.ID, origin, payload)
	}
	if req.ErrandID != session.ID || !models.IsErrandSession(session) {
		return nil, false, models.ErrInvalidErrand
	}
	decision := s.sessionCeiling.admit(ctx, admissionRequest{taskID: session.TaskID, sessionID: session.ID, origin: launchOriginAutomatic, seam: "errand_prepared"})
	if !decision.admitted {
		return nil, false, ErrSessionCeilingRefused
	}
	return &sessionKeyedCeilingReservation{controller: s.sessionCeiling, key: decision.reservationKey}, false, nil
}

// The private reservation context is only set by the classified initial
// launcher. A retained pending turn is reused by ID without publishing or
// completing it to work around the session's uniqueness constraint.
func (s *Service) startInitialLaunchTurn(ctx context.Context, sessionID string) (string, bool, error) {
	req, reserved := ctx.Value(errandLaunchReservationKey{}).(errands.LaunchRequest)
	if !reserved {
		id, created := s.startTurnForSessionWithOwnership(ctx, sessionID)
		return id, created, nil
	}
	if req.ErrandID != sessionID {
		return "", false, models.ErrInvalidErrand
	}
	guard, err := s.lockErrandLaunchAdmission(ctx, req, false)
	if err != nil {
		return "", false, err
	}
	defer guard.release()
	evidence, state, err := s.readErrandLaunchEvidence(ctx, req)
	if err != nil {
		return "", false, err
	}
	if state != errandLaunchRecoveryPrepared {
		return "", false, errors.New("reserved initial turn is no longer unattempted")
	}
	turn := evidence.ReservedTurn
	if turn == nil {
		turn, err = s.turnService.ReserveTurn(ctx, sessionID, nil)
		if err != nil {
			return "", false, err
		}
	}
	s.reservedPromptTurns.Store(sessionID, newReservedPromptTurn(turn.ID))
	return turn.ID, false, nil
}

func (s *Service) configureReservedErrandTurn(ctx context.Context, session *models.TaskSession, opts *executor.LaunchOptions) error {
	req, reserved := ctx.Value(errandLaunchReservationKey{}).(errands.LaunchRequest)
	if !reserved {
		return nil
	}
	if req.ErrandID != session.ID || opts.TurnID != req.TurnID {
		return models.ErrInvalidErrand
	}
	turn, err := s.turnService.GetTurn(ctx, req.TurnID)
	if err != nil {
		return err
	}
	if turn == nil || !errandLaunchTurnMatches(&models.Errand{ID: req.ErrandID, TaskID: req.TaskID, FirstTurnID: req.TurnID}, turn) || !errandLaunchTurnUnattempted(turn) {
		return errors.New("reserved initial turn changed before dispatch configuration")
	}
	before, accepted := opts.BeforeInitialPromptDispatch, opts.OnInitialPromptAccepted
	opts.BeforeInitialPromptDispatch = func(ctx context.Context, id, prompt string, attachments []v1.MessageAttachment) error {
		if err := before(ctx, id, prompt, attachments); err != nil {
			return err
		}
		return s.turnService.MarkReservedTurnDispatchAttempted(ctx, turn)
	}
	opts.OnInitialPromptAccepted = func(id string) {
		if accepted != nil {
			accepted(id)
		}
		if err := s.turnService.PublishReservedTurn(context.Background(), turn); err != nil {
			s.logger.Warn("reserved errand turn publication retained for recovery", zap.String("session_id", session.ID), zap.String("turn_id", turn.ID), zap.Error(err))
			return
		}
		if turn.CompletedAt == nil {
			s.activeTurns.Store(session.ID, turn.ID)
		}
		s.resolveReservedPromptTurn(session.ID, turn.ID, true)
	}
	return nil
}
