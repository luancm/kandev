package orchestrator

import (
	"context"

	"go.uber.org/zap"
)

// errandDeliverySweeper retries delivery for parents holding pending results.
// Type-asserted from ErrandController.
type errandDeliverySweeper interface {
	SweepDelivery(ctx context.Context) error
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
