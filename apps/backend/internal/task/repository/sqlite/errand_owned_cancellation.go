package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// Cancellation captures the generation under the same task lock used by
// reservation. An unbound generation may still acquire its first execution.
func validateOwnedCancellationTarget(ctx context.Context, tx *sqlx.Tx, current, next *models.Errand) error {
	intent := next.TeardownIntent
	if intent == nil || intent.Purpose != models.ErrandTeardownPurposeCancellation || intent.Target.ErrandID == "" ||
		(current.TeardownIntent != nil && current.TeardownIntent.ID == intent.ID) {
		return nil
	}
	target := intent.Target
	var generation uint64
	var executionID string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT generation,execution_id FROM errand_execution_generations
		WHERE errand_id=? ORDER BY generation DESC LIMIT 1`), current.ID).Scan(&generation, &executionID)
	if errors.Is(err, sql.ErrNoRows) && target.OwnerGeneration == 0 {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err != nil || generation != target.OwnerGeneration || (target.ExecutionID != "" && target.ExecutionID != executionID) {
		return models.ErrErrandVersionConflict
	}
	return nil
}
