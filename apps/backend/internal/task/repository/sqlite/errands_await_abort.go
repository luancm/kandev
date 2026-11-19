package sqlite

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// AbortAwait durably fences the attempt, including a claim not yet started.
// The caller must have irrevocably selected a pre-success error outcome.
func (r *Repository) AbortAwait(ctx context.Context, taskID, parentID, token string) ([]*models.Errand, error) {
	if taskID == "" || parentID == "" || token == "" {
		return nil, models.ErrInvalidErrand
	}
	return r.abortAwait(ctx, taskID, parentID, token, nil)
}

func (r *Repository) abortAwait(ctx context.Context, taskID, parentID, token string, owner *models.ErrandAwaitIdentity) ([]*models.Errand, error) {
	var changed []*models.Errand
	err := r.withErrandTask(ctx, taskID, func(tx *sqlx.Tx) error {
		if owner != nil {
			if _, err := bindErrandAwait(ctx, tx, *owner, r.nowUTC()); err != nil {
				return err
			}
		}
		if err := abortAwaitAttempt(ctx, tx, taskID, parentID, token, r.nowUTC()); err != nil {
			return err
		}
		claims, err := readErrandReleaseClaims(ctx, tx, taskID, parentID, "i.token = ? AND c.route = 'await'", token)
		if err != nil {
			return err
		}
		for _, claim := range claims {
			row, err := releaseErrandResultClaim(ctx, tx, taskID, parentID, claim, r.nowUTC())
			if err != nil {
				return err
			}
			if row != nil {
				changed = append(changed, row)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return changed, nil
}

func abortAwaitAttempt(ctx context.Context, tx *sqlx.Tx, taskID, parentID, token string, at time.Time) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO errand_await_attempt (token, task_id, parent_session_id, state, created_at, updated_at)
		VALUES (?, ?, ?, 'aborted', ?, ?) ON CONFLICT (token) DO NOTHING`), token, taskID, parentID, at, at)
	if err != nil {
		return err
	}
	var storedTask, storedParent, state string
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT task_id, parent_session_id, state FROM errand_await_attempt WHERE token = ?`), token).Scan(&storedTask, &storedParent, &state); err != nil {
		return err
	}
	if storedTask != taskID || storedParent != parentID || state == errandAwaitReceivedState {
		return models.ErrErrandVersionConflict
	}
	var wrongRoute bool
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM errand_result_claim WHERE token = ?
		AND (task_id <> ? OR parent_session_id <> ? OR route <> 'await'))`), token, taskID, parentID).Scan(&wrongRoute); err != nil {
		return err
	}
	if wrongRoute {
		return models.ErrErrandVersionConflict
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_await_attempt SET state = 'aborted', updated_at = ? WHERE token = ?`), at, token)
	return err
}
