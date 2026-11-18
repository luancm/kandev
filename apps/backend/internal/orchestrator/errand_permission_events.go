package orchestrator

import (
	"context"
	"errors"
	"time"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

func (s *Service) handleNonSessionPermissionRequest(ctx context.Context, event watcher.PermissionRequestData) bool {
	switch event.OwnerKind {
	case string(runtimeapi.ExecutionOwnerRun):
		return true
	case string(runtimeapi.ExecutionOwnerErrand):
		if s.ownedErrandPermissionHandler != nil {
			s.ownedErrandPermissionHandler(ctx, event)
		}
		return true
	default:
		return false
	}
}

// WireOwnedPermissionEvents delivers projected permission events to the owned
// resolver and refreshes task-scoped status when an action needs attention.
func (a *ErrandAdapter) WireOwnedPermissionEvents(statusChanged func(context.Context, string, string)) {
	a.permissionStatusChanged = statusChanged
	a.svc.ownedErrandPermissionHandler = a.handleOwnedPermissionRequest
}

func (a *ErrandAdapter) handleOwnedPermissionRequest(ctx context.Context, event watcher.PermissionRequestData) {
	if event.ErrandID == "" || event.TaskID == "" || event.TaskSessionID != "" || event.HumanInput {
		return
	}
	if !event.AutoApprovalPending {
		if event.AutoApprovedOptionID == "" && a.permissionStatusChanged != nil {
			a.permissionStatusChanged(ctx, event.TaskID, event.ErrandID)
		}
		return
	}
	request := errands.PermissionResponseRequest{
		TaskID: event.TaskID, ErrandID: event.ErrandID, RequestID: event.RequestID,
		PendingID: event.PendingID, OptionID: event.AutoApprovedOptionID,
	}
	if !request.Valid() || event.AutoApprovalSource != streams.PermissionDecisionSourceAutoApprove {
		return
	}
	err := a.resolveOwnedPermission(ctx, request, &event)
	if err != nil && !errors.Is(err, ErrPermissionAlreadyResolved) && !errors.Is(err, ErrPermissionResolutionInProgress) {
		a.svc.logger.Debug("resolve automatic errand permission", zap.Error(err))
	}
}

func ownedPermissionOriginMatches(event *watcher.PermissionRequestData, source models.ErrandPermissionSource) bool {
	return source.Owner.ExecutionID == event.AgentID && source.Owner.Generation == event.ErrandGeneration &&
		source.SubmissionID == event.DeliverySubmissionID && source.TurnID == event.TurnID && source.PromptGeneration == event.PromptGeneration
}

func (a *ErrandAdapter) showUnclaimedAutomaticPermission(ctx context.Context, store ownedErrandPermissionStore, source models.ErrandPermissionSource, event *watcher.PermissionRequestData, selectionErr error) {
	presentation := models.PermissionPresentationUnavailable
	if errors.Is(selectionErr, ErrPermissionOptionNotOffered) {
		presentation = models.PermissionPresentationInteractive
	}
	updateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	changed, err := store.UpdateErrandAutomaticPermissionPresentation(updateCtx, source, event.AutoApprovedOptionID, event.AutoApprovedOptionKind, presentation)
	if err != nil {
		a.svc.logger.Debug("update automatic errand permission presentation", zap.Error(err))
		return
	}
	if changed {
		a.publishErrandPermissionChange(updateCtx, source.Owner)
	}
}
