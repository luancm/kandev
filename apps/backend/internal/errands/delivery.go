package errands

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// SettleCause names why a parent turn ended. A user cancel leaves pending
// results for a later parent turn instead of starting a notification.
type SettleCause string

const (
	// SettleCauseTurnDone is a turn that finished on its own, including failure.
	SettleCauseTurnDone SettleCause = "turn_done"
	// SettleCauseUserCancel is a turn the user cancelled.
	SettleCauseUserCancel SettleCause = "user_cancel"
)

// OnParentTurnSettled runs when a parent turn ends, before the orchestrator
// drains the parent's queue. It delivers pending results unless the user
// cancelled the turn, then reports whether the parent is still outstanding.
// The orchestrator records the outstanding branch only when no turn started.
func (s *Service) OnParentTurnSettled(ctx context.Context, parentSessionID string, cause SettleCause) (bool, error) {
	if !s.featureEnabled() || parentSessionID == "" {
		return false, nil
	}
	if cause != SettleCauseUserCancel {
		s.deliverPending(ctx, parentSessionID)
	}
	return s.ParentOutstanding(ctx, parentSessionID)
}

// ParentOutstanding reports whether a parent owes an outcome: one of its
// errands is queued, awaiting a parent-initiated turn, or holds a pending
// result. A parent without errands is never outstanding.
func (s *Service) ParentOutstanding(ctx context.Context, parentSessionID string) (bool, error) {
	if !s.featureEnabled() || parentSessionID == "" {
		return false, nil
	}
	rows, err := s.store.ListErrandsByParent(ctx, parentSessionID)
	if err != nil {
		return false, fmt.Errorf("list errands of parent: %w", err)
	}
	for _, row := range rows {
		if IsOutstanding(row) {
			return true, nil
		}
	}
	return false, nil
}

// deliverPending asks the deliverer for one notification when the parent has a
// pending result. The per-parent mutex covers only the read; the deliverer is
// called unlocked because it may re-enter the orchestrator. A refusal leaves
// the results pending for the next trigger or sweep.
func (s *Service) deliverPending(ctx context.Context, parentSessionID string) {
	if s.deliverer == nil || parentSessionID == "" || !s.featureEnabled() {
		return
	}
	unlock := s.parentLocks.lock(parentSessionID)
	pending, err := s.hasPendingResult(ctx, parentSessionID)
	unlock()
	if err != nil {
		s.logger.Warn("errand delivery check failed",
			zap.String("parent_session_id", parentSessionID), zap.Error(err))
		return
	}
	if !pending || s.deliverer.NotificationBlocked(ctx, parentSessionID) {
		return
	}
	s.deliverer.DeliverPending(ctx, parentSessionID)
}

func (s *Service) hasPendingResult(ctx context.Context, parentSessionID string) (bool, error) {
	rows, err := s.store.ListErrandsByParent(ctx, parentSessionID)
	if err != nil {
		return false, fmt.Errorf("list errands of parent: %w", err)
	}
	for _, row := range rows {
		if row.ResultState == models.ErrandResultPending {
			return true, nil
		}
	}
	return false, nil
}

// SweepDelivery retries delivery for every parent holding a pending result.
// The ceiling sweep calls it so a refused notification is not stranded.
func (s *Service) SweepDelivery(ctx context.Context) error {
	if !s.featureEnabled() {
		return nil
	}
	rows, err := s.store.ListErrands(ctx)
	if err != nil {
		return fmt.Errorf("list errands for delivery sweep: %w", err)
	}
	parents := map[string]struct{}{}
	for _, row := range rows {
		if row.ResultState == models.ErrandResultPending {
			parents[row.ParentSessionID] = struct{}{}
		}
	}
	for _, parentID := range sortedKeys(parents) {
		s.deliverPending(ctx, parentID)
	}
	return nil
}

// PreserveCancellationNotice cancels an errand whose session is being deleted
// and admits its cancellation notice to the parent before the row is removed.
// A parent with no outstanding work gets nothing. A refused admission returns
// an error so the caller keeps the row and the notice is not lost. It runs
// whether or not the feature flag is on.
func (s *Service) PreserveCancellationNotice(ctx context.Context, errandID string) error {
	row, _, err := s.cancelRow(ctx, errandID, models.ErrandCancelSourceUser)
	if err != nil {
		return err
	}
	if row.ResultState != models.ErrandResultPending || s.deliverer == nil {
		return nil
	}
	now := s.clock.Now()
	block := renderStatusBlock([]composedErrand{{errand: row, status: models.ErrandStatusCancelled}},
		map[string]*models.Errand{row.ID: row}, now)
	metadata := map[string]interface{}{"errand_delivery": string(models.ErrandDeliveryNotification)}
	if err := s.deliverer.AdmitPrepared(ctx, row.ParentSessionID, block, metadata); err != nil {
		return fmt.Errorf("admit cancellation notice: %w", err)
	}
	countDelivery(models.ErrandDeliveryNotification)
	return nil
}
