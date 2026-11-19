package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// ListErrandResultClaims reads immutable ownership, including the generation
// before any accepted successor. Current errand rows cannot reconstruct it.
func (r *Repository) ListErrandResultClaims(ctx context.Context, taskID, parentID, token string) ([]models.ErrandResultClaim, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`SELECT i.errand_id, i.result_revision, i.parent_admission_generation, i.token, c.route
		FROM errand_result_claim_item i JOIN errand_result_claim c ON c.token = i.token
		WHERE c.task_id = ? AND c.parent_session_id = ? AND c.token = ? ORDER BY i.errand_id, i.result_revision`), taskID, parentID, token)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var claims []models.ErrandResultClaim
	for rows.Next() {
		var c models.ErrandResultClaim
		if err := rows.Scan(&c.ErrandID, &c.ResultRevision, &c.ParentAdmissionGeneration, &c.Token, &c.Route); err != nil {
			return nil, err
		}
		claims = append(claims, c)
	}
	return claims, rows.Err()
}

// ReleaseErrandResultClaims records a proved abort against immutable receipts.
// Unresolved admissions defer the release in SQL until their decision commits.
// Await callers must use AbortAwait to fence delayed claims as well.
func (r *Repository) ReleaseErrandResultClaims(ctx context.Context, taskID, parentID string, claims []models.ErrandResultClaim) ([]*models.Errand, error) {
	if taskID == "" || parentID == "" {
		return nil, models.ErrInvalidErrand
	}
	ordered := append([]models.ErrandResultClaim(nil), claims...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ErrandID < ordered[j].ErrandID })
	var changed []*models.Errand
	err := r.withErrandTask(ctx, taskID, func(tx *sqlx.Tx) error {
		for _, claim := range ordered {
			if claim.Route == models.ErrandDeliveryAwait {
				return models.ErrInvalidErrand
			}
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

func releaseErrandResultClaim(ctx context.Context, tx *sqlx.Tx, taskID, parentID string, claim models.ErrandResultClaim, at time.Time) (*models.Errand, error) {
	var state string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT i.state FROM errand_result_claim_item i JOIN errand_result_claim c ON c.token = i.token
		WHERE c.task_id = ? AND c.parent_session_id = ? AND c.route = ? AND i.token = ? AND i.errand_id = ?
		AND i.result_revision = ? AND i.parent_admission_generation = ?`), taskID, parentID, claim.Route, claim.Token,
		claim.ErrandID, claim.ResultRevision, claim.ParentAdmissionGeneration).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil || (state != errandClaimedState && state != "release_pending") {
		return nil, err
	}
	row, err := scanErrand(tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+errandColumns+` FROM task_session_errands WHERE id = ?`), claim.ErrandID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, setErrandClaimState(ctx, tx, claim, "superseded", at)
	}
	if err != nil {
		return nil, err
	}
	if !errandMatchesClaim(row, taskID, parentID, claim) {
		return nil, setErrandClaimState(ctx, tx, claim, "superseded", at)
	}
	if row.ParentAdmissionGeneration != claim.ParentAdmissionGeneration {
		row.ResultState = models.ErrandResultSuperseded
		state = "superseded"
	} else {
		pending, err := unresolvedErrandAdmission(ctx, tx, row.ID)
		if err != nil {
			return nil, err
		}
		if pending {
			return nil, setErrandClaimState(ctx, tx, claim, "release_pending", at)
		}
		row.ResultState = models.ErrandResultPending
		state = "released"
	}
	if err := writeReleasedErrand(ctx, tx, row, at); err != nil {
		return nil, err
	}
	if err := setErrandClaimState(ctx, tx, claim, state, at); err != nil {
		return nil, err
	}
	return row, nil
}

func errandMatchesClaim(row *models.Errand, taskID, parentID string, claim models.ErrandResultClaim) bool {
	return row.TaskID == taskID && row.ParentSessionID == parentID && row.ResultRevision == claim.ResultRevision &&
		row.DeliveryToken == claim.Token && row.DeliveredRoute == claim.Route && row.ResultState == models.ErrandResultDelivered
}

func writeReleasedErrand(ctx context.Context, tx *sqlx.Tx, row *models.Errand, at time.Time) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_session_errands SET result_state = ?, delivered_route = '',
		delivery_token = '', delivered_at = NULL, version = version + 1, updated_at = ? WHERE id = ?`), row.ResultState, at, row.ID)
	if err != nil {
		return err
	}
	row.DeliveredRoute, row.DeliveryToken, row.DeliveredAt = "", "", nil
	row.Version++
	row.UpdatedAt = at
	return nil
}

func setErrandClaimState(ctx context.Context, tx *sqlx.Tx, claim models.ErrandResultClaim, state string, at time.Time) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_result_claim_item SET state = ?, updated_at = ?
		WHERE token = ? AND errand_id = ? AND result_revision = ? AND parent_admission_generation = ?`),
		state, at, claim.Token, claim.ErrandID, claim.ResultRevision, claim.ParentAdmissionGeneration)
	return err
}

// Generic CAS callers cannot bypass delivery ownership. Await release requires
// an already committed abort; other routes preserve their durable abort intent.
func legacyErrandRelease(ctx context.Context, tx *sqlx.Tx, current *models.Errand, at time.Time) (*models.Errand, error) {
	if current.DeliveredRoute == models.ErrandDeliveryAwait {
		var aborted bool
		if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM errand_await_attempt
			WHERE token = ? AND task_id = ? AND parent_session_id = ? AND state = 'aborted')`),
			current.DeliveryToken, current.TaskID, current.ParentSessionID).Scan(&aborted); err != nil {
			return nil, err
		}
		if !aborted {
			return current, nil
		}
	}
	var generation int64
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT parent_admission_generation FROM errand_result_claim_item
		WHERE token = ? AND errand_id = ? AND result_revision = ?`), current.DeliveryToken, current.ID, current.ResultRevision).Scan(&generation)
	if errors.Is(err, sql.ErrNoRows) {
		return current, nil
	}
	if err != nil {
		return nil, err
	}
	claim := models.ErrandResultClaim{ErrandID: current.ID, ResultRevision: current.ResultRevision,
		ParentAdmissionGeneration: generation, Token: current.DeliveryToken, Route: current.DeliveredRoute}
	row, err := releaseErrandResultClaim(ctx, tx, current.TaskID, current.ParentSessionID, claim, at)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return current, nil
	}
	return row, nil
}

func retryErrandDeferredReleases(ctx context.Context, tx *sqlx.Tx, taskID, parentID, errandID string, at time.Time) error {
	claims, err := readErrandReleaseClaims(ctx, tx, taskID, parentID, "i.errand_id = ? AND i.state = 'release_pending'", errandID)
	if err != nil {
		return err
	}
	for _, claim := range claims {
		if _, err := releaseErrandResultClaim(ctx, tx, taskID, parentID, claim, at); err != nil {
			return err
		}
	}
	return nil
}

func readErrandReleaseClaims(ctx context.Context, tx *sqlx.Tx, taskID, parentID, predicate string, args ...any) ([]models.ErrandResultClaim, error) {
	params := append([]any{taskID, parentID}, args...)
	rows, err := tx.QueryContext(ctx, tx.Rebind(`SELECT i.errand_id, i.result_revision, i.parent_admission_generation, i.token, c.route
		FROM errand_result_claim_item i JOIN errand_result_claim c ON c.token = i.token
		WHERE c.task_id = ? AND c.parent_session_id = ? AND `+predicate+` ORDER BY i.errand_id, i.token, i.result_revision`), params...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var claims []models.ErrandResultClaim
	for rows.Next() {
		var c models.ErrandResultClaim
		if err := rows.Scan(&c.ErrandID, &c.ResultRevision, &c.ParentAdmissionGeneration, &c.Token, &c.Route); err != nil {
			return nil, err
		}
		claims = append(claims, c)
	}
	return claims, rows.Err()
}
