package errands

import (
	"context"
	"errors"
)

// SetInterruptedDelegationReconciler wires authenticated recovery before result
// reconciliation. Install it during construction, before workers start.
func (s *Service) SetInterruptedDelegationReconciler(reconcile func(context.Context, string) error) {
	s.reconcileInterruptedDelegation = reconcile
}

func (s *Service) retryInterruptedDelegations(ctx context.Context) error {
	if s.reconcileInterruptedDelegation == nil {
		return nil
	}
	store, ok := s.store.(interface {
		HasUnsettledInterruptedDelegation(context.Context, string) (bool, error)
	})
	if !ok {
		return nil
	}
	rows, err := s.store.ListErrands(ctx)
	if err != nil {
		return err
	}
	owners := s.inspectLifecycle(ctx, rows)
	failures := owners.err
	for _, row := range rows {
		if !owners.permits(row) {
			continue
		}
		pending, err := store.HasUnsettledInterruptedDelegation(ctx, row.ID)
		if err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		if pending {
			failures = errors.Join(failures, s.ReconcileTerminalDecisions(ctx, row.ID))
		}
	}
	return failures
}
