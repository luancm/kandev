package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

const errandClaimedState = "claimed"

func prepareErrandClaim(ctx context.Context, tx *sqlx.Tx, taskID, parentID string, route models.ErrandDeliveryRoute, token string, candidates []models.ErrandClaimCandidate, at time.Time) (bool, error) {
	var storedTask, storedParent, storedRoute string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT task_id, parent_session_id, route FROM errand_result_claim WHERE token = ?`), token).
		Scan(&storedTask, &storedParent, &storedRoute)
	if err == nil {
		if storedTask != taskID || storedParent != parentID || storedRoute != string(route) {
			return false, models.ErrErrandVersionConflict
		}
		var count int
		if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT COUNT(*) FROM errand_result_claim_item WHERE token = ?`), token).Scan(&count); err != nil {
			return false, err
		}
		if count != len(candidates) {
			return false, models.ErrErrandVersionConflict
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	replay := err == nil
	if route == models.ErrandDeliveryAwait {
		if err := prepareAwaitClaim(ctx, tx, taskID, parentID, token, at); err != nil {
			return false, err
		}
	}
	if !replay {
		_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO errand_result_claim (token, task_id, parent_session_id, route, created_at) VALUES (?, ?, ?, ?, ?)`), token, taskID, parentID, route, at)
	}
	return replay, err
}

func prepareAwaitClaim(ctx context.Context, tx *sqlx.Tx, taskID, parentID, token string, at time.Time) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO errand_await_attempt (token, task_id, parent_session_id, state, created_at, updated_at)
		VALUES (?, ?, ?, 'open', ?, ?) ON CONFLICT (token) DO NOTHING`), token, taskID, parentID, at, at)
	if err != nil {
		return err
	}
	var task, parent, state string
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT task_id, parent_session_id, state FROM errand_await_attempt WHERE token = ?`), token).Scan(&task, &parent, &state); err != nil {
		return err
	}
	if task != taskID || parent != parentID || (state != "open" && state != errandClaimedState) {
		return models.ErrErrandVersionConflict
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_await_attempt SET state = 'claimed', updated_at = ? WHERE token = ? AND state = 'open'`), at, token)
	return err
}

func insertErrandClaimItem(ctx context.Context, tx *sqlx.Tx, row *models.Errand, at time.Time) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO errand_result_claim_item (token, errand_id, result_revision, parent_admission_generation, updated_at)
		VALUES (?, ?, ?, ?, ?)`), row.DeliveryToken, row.ID, row.ResultRevision, row.ParentAdmissionGeneration, at)
	if err != nil || row.DeliveredRoute != models.ErrandDeliveryAwait {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO errand_await_attempt_item (token, errand_id, result_revision, parent_admission_generation)
		VALUES (?, ?, ?, ?)`), row.DeliveryToken, row.ID, row.ResultRevision, row.ParentAdmissionGeneration)
	return err
}
