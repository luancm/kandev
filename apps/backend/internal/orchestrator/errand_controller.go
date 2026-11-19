package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/errands"
)

// ErrandController is the part of errands.Service that session lifecycle code
// calls for ordinary parent conversations.
type ErrandController interface {
	DrainQueued(ctx context.Context)
	CascadeParent(parentSessionID string) bool
	OnParentTurnSettled(ctx context.Context, parentSessionID string, cause errands.SettleCause) (bool, error)
	ReconcileLifecycle(ctx context.Context) error
	Reconcile(ctx context.Context) error
}

// SetErrandController wires errand lifecycle hooks. Nil disables them.
func (s *Service) SetErrandController(controller ErrandController) {
	s.errands = controller
}

// drainQueuedErrands launches queued errands that the session ceiling refused.
func (s *Service) drainQueuedErrands(ctx context.Context) {
	if s.errands != nil {
		s.errands.DrainQueued(ctx)
	}
}
