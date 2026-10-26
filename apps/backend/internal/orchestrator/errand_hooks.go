package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// ParentAwaitEnder ends every in-flight await of a parent session without
// cancelling its errands.
type ParentAwaitEnder interface {
	EndParentAwaits(parentSessionID string)
}

// SetParentAwaitEnder wires the registry that ends a cancelled parent turn's
// awaits. Nil disables the hook.
func (s *Service) SetParentAwaitEnder(ender ParentAwaitEnder) {
	s.parentAwaits = ender
}

// endParentAwaits ends the awaits of a parent whose turn was cancelled. It
// cancels no errand.
func (s *Service) endParentAwaits(sessionID string) {
	if s.parentAwaits != nil {
		s.parentAwaits.EndParentAwaits(sessionID)
	}
}

// cancelErrandOnUserStop records a user stop of an errand session before its
// runtime stops, so the stop is not mistaken for a failure.
func (s *Service) cancelErrandOnUserStop(ctx context.Context, sessionID string) {
	if s.errands == nil {
		return
	}
	if err := s.errands.CancelOnSessionStop(ctx, sessionID); err != nil {
		s.logger.Warn("failed to record errand cancellation on session stop",
			zap.String("session_id", sessionID), zap.Error(err))
	}
}

// cascadeErrandsOfParent cancels the errands of a session that stopped,
// failed, or was deleted, off the calling transition.
func (s *Service) cascadeErrandsOfParent(sessionID string) {
	if s.errands != nil {
		s.errands.CascadeParent(sessionID)
	}
}

// CancelErrandsForTask cancels every errand of an archived task, queued ones
// included, off the calling archive. It implements the task service's
// TaskErrandCanceller.
func (s *Service) CancelErrandsForTask(_ context.Context, taskID string) {
	if s.errands != nil {
		s.errands.CascadeTask(taskID)
	}
}

// onSessionTerminalForErrands cascades a terminal parent to its errands.
func (s *Service) onSessionTerminalForErrands(sessionID string, state models.TaskSessionState) {
	if isTerminalSessionState(state) {
		s.cascadeErrandsOfParent(sessionID)
	}
}
