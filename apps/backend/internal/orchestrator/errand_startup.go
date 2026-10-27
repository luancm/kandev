package orchestrator

import (
	"context"

	"go.uber.org/zap"
)

// errandStartupReconciler is the startup part of the errands service. It is
// type-asserted from ErrandController so the controller's method set stays fixed.
type errandStartupReconciler interface {
	Reconcile(ctx context.Context) error
}

// reconcileErrandsOnStartup repairs errand state after session recovery. A
// failure is logged; the next boot retries the same repairs.
func (s *Service) reconcileErrandsOnStartup(ctx context.Context) {
	reconciler, ok := s.errands.(errandStartupReconciler)
	if !ok {
		return
	}
	if err := reconciler.Reconcile(ctx); err != nil {
		s.logger.Warn("errand startup reconciliation failed", zap.Error(err))
	}
}
