package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

type errandClaimSnapshot struct {
	Candidates []models.ErrandClaimCandidate
	Inventory  []models.ErrandClaimCandidate
	Rows       []*models.Errand
}

func orderedErrandCandidates(input []models.ErrandClaimCandidate) ([]models.ErrandClaimCandidate, error) {
	rows := append([]models.ErrandClaimCandidate(nil), input...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	for i, row := range rows {
		if row.ID == "" || row.Version < 1 || (i > 0 && rows[i-1].ID == row.ID) {
			return nil, models.ErrInvalidErrand
		}
	}
	return rows, nil
}

func validateErrandInventory(ctx context.Context, tx *sqlx.Tx, task, parent string, inventory []models.ErrandClaimCandidate) error {
	var current []models.ErrandClaimCandidate
	if err := tx.SelectContext(ctx, &current, tx.Rebind(`SELECT id, version FROM task_session_errands
		WHERE task_id = ? AND parent_session_id = ? ORDER BY id`), task, parent); err != nil {
		return err
	}
	if !slices.Equal(current, inventory) {
		return models.ErrErrandVersionConflict
	}
	return nil
}

func saveErrandClaimSnapshot(ctx context.Context, tx *sqlx.Tx, token string, candidates, inventory []models.ErrandClaimCandidate, rows []*models.Errand) error {
	snapshot, err := json.Marshal(errandClaimSnapshot{Candidates: candidates, Inventory: inventory, Rows: rows})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_result_claim SET snapshot = ? WHERE token = ? AND snapshot = ''`), string(snapshot), token)
	return err
}

func readErrandClaimSnapshot(ctx context.Context, tx *sqlx.Tx, task, parent string, route models.ErrandDeliveryRoute, token string, candidates, inventory []models.ErrandClaimCandidate) ([]*models.Errand, error) {
	var encoded string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT snapshot FROM errand_result_claim
		WHERE token = ? AND task_id = ? AND parent_session_id = ? AND route = ?`), token, task, parent, route).Scan(&encoded)
	if err != nil {
		return nil, err
	}
	var snapshot errandClaimSnapshot
	if encoded == "" || json.Unmarshal([]byte(encoded), &snapshot) != nil || !slices.Equal(snapshot.Candidates, candidates) ||
		!slices.Equal(snapshot.Inventory, inventory) || len(snapshot.Rows) != len(candidates) {
		return nil, models.ErrErrandVersionConflict
	}
	var count int
	if err := tx.GetContext(ctx, &count, tx.Rebind(`SELECT COUNT(*) FROM errand_result_claim_item WHERE token = ? AND state = 'claimed'`), token); err != nil {
		return nil, err
	}
	if count != len(candidates) {
		return nil, models.ErrErrandVersionConflict
	}
	if route == models.ErrandDeliveryAwait {
		var state string
		if err := tx.GetContext(ctx, &state, tx.Rebind(`SELECT state FROM errand_await_attempt WHERE token = ?`), token); err != nil {
			return nil, err
		}
		if state != errandClaimedState {
			return nil, models.ErrErrandVersionConflict
		}
	}
	return snapshot.Rows, nil
}

// A commit error cannot authorize a second claim or an empty success. Read the
// immutable receipt on the writer using the original token, independently of
// cancellation of the call whose commit outcome was lost.
func (r *Repository) readBackErrandClaim(ctx context.Context, task, parent string, route models.ErrandDeliveryRoute, token string, candidates, inventory []models.ErrandClaimCandidate, commitErr error) ([]*models.Errand, error) {
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	snapshot, err := r.readCommittedErrandClaim(readCtx, task, parent, route, token, candidates, inventory)
	if err != nil {
		return nil, fmt.Errorf("%w (token %s): commit: %v; readback: %v", models.ErrErrandClaimCommitUnknown, token, commitErr, err)
	}
	return snapshot, nil
}

func (r *Repository) readCommittedErrandClaim(ctx context.Context, task, parent string, route models.ErrandDeliveryRoute, token string, candidates, inventory []models.ErrandClaimCandidate) ([]*models.Errand, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockErrandTask(ctx, tx, task); err != nil {
		return nil, err
	}
	return readErrandClaimSnapshot(ctx, tx, task, parent, route, token, candidates, inventory)
}
