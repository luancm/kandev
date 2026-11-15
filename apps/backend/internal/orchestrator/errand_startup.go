package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// A pending exact target must retain its runtime and turn evidence until
// authenticated recovery and Errands reconciliation classify that target.
func (s *Service) errandOwnsStartupTeardown(ctx context.Context, session *models.TaskSession) bool {
	if !models.IsErrandSession(session) {
		return false
	}
	store, ok := s.repo.(interface {
		GetErrand(context.Context, string) (*models.Errand, error)
	})
	if !ok {
		s.logger.Warn("errand startup teardown store is unavailable", zap.String("session_id", session.ID))
		return true
	}
	row, err := store.GetErrand(ctx, session.ID)
	if err != nil {
		s.logger.Warn("errand startup teardown identity is unreadable", zap.String("session_id", session.ID), zap.Error(err))
		return true
	}
	return row.Lifecycle == models.ErrandLifecycleCancelled &&
		(row.TeardownIntent == nil || row.TeardownIntent.State == models.ErrandTeardownIntentPending)
}

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
