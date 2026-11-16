package orchestrator

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// errandDeletionNotifier cancels an errand whose session is being deleted and
// admits its cancellation notice before the row is removed. Type-asserted from
// ErrandController.
type errandDeletionNotifier interface {
	PreserveCancellationNotice(ctx context.Context, errandID string) error
}

// errandDeliverySweeper retries delivery for parents holding pending results.
// Type-asserted from ErrandController.
type errandDeliverySweeper interface {
	SweepDelivery(ctx context.Context) error
}

// preserveErrandNoticeOnDelete runs before a session row is deleted. A session
// that is not an errand is ignored. A refused notice admission is returned so
// the caller keeps the row: the notice must survive the deletion.
func (s *Service) preserveErrandNoticeOnDelete(ctx context.Context, sessionID string) error {
	notifier, ok := s.errandDeletionNotifier()
	if !ok {
		s.cancelErrandOnUserStop(ctx, sessionID)
		return nil
	}
	err := notifier.PreserveCancellationNotice(ctx, sessionID)
	if err == nil || errors.Is(err, models.ErrErrandNotFound) {
		return nil
	}
	s.logger.Warn("failed to preserve errand cancellation notice before session delete",
		zap.String("session_id", sessionID), zap.Error(err))
	return err
}

func (s *Service) errandDeletionNotifier() (errandDeletionNotifier, bool) {
	if s.errands == nil {
		return nil, false
	}
	notifier, ok := s.errands.(errandDeletionNotifier)
	return notifier, ok
}

// sweepErrandDelivery retries errand result delivery for every parent with a
// pending result. A refused notification would otherwise wait for an unrelated
// parent turn.
func (s *Service) sweepErrandDelivery(ctx context.Context) {
	if s.errands == nil {
		return
	}
	sweeper, ok := s.errands.(errandDeliverySweeper)
	if !ok {
		return
	}
	if err := sweeper.SweepDelivery(ctx); err != nil {
		s.logger.Warn("errand delivery sweep failed", zap.Error(err))
	}
}
