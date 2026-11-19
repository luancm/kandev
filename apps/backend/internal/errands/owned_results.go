package errands

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

type ownedErrandResultStore interface {
	ListOwnedErrandsAwaitingSettlement(context.Context) ([]*models.Errand, error)
	ReadOwnedErrandLatestTerminal(context.Context, string) (models.ErrandTeardownTarget, models.DeliverySubmissionState, error)
	ReadOwnedErrandParentAdmission(context.Context, models.ErrandParentAdmission) (models.ErrandAdmissionState, error)
}

func (s *Service) reconcileOwnedErrandResults(ctx context.Context) error {
	store := s.store
	rows, err := store.ListOwnedErrandsAwaitingSettlement(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, row := range rows {
		if err := s.reconcileOwnedErrandResult(ctx, store, row); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *Service) reconcileOwnedErrandResult(ctx context.Context, store ownedErrandResultStore, row *models.Errand) error {
	target, outcome, err := store.ReadOwnedErrandLatestTerminal(ctx, row.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	view, err := s.readTurn(ctx, row.ID, target.TurnID)
	if err != nil {
		return err
	}
	if !view.Completed {
		return errors.New("owned errand terminal receipt has no completed turn")
	}
	settlement := turnSettlement{TurnID: target.TurnID, Outcome: models.ErrandOutcomeDone}
	if outcome != models.DeliverySubmissionCompleted {
		settlement.Outcome = models.ErrandOutcomeFailed
		settlement.Interrupted = outcome == models.DeliverySubmissionCancelled || outcome == models.DeliverySubmissionInterruptedUnknown
	}
	parentInitiated, err := s.reconcileOwnedErrandResultOrigin(ctx, store, row, target)
	if err != nil {
		return err
	}
	text, truncated, kind := resultOf(settlement, view, parentInitiated)
	now := s.clock.Now()
	unlock := s.parentLocks.lock(row.ParentSessionID)
	next, changed, err := s.update(ctx, row.ID, func(e *models.Errand) bool {
		if !canSettleOwnedErrand(e, row.LatestSettledTurnID, target.TurnID, parentInitiated) {
			return false
		}
		applySettlement(e, settlement, parentInitiated, text, truncated, kind, now)
		e.TeardownIntent = &models.ErrandTeardownIntent{
			Version: models.ErrandTeardownIntentVersion, ID: uuid.NewString(), RequestedAt: now,
			State: models.ErrandTeardownIntentPending, Purpose: models.ErrandTeardownPurposeCompletion,
			Target: target,
		}
		return true
	})
	unlock()
	if err != nil || !changed {
		return err
	}
	if parentInitiated {
		status := models.ErrandStatusDone
		if settlement.Outcome == models.ErrandOutcomeFailed {
			status = models.ErrandStatusFailed
		}
		countSettled(status)
		s.logger.Info("errand.settled", s.errandFields(next, "status", string(status))...)
		s.afterResultWrite(ctx, next)
	} else {
		s.publish(ctx, next)
	}
	return nil
}

func canSettleOwnedErrand(current *models.Errand, previousTurnID, terminalTurnID string, parentInitiated bool) bool {
	return current.Lifecycle == models.ErrandLifecycleLaunched &&
		current.LatestSettledTurnID == previousTurnID && current.LatestSettledTurnID != terminalTurnID &&
		(!parentInitiated || current.AwaitingResult)
}

func (s *Service) reconcileOwnedErrandResultOrigin(ctx context.Context, store ownedErrandResultStore, row *models.Errand, target models.ErrandTeardownTarget) (bool, error) {
	if target.TurnID == row.FirstTurnID {
		return true, nil
	}
	admission := target.ParentAdmission(row.ParentSessionID)
	state, err := store.ReadOwnedErrandParentAdmission(ctx, admission)
	if err != nil || state == "" {
		return false, err
	}
	return true, s.store.DecideErrandParentAdmission(ctx, admission, models.ErrandAdmissionAccepted)
}
