package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// HasErrandPreparedOwnership checks full live source identity rather than an
// historical admission receipt or a scalar token. Release rechecks in its own
// transaction so reconciliation's observation never authorizes a stale write.
func (r *Repository) HasErrandPreparedOwnership(ctx context.Context, taskID, parentID string, claim models.ErrandResultClaim) (bool, error) {
	var owned bool
	err := r.withErrandTask(ctx, taskID, func(tx *sqlx.Tx) error {
		var err error
		owned, err = hasErrandPreparedOwnershipTx(ctx, tx, taskID, parentID, claim)
		return err
	})
	return owned, err
}

func hasErrandPreparedOwnershipTx(ctx context.Context, tx *sqlx.Tx, taskID, parentID string, claim models.ErrandResultClaim) (bool, error) {
	if claim.Route != models.ErrandDeliveryNotification {
		return false, nil
	}
	sources, err := messagequeue.LiveServerSourcesInTransaction(ctx, tx, taskID, parentID)
	if err != nil || len(sources) == 0 {
		return false, err
	}
	var incarnation string
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT queue_incarnation_id FROM task_sessions WHERE task_id = ? AND id = ?`), taskID, parentID).Scan(&incarnation)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	generation, _, err := messagequeue.SourceGenerationsInTransaction(ctx, tx, taskID, parentID)
	if err != nil {
		return false, err
	}
	for i := range sources {
		stamp, err := readPreparedErrandSource(&sources[i])
		if err != nil {
			continue
		}
		if stamp.Identity.SessionIncarnationID == incarnation && stamp.QueueGeneration != nil && *stamp.QueueGeneration == generation &&
			slices.Contains(stamp.Receipt.Claims, claim) {
			return true, nil
		}
	}
	return false, nil
}
