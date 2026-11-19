package errands

import "context"

// SetInterruptionReconciler wires retained journal recovery before the workers start.
func (s *Service) SetInterruptionReconciler(reconcile func(context.Context) error) {
	s.reconcileInterruptions = reconcile
}

func (s *Service) recoverInterruptions(ctx context.Context) error {
	if s.reconcileInterruptions == nil {
		return nil
	}
	return s.reconcileInterruptions(ctx)
}
