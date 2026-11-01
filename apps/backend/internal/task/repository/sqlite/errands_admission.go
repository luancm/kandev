package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

// ListUnresolvedErrandParentAdmissions retains the original sender, turn and
// submission for recovery. An empty errandID selects startup's full inventory.
func (r *Repository) ListUnresolvedErrandParentAdmissions(ctx context.Context, errandID string) ([]models.ErrandParentAdmission, error) {
	query := `SELECT task_id, parent_session_id, errand_id, sender_session_id, turn_id, submission_id
		FROM errand_parent_admission WHERE state IN ('reserved', 'unknown')`
	var args []interface{}
	if errandID != "" {
		query += " AND errand_id = ?"
		args = append(args, errandID)
	}
	query += " ORDER BY errand_id, submission_id"
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var admissions []models.ErrandParentAdmission
	for rows.Next() {
		var a models.ErrandParentAdmission
		if err := rows.Scan(&a.TaskID, &a.ParentSessionID, &a.ErrandID, &a.SenderSessionID, &a.TurnID, &a.SubmissionID); err != nil {
			return nil, err
		}
		admissions = append(admissions, a)
	}
	return admissions, rows.Err()
}

// ReserveErrandParentAdmission must commit before dispatch. Reusing the exact
// turn/submission is idempotent; a different unresolved dispatch is fenced.
func (r *Repository) ReserveErrandParentAdmission(ctx context.Context, admission models.ErrandParentAdmission) (models.ErrandAdmissionReservation, error) {
	if !validErrandAdmission(admission) {
		return models.ErrandAdmissionReservation{}, models.ErrInvalidErrand
	}
	var reservation models.ErrandAdmissionReservation
	err := r.withErrandTask(ctx, admission.TaskID, func(tx *sqlx.Tx) error {
		if err := db.RequireErrandTurnSettlements(ctx, tx, admission.ErrandID, admission.TurnID); err != nil {
			return err
		}
		row, err := admissionErrand(ctx, tx, admission)
		if err != nil {
			return err
		}
		state, err := readErrandAdmission(ctx, tx, admission)
		if err != nil {
			return err
		}
		if state != "" {
			reservation.State = models.ErrandAdmissionState(state)
			return nil
		}
		if admission.SenderSessionID != row.ParentSessionID {
			return nil
		}
		if row.Lifecycle == models.ErrandLifecycleCancelled {
			return models.ErrErrandVersionConflict
		}
		pending, err := unresolvedErrandAdmission(ctx, tx, row.ID)
		if err != nil {
			return err
		}
		if pending {
			return models.ErrErrandVersionConflict
		}
		_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO errand_parent_admission
			(task_id, parent_session_id, errand_id, sender_session_id, turn_id, submission_id, state, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 'reserved', ?, ?)`), admission.TaskID, admission.ParentSessionID, admission.ErrandID,
			admission.SenderSessionID, admission.TurnID, admission.SubmissionID, r.nowUTC(), r.nowUTC())
		reservation = models.ErrandAdmissionReservation{Created: true, State: models.ErrandAdmissionReserved}
		return err
	})
	if err != nil {
		return models.ErrandAdmissionReservation{}, err
	}
	return reservation, nil
}

// DecideErrandParentAdmission consumes only positive acceptance or definite
// refusal evidence for the reserved identity. Unknown never clears the fence.
func (r *Repository) DecideErrandParentAdmission(ctx context.Context, admission models.ErrandParentAdmission, decision models.ErrandAdmissionState) error {
	if !validErrandAdmission(admission) || !validErrandAdmissionDecision(decision) {
		return models.ErrInvalidErrand
	}
	return r.withErrandTask(ctx, admission.TaskID, func(tx *sqlx.Tx) error {
		row, err := admissionErrand(ctx, tx, admission)
		if err != nil {
			return err
		}
		state, err := readErrandAdmission(ctx, tx, admission)
		if err != nil {
			return err
		}
		if admission.SenderSessionID != row.ParentSessionID {
			return nil
		}
		if state == "" {
			return models.ErrErrandVersionConflict
		}
		if state == string(models.ErrandAdmissionAccepted) || state == string(models.ErrandAdmissionRefused) {
			if state != string(decision) {
				return models.ErrErrandVersionConflict
			}
			return retryErrandDeferredReleases(ctx, tx, admission.TaskID, admission.ParentSessionID, admission.ErrandID, r.nowUTC())
		}
		if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_parent_admission SET state = ?, updated_at = ?
			WHERE errand_id = ? AND submission_id = ?`), decision, r.nowUTC(), admission.ErrandID, admission.SubmissionID); err != nil {
			return err
		}
		if decision == models.ErrandAdmissionUnknown {
			return nil
		}
		if decision == models.ErrandAdmissionAccepted {
			if err := acceptErrandParentAdmission(ctx, tx, row, admission.TurnID, r.nowUTC()); err != nil {
				return err
			}
		}
		return retryErrandDeferredReleases(ctx, tx, admission.TaskID, admission.ParentSessionID, admission.ErrandID, r.nowUTC())
	})
}

func validErrandAdmission(a models.ErrandParentAdmission) bool {
	return a.TaskID != "" && a.ParentSessionID != "" && a.ErrandID != "" && a.TurnID != "" && a.SubmissionID != ""
}

func validErrandAdmissionDecision(d models.ErrandAdmissionState) bool {
	return d == models.ErrandAdmissionAccepted || d == models.ErrandAdmissionRefused || d == models.ErrandAdmissionUnknown
}

func admissionErrand(ctx context.Context, tx *sqlx.Tx, a models.ErrandParentAdmission) (*models.Errand, error) {
	row, err := scanErrand(tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+errandColumns+` FROM task_session_errands WHERE id = ?`), a.ErrandID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.ErrErrandNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.TaskID != a.TaskID || row.ParentSessionID != a.ParentSessionID {
		return nil, models.ErrInvalidErrand
	}
	return row, nil
}

func readErrandAdmission(ctx context.Context, tx *sqlx.Tx, a models.ErrandParentAdmission) (string, error) {
	var task, parent, sender, turn, state string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT task_id, parent_session_id, sender_session_id, turn_id, state
		FROM errand_parent_admission WHERE errand_id = ? AND submission_id = ?`), a.ErrandID, a.SubmissionID).Scan(&task, &parent, &sender, &turn, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if task != a.TaskID || parent != a.ParentSessionID || sender != a.SenderSessionID || turn != a.TurnID {
		return "", models.ErrErrandVersionConflict
	}
	return state, nil
}

func unresolvedErrandAdmission(ctx context.Context, tx *sqlx.Tx, errandID string) (bool, error) {
	var pending bool
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM errand_parent_admission
		WHERE errand_id = ? AND state IN ('reserved', 'unknown'))`), errandID).Scan(&pending)
	return pending, err
}

func acceptErrandParentAdmission(ctx context.Context, tx *sqlx.Tx, row *models.Errand, turnID string, at time.Time) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_session_errands SET parent_admission_generation = parent_admission_generation + 1,
		awaiting_result = CASE WHEN lifecycle = 'cancelled' OR latest_settled_turn_id = ? THEN awaiting_result ELSE TRUE END,
		result_state = CASE WHEN lifecycle <> 'cancelled' AND result_state = 'pending' AND result_turn_id <> ? THEN 'superseded' ELSE result_state END,
		updated_at = ?, version = version + 1 WHERE id = ?`), turnID, turnID, at, row.ID)
	return err
}
