package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

// ParentAwaitEnder ends every in-flight await of a parent session without
// cancelling its errands.
type ParentAwaitEnder interface {
	EndParentAwaits(parentSessionID string)
}

func (s *Service) preserveErrandDeletionWithOwnership(ctx context.Context, session *models.TaskSession, guard *lockedCancelInFlightGuard) (*models.TaskSession, error) {
	if !models.IsErrandSession(session) {
		return session, s.preserveErrandNoticeOnDelete(ctx, session.ID)
	}
	expected, err := s.captureErrandDeletionTarget(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	guard.unlock()
	noticeErr := s.preserveErrandNoticeOnDelete(errands.WithCancellationTargetFence(ctx, expected), session.ID)
	guardErr := guard.relockWithContext(ctx)
	if err := errors.Join(noticeErr, guardErr); err != nil {
		return nil, err
	}
	observed, err := s.captureErrandDeletionTarget(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	// Successful physical stop may remove the execution. Every remaining
	// identity component must still describe the original deletion target.
	if observed.ExecutionID == "" {
		expected.ExecutionID, expected.PromptGeneration = "", 0
	}
	if observed != expected {
		return nil, models.ErrErrandVersionConflict
	}
	fresh, err := s.repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	if fresh.State == models.TaskSessionStateRunning || fresh.State == models.TaskSessionStateStarting {
		return nil, models.ErrErrandVersionConflict
	}
	return fresh, nil
}

func (s *Service) captureErrandDeletionTarget(ctx context.Context, sessionID string) (models.ErrandTeardownTarget, error) {
	store, ok := s.repo.(errandTeardownStore)
	if !ok || s.turnService == nil || s.agentManager == nil {
		return models.ErrandTeardownTarget{}, errors.New("errand deletion identity authority is unavailable")
	}
	row, err := store.GetErrand(ctx, sessionID)
	if err != nil {
		return models.ErrandTeardownTarget{}, err
	}
	owner := errandTeardownOwner{svc: s}
	return owner.captureTarget(ctx, store, row)
}

func (s *Service) notifyErrandSessionStateChanged(ctx context.Context, sessionID string) {
	observer, ok := s.errands.(interface {
		OnSessionStateChanged(context.Context, string) error
	})
	if !ok {
		return
	}
	if err := observer.OnSessionStateChanged(ctx, sessionID); err != nil {
		s.logger.Warn("failed to notify errands of session state change",
			zap.String("session_id", sessionID), zap.Error(err))
	}
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

// stopErrandOnUserStop routes a positively identified errand to its complete
// cancellation owner. Unreadable role evidence never authorizes ordinary stop.
func (s *Service) stopErrandOnUserStop(ctx context.Context, sessionID string) (bool, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("read session role before stop: %w", err)
	}
	if session == nil {
		return false, models.ErrTaskSessionNotFound
	}
	if !models.IsErrandSession(session) {
		return false, nil
	}
	if s.errands == nil {
		return true, errors.New("errand cancellation service is unavailable")
	}
	return true, s.errands.CancelOnSessionStop(ctx, sessionID)
}

// cancelErrandOnUserStop is the compatibility notification for deletion hooks.
func (s *Service) cancelErrandOnUserStop(ctx context.Context, sessionID string) {
	if _, err := s.stopErrandOnUserStop(ctx, sessionID); err != nil {
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
