package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// Cancellation verifies the generation and active prompt under the task lock
// used by admission. An unbound generation may still acquire its first execution.
func validateOwnedCancellationTarget(ctx context.Context, tx *sqlx.Tx, current, next *models.Errand) error {
	intent := next.TeardownIntent
	if intent == nil || intent.Purpose != models.ErrandTeardownPurposeCancellation || intent.Target.ErrandID == "" ||
		(current.TeardownIntent != nil && current.TeardownIntent.ID == intent.ID) {
		return nil
	}
	target := intent.Target
	var currentExecution models.ErrandExecutionDescriptor
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT generation,execution_id,active_turn_id,active_submission_id,prompt_generation
		FROM errand_execution_generations WHERE errand_id=? ORDER BY generation DESC LIMIT 1`), current.ID).
		Scan(&currentExecution.Generation, &currentExecution.ExecutionID, &currentExecution.ActiveTurnID,
			&currentExecution.ActiveSubmissionID, &currentExecution.PromptGeneration)
	if errors.Is(err, sql.ErrNoRows) && target.OwnerGeneration == 0 {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err != nil || !ownedCancellationTargetMatches(target, currentExecution) {
		return models.ErrErrandVersionConflict
	}
	return nil
}

func ownedCancellationTargetMatches(target models.ErrandTeardownTarget, execution models.ErrandExecutionDescriptor) bool {
	return target.OwnerGeneration == execution.Generation &&
		(target.ExecutionID == "" || target.ExecutionID == execution.ExecutionID) &&
		target.TurnID == execution.ActiveTurnID && target.SubmissionID == execution.ActiveSubmissionID &&
		target.PromptGeneration == execution.PromptGeneration
}
