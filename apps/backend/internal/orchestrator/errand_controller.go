package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/errands"
)

// ErrandController is the part of errands.Service that session lifecycle code
// calls. Every method is safe to call for sessions that are not errands or
// parents.
type ErrandController interface {
	DrainQueued(ctx context.Context)
	CascadeParent(parentSessionID string) bool
	CancelOnSessionStop(ctx context.Context, sessionID string) error
	CascadeTask(taskID string) bool
	OnSessionStateChanged(ctx context.Context, sessionID string) error
	OnErrandTurnSettled(ctx context.Context, settlement errands.TurnSettlement) error
	OnErrandTurnCancelled(ctx context.Context, sessionID, turnID, submissionID string) error
	OnParentTurnSettled(ctx context.Context, parentSessionID string, cause errands.SettleCause) (bool, error)
	RetryErrandCompletion(ctx context.Context, sessionID string) error
	ReconcileTerminalDecisions(ctx context.Context, sessionID string) error
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
