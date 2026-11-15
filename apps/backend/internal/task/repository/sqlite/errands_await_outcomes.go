package sqlite

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

const errandAwaitOpenState = "open"

func (r *Repository) BeginErrandAwait(ctx context.Context, id models.ErrandAwaitIdentity) error {
	if !id.Valid() {
		return models.ErrInvalidErrand
	}
	return r.withErrandTask(ctx, id.TaskID, func(tx *sqlx.Tx) error {
		state, err := bindErrandAwait(ctx, tx, id, r.nowUTC())
		if err != nil {
			return err
		}
		if state != errandAwaitOpenState {
			return models.ErrErrandVersionConflict
		}
		return nil
	})
}

func bindErrandAwait(ctx context.Context, tx *sqlx.Tx, id models.ErrandAwaitIdentity, at time.Time) (string, error) {
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO errand_await_attempt
 (token,task_id,parent_session_id,state,parent_incarnation_id,harness_generation,turn_id,submission_id,created_at,updated_at)
 VALUES (?,?,?,'open',?,?,?,?,?,?) ON CONFLICT (token) DO NOTHING`), id.Token, id.TaskID, id.ParentSessionID, id.ParentIncarnationID, id.HarnessGeneration, id.TurnID, id.SubmissionID, at, at)
	if err != nil {
		return "", err
	}
	return checkErrandAwaitOwner(ctx, tx, id)
}

func checkErrandAwaitOwner(ctx context.Context, tx *sqlx.Tx, id models.ErrandAwaitIdentity) (string, error) {
	var stored models.ErrandAwaitIdentity
	var state string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT token,task_id,parent_session_id,parent_incarnation_id,harness_generation,turn_id,submission_id,state FROM errand_await_attempt WHERE token=?`), id.Token).
		Scan(&stored.Token, &stored.TaskID, &stored.ParentSessionID, &stored.ParentIncarnationID, &stored.HarnessGeneration, &stored.TurnID, &stored.SubmissionID, &state)
	if err != nil {
		return "", err
	}
	if stored != id {
		return "", models.ErrErrandVersionConflict
	}
	return state, nil
}

// OfferErrandAwait authorizes handing off an assembled successful body. A
// commit error is returned even if the write committed; it is not a receipt.
func (r *Repository) OfferErrandAwait(ctx context.Context, id models.ErrandAwaitIdentity, items []models.ErrandAwaitDeliveryItem) error {
	if !id.Valid() || len(items) > 5 {
		return models.ErrInvalidErrand
	}
	return r.withErrandTask(ctx, id.TaskID, func(tx *sqlx.Tx) error {
		state, err := checkErrandAwaitOwner(ctx, tx, id)
		if err != nil {
			return err
		}
		if state != errandAwaitOpenState && state != "claimed" && state != errandAwaitOfferedState {
			return models.ErrErrandVersionConflict
		}
		if err := matchErrandAwaitItems(ctx, tx, id.Token, items); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_await_attempt SET state='offered',updated_at=? WHERE token=?`), r.nowUTC(), id.Token)
		return err
	})
}

func matchErrandAwaitItems(ctx context.Context, tx *sqlx.Tx, token string, items []models.ErrandAwaitDeliveryItem) error {
	rows, err := tx.QueryContext(ctx, tx.Rebind(`SELECT errand_id,result_revision FROM errand_await_attempt_item WHERE token=? ORDER BY errand_id,result_revision`), token)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	actual := make([]models.ErrandAwaitDeliveryItem, 0, 5)
	for rows.Next() {
		var item models.ErrandAwaitDeliveryItem
		if err := rows.Scan(&item.ErrandID, &item.ResultRevision); err != nil {
			return err
		}
		actual = append(actual, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	wanted := slices.Clone(items)
	slices.SortFunc(wanted, func(a, b models.ErrandAwaitDeliveryItem) int {
		if a.ErrandID < b.ErrandID {
			return -1
		}
		if a.ErrandID > b.ErrandID {
			return 1
		}
		return 0
	})
	if !slices.Equal(actual, wanted) {
		return fmt.Errorf("await response revisions differ: %w", models.ErrErrandVersionConflict)
	}
	return nil
}

// AbortErrandAwait binds an early tombstone to the exact invocation before
// releasing claims. Received is terminal through abortAwaitAttempt.
func (r *Repository) AbortErrandAwait(ctx context.Context, id models.ErrandAwaitIdentity) ([]*models.Errand, error) {
	if !id.Valid() {
		return nil, models.ErrInvalidErrand
	}
	return r.abortAwait(ctx, id.TaskID, id.ParentSessionID, id.Token, &id)
}
