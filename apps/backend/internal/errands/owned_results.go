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
	store, ok := s.store.(ownedErrandResultStore)
	if !ok {
		return nil
	}
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
	settlement := TurnSettlement{TurnID: target.TurnID, SubmissionID: target.SubmissionID, Outcome: models.ErrandOutcomeDone}
	if outcome != models.DeliverySubmissionCompleted {
		settlement.Outcome = models.ErrandOutcomeFailed
		settlement.Interrupted = outcome == models.DeliverySubmissionCancelled
	}
	parentInitiated, err := s.reconcileOwnedErrandResultOrigin(ctx, store, row, target)
	if err != nil {
		return err
	}
	text, truncated, kind := resultOf(settlement, view, parentInitiated)
	now := s.clock.Now()
	unlock := s.parentLocks.lock(row.ParentSessionID)
	next, changed, err := s.update(ctx, row.ID, func(e *models.Errand) bool {
		if e.Lifecycle != models.ErrandLifecycleLaunched || e.LatestSettledTurnID != row.LatestSettledTurnID ||
			e.LatestSettledTurnID == target.TurnID || (parentInitiated && !e.AwaitingResult) {
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
	s.recordTerminalSettlement(next, settlement, parentInitiated, false)
	if parentInitiated {
		s.afterResultWrite(ctx, next)
	} else {
		s.publish(ctx, next)
	}
	return nil
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
