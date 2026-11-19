package orchestrator

import (
	"context"

	"go.uber.org/zap"
)

// reconcileErrandsOnStartup repairs errand state after session recovery. A
// failure is logged; the next boot retries the same repairs.
func (s *Service) reconcileErrandsOnStartup(ctx context.Context) {
	reconciler := s.errands
	if reconciler == nil {
		return
	}
	if err := s.reconcileErrandParentAdmissions(ctx); err != nil {
		s.logger.Warn("errand admission recovery failed", zap.Error(err))
	}
	if err := reconciler.Reconcile(ctx); err != nil {
		s.logger.Warn("errand startup reconciliation failed", zap.Error(err))
	}
}
