package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"go.uber.org/zap"
)

type ownedErrandPermissionStore interface {
	GetErrand(context.Context, string) (*models.Errand, error)
	GetErrandExecutionDescriptor(context.Context, string) (*models.ErrandExecutionDescriptor, error)
	LookupErrandPermissionSource(context.Context, models.ErrandPermissionOwner, string, string) (*models.ErrandPermissionSource, error)
	ClaimErrandPermissionResolution(context.Context, models.ErrandPermissionSource, models.PermissionResolutionAudit, bool) (*models.PermissionResolutionClaimResult, error)
	FinalizeErrandPermissionResolution(context.Context, models.ErrandPermissionSource, models.ErrandPermissionFinalization) (*models.PermissionResolutionFinalizeResult, error)
}

// RespondToErrandPermission accepts one provider-offered action after claiming
// its immutable Errand source. A final result belongs to that original claim.
func (a *ErrandAdapter) RespondToErrandPermission(ctx context.Context, request errands.PermissionResponseRequest) error {
	if !request.Valid() {
		return ErrPermissionNotFound
	}
	if err := a.svc.authorizeTask(ctx, request.TaskID); err != nil {
		return models.ErrErrandNotFound
	}
	store, ok := a.svc.repo.(ownedErrandPermissionStore)
	if !ok || a.runtime == nil {
		return ErrPermissionAuditFailed
	}
	target, source, err := a.readErrandPermissionTarget(ctx, store, request)
	if err != nil {
		return err
	}
	snapshot, err := a.runtime.SnapshotErrandPermissions(ctx, target)
	if runtimeapi.IsNotFound(err) || runtimeapi.IsTerminalPermissionError(err) {
		return ErrPermissionStale
	}
	if err != nil {
		return err
	}
	choice, err := chooseErrandPermission(snapshot, request)
	if err != nil {
		return err
	}
	claimID, err := claimErrandPermission(ctx, store, source, choice)
	if err != nil {
		return err
	}
	status, deliveryErr := a.deliverErrandPermission(ctx, snapshot, source, choice, request.Cancelled)
	return a.finalizeErrandPermission(ctx, store, source, claimID, status, deliveryErr)
}

func (a *ErrandAdapter) readErrandPermissionTarget(ctx context.Context, store ownedErrandPermissionStore, request errands.PermissionResponseRequest) (runtimeapi.ErrandPermissionTarget, models.ErrandPermissionSource, error) {
	var target runtimeapi.ErrandPermissionTarget
	var source models.ErrandPermissionSource
	row, err := store.GetErrand(ctx, request.ErrandID)
	if err != nil || row == nil || row.TaskID != request.TaskID {
		return target, source, models.ErrErrandNotFound
	}
	task, err := a.svc.repo.GetTask(ctx, request.TaskID)
	if err != nil || task == nil || task.WorkspaceID == "" {
		return target, source, models.ErrErrandNotFound
	}
	descriptor, err := store.GetErrandExecutionDescriptor(ctx, request.ErrandID)
	if errors.Is(err, sql.ErrNoRows) {
		return target, source, ErrPermissionStale
	}
	if err != nil {
		return target, source, fmt.Errorf("read errand permission execution: %w", err)
	}
	if descriptor == nil || descriptor.TaskID != request.TaskID || descriptor.ErrandID != request.ErrandID || descriptor.ExecutionID == "" {
		return target, source, ErrPermissionStale
	}
	owner := models.ErrandPermissionOwner{
		ErrandID: descriptor.ErrandID, TaskID: descriptor.TaskID, Generation: descriptor.Generation, ExecutionID: descriptor.ExecutionID,
	}
	origin, err := lookupOwnedErrandPermissionSource(ctx, store, owner, request)
	if err != nil {
		return target, source, err
	}
	target = runtimeapi.ErrandPermissionTarget{
		Owner: runtimeapi.ExecutionOwner{
			Kind: runtimeapi.ExecutionOwnerErrand, TaskID: owner.TaskID, WorkspaceID: task.WorkspaceID,
			ErrandID: owner.ErrandID, Generation: owner.Generation, AgentProfileID: row.AgentProfileID,
		},
		ExecutionID: owner.ExecutionID, IncarnationID: origin.IncarnationID,
		HarnessGeneration: uint64(origin.HarnessGeneration), StreamID: origin.StreamID,
	}
	return target, *origin, nil
}

func lookupOwnedErrandPermissionSource(ctx context.Context, store ownedErrandPermissionStore, owner models.ErrandPermissionOwner, request errands.PermissionResponseRequest) (*models.ErrandPermissionSource, error) {
	source, err := store.LookupErrandPermissionSource(ctx, owner, request.RequestID, request.PendingID)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) {
		return nil, ErrPermissionStale
	}
	if err != nil {
		return nil, fmt.Errorf("read errand permission source: %w", err)
	}
	if source == nil || source.Owner != owner || source.HarnessGeneration <= 0 {
		return nil, ErrPermissionStale
	}
	return source, nil
}

func chooseErrandPermission(snapshot *runtimeapi.ErrandPermissionSnapshot, request errands.PermissionResponseRequest) (*streams.PermissionChoice, error) {
	if snapshot == nil {
		return nil, ErrPermissionStale
	}
	permission, err := findPendingPermission(snapshot.Permissions, request.RequestID, request.PendingID)
	if err != nil {
		return nil, err
	}
	if permission.HumanInput {
		return nil, ErrPermissionHumanInputRequired
	}
	if request.Cancelled {
		return &streams.PermissionChoice{OptionID: stopReasonCancelled, Kind: streams.PermissionOptionKind(stopReasonCancelled)}, nil
	}
	return offeredPermissionOption(permission, request.OptionID)
}

func claimErrandPermission(ctx context.Context, store ownedErrandPermissionStore, source models.ErrandPermissionSource, choice *streams.PermissionChoice) (string, error) {
	userID, actor := permissionAuditActor(ctx)
	audit := models.PermissionResolutionAudit{
		ClaimID: uuid.NewString(), ActorUserID: userID, ActorKind: actor, Source: models.PermissionSourceWeb,
		RequestID: source.RequestID, PendingID: source.PendingID, OptionID: choice.OptionID,
		OptionKind: string(choice.Kind), SelectedAt: time.Now().UTC(),
	}
	claim, err := store.ClaimErrandPermissionResolution(ctx, source, audit, false)
	if errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) {
		return "", ErrPermissionStale
	}
	if err != nil || claim == nil {
		return "", ErrPermissionAuditFailed
	}
	switch claim.Outcome {
	case models.PermissionClaimed:
		return audit.ClaimID, nil
	case models.PermissionClaimInProgress:
		return "", ErrPermissionResolutionInProgress
	case models.PermissionClaimAlreadyFinal:
		return "", ErrPermissionAlreadyResolved
	case models.PermissionClaimNotFound:
		return "", ErrPermissionStale
	default:
		return "", ErrPermissionAuditFailed
	}
}

func (a *ErrandAdapter) deliverErrandPermission(ctx context.Context, snapshot *runtimeapi.ErrandPermissionSnapshot, source models.ErrandPermissionSource, choice *streams.PermissionChoice, cancelled bool) (models.PermissionStatus, error) {
	if cancelled {
		return a.cancelOwnedErrandPermission(ctx, snapshot, source)
	}
	result, err := a.runtime.ResolveErrandPermission(ctx, snapshot, source, choice.OptionID)
	if err != nil {
		return "", err
	}
	if result == nil || result.RequestID != source.RequestID || result.PendingID != source.PendingID ||
		result.OptionID != choice.OptionID || result.OptionKind != choice.Kind || result.Status != "resolved" {
		return "", ErrPermissionDeliveryFailed
	}
	if strings.HasPrefix(string(result.OptionKind), "reject_") {
		return models.PermissionStatusRejected, nil
	}
	return models.PermissionStatusApproved, nil
}

func (a *ErrandAdapter) cancelOwnedErrandPermission(ctx context.Context, snapshot *runtimeapi.ErrandPermissionSnapshot, source models.ErrandPermissionSource) (models.PermissionStatus, error) {
	result, err := a.runtime.CancelErrandPermission(ctx, snapshot, source)
	if err != nil {
		return "", err
	}
	if result == nil || result.RequestID != source.RequestID || result.PendingID != source.PendingID || result.Status != stopReasonCancelled {
		return "", ErrPermissionDeliveryFailed
	}
	return models.PermissionStatusRejected, nil
}

func (a *ErrandAdapter) finalizeErrandPermission(ctx context.Context, store ownedErrandPermissionStore, source models.ErrandPermissionSource, claimID string, status models.PermissionStatus, deliveryErr error) error {
	result := models.PermissionResolutionAccepted
	var outcomeErr error
	if deliveryErr != nil {
		result, status, outcomeErr = models.PermissionResolutionFailed, models.PermissionStatusExpired, ErrPermissionDeliveryFailed
		if runtimeapi.IsNotFound(deliveryErr) || runtimeapi.IsTerminalPermissionError(deliveryErr) {
			result, outcomeErr = models.PermissionResolutionStale, ErrPermissionStale
		}
	}
	finalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	finalized, err := store.FinalizeErrandPermissionResolution(finalCtx, source, models.ErrandPermissionFinalization{
		ClaimID: claimID, Result: result, Status: status, FinalizedAt: time.Now().UTC(),
	})
	if err != nil || finalized == nil || finalized.Outcome != models.PermissionFinalized {
		return ErrPermissionFinalizationFailed
	}
	if a.svc.eventBus != nil {
		payload := map[string]any{"task_id": source.Owner.TaskID, "errand_id": source.Owner.ErrandID}
		event := bus.NewEvent(events.ErrandTranscriptChanged, "orchestrator", payload)
		if err := a.svc.eventBus.Publish(finalCtx, events.ErrandTranscriptChanged, event); err != nil {
			a.svc.logger.Debug("publish errand permission change", zap.Error(err))
		}
	}
	return outcomeErr
}
