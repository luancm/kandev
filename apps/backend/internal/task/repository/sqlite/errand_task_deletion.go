package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// requireTaskErrandsStopped runs under the task row lock, before deleting
// the Errand records that carry exact runtime stop evidence.
func requireTaskErrandsStopped(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	return requireErrandsStopped(ctx, tx, `e.task_id = ?`, taskID)
}

func requireWorkspaceErrandsStopped(ctx context.Context, tx *sqlx.Tx, workspaceID string) error {
	return requireErrandsStopped(ctx, tx, `e.task_id IN (SELECT id FROM tasks WHERE workspace_id = ?)`, workspaceID)
}

func requireErrandsStopped(ctx context.Context, tx *sqlx.Tx, scope, ownerID string) error {
	rows, err := tx.QueryContext(ctx, tx.Rebind(`
		SELECT e.id, e.task_id, e.lifecycle, e.cancel_source, e.first_turn_id,
		       e.submission_id, e.teardown_intent, g.generation, g.execution_id
		FROM task_session_errands e
		JOIN errand_execution_generations g ON g.errand_id = e.id
			AND g.generation = (SELECT MAX(latest.generation) FROM errand_execution_generations latest WHERE latest.errand_id = e.id)
		WHERE `+scope), ownerID)
	if err != nil {
		return fmt.Errorf("read errand stop evidence for task deletion: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var row models.Errand
		var encoded, executionID string
		var generation uint64
		if err := rows.Scan(&row.ID, &row.TaskID, &row.Lifecycle, &row.CancelSource, &row.FirstTurnID,
			&row.SubmissionID, &encoded, &generation, &executionID); err != nil {
			return fmt.Errorf("scan errand stop evidence: %w", err)
		}
		if err := decodeErrandTeardownIntent(encoded, &row); err != nil {
			return err
		}
		intent := row.TeardownIntent
		if intent == nil || intent.State != models.ErrandTeardownIntentCompleted ||
			intent.Target.ErrandID != row.ID || intent.Target.OwnerGeneration != generation ||
			executionID == "" || (intent.Target.ExecutionID != "" && intent.Target.ExecutionID != executionID) {
			return fmt.Errorf("%w: errand %s stop is unconfirmed", repoerrors.ErrTaskCleanupInProgress, row.ID)
		}
	}
	return rows.Err()
}
