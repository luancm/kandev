package orchestrator

import "context"

// ErrandController is the part of errands.Service that session lifecycle code
// calls. Every method is safe to call for sessions that are not errands or
// parents.
type ErrandController interface {
	DrainQueued(ctx context.Context)
	CascadeParent(parentSessionID string) bool
	CancelOnSessionStop(ctx context.Context, sessionID string) error
	CascadeTask(taskID string) bool
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
