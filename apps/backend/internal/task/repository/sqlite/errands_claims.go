package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func lockErrandTask(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	return db.LockErrandDelivery(ctx, tx, tx.DriverName(), taskID)
}

func (r *Repository) withErrandTask(ctx context.Context, taskID string, apply func(*sqlx.Tx) error) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockErrandTask(ctx, tx, taskID); err != nil {
		return err
	}
	if err := apply(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("errand transaction commit outcome unknown: %w", err)
	}
	return nil
}

// ClaimResults commits a batch of pending results as a single transaction.
func (r *Repository) ClaimResults(ctx context.Context, taskID, parentID string, route models.ErrandDeliveryRoute, token string, candidates []models.ErrandClaimCandidate, at time.Time, inventory []models.ErrandClaimCandidate) ([]*models.Errand, error) {
	if !validErrandClaim(taskID, parentID, route, token) {
		return nil, models.ErrInvalidErrand
	}
	candidates, err := orderedErrandCandidates(candidates)
	if err != nil {
		return nil, err
	}
	inventory, err = orderedErrandCandidates(inventory)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockErrandTask(ctx, tx, taskID); err != nil {
		return nil, err
	}
	replay, err := prepareErrandClaim(ctx, tx, taskID, parentID, route, token, candidates, at)
	if err != nil {
		return nil, err
	}
	if replay {
		return readErrandClaimSnapshot(ctx, tx, taskID, parentID, route, token, candidates, inventory)
	}
	if err := validateErrandInventory(ctx, tx, taskID, parentID, inventory); err != nil {
		return nil, err
	}
	claimed := make([]*models.Errand, 0, len(candidates))
	for _, candidate := range candidates {
		row, err := claimErrandResult(ctx, tx, taskID, parentID, route, token, candidate, at)
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, row)
	}
	if err := saveErrandClaimSnapshot(ctx, tx, token, candidates, inventory, claimed); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return r.readBackErrandClaim(ctx, taskID, parentID, route, token, candidates, inventory, err)
	}
	return claimed, nil
}

func validErrandClaim(taskID, parentID string, route models.ErrandDeliveryRoute, token string) bool {
	return taskID != "" && parentID != "" && token != "" &&
		(route == models.ErrandDeliveryAwait || route == models.ErrandDeliveryTurnContext || route == models.ErrandDeliveryNotification)
}

func claimErrandResult(ctx context.Context, tx *sqlx.Tx, taskID, parentID string, route models.ErrandDeliveryRoute, token string, candidate models.ErrandClaimCandidate, at time.Time) (*models.Errand, error) {
	row, err := scanErrand(tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+errandColumns+` FROM task_session_errands WHERE id = ?`), candidate.ID))
	if err != nil {
		return nil, fmt.Errorf("read claim candidate: %w", err)
	}
	if row.TaskID != taskID || row.ParentSessionID != parentID || row.ResultRevision == "" {
		return nil, models.ErrErrandVersionConflict
	}
	if row.Version != candidate.Version || row.ResultState != models.ErrandResultPending {
		return nil, models.ErrErrandVersionConflict
	}
	pending, err := unresolvedErrandAdmission(ctx, tx, row.ID)
	if err != nil {
		return nil, err
	}
	if pending {
		return nil, models.ErrErrandVersionConflict
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_session_errands SET result_state = ?, delivered_route = ?, delivery_token = ?, delivered_at = ?, updated_at = ?, version = version + 1 WHERE id = ? AND version = ?`), string(models.ErrandResultDelivered), string(route), token, at, at, row.ID, row.Version)
	if err != nil {
		return nil, fmt.Errorf("claim result: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, models.ErrErrandVersionConflict
	}
	row.ResultState, row.DeliveredRoute, row.DeliveryToken, row.DeliveredAt, row.UpdatedAt = models.ErrandResultDelivered, route, token, &at, at
	row.Version++
	if err := insertErrandClaimItem(ctx, tx, row, at); err != nil {
		return nil, err
	}
	return row, nil
}
