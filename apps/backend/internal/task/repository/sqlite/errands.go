package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

const errandColumns = `id, task_id, parent_session_id, title, objective, agent_profile_id, mcp_server_ids,
	first_turn_id, submission_id, lifecycle, cancel_source, last_outcome, awaiting_result,
	result_text, result_truncated, result_turn_id, result_kind, result_state,
	delivered_route, delivery_token, delivered_at, latest_settled_turn_id,
	version, created_at, updated_at, settled_at`

// liveErrandPredicate selects the errands whose projected status is queued,
// running, or needs_attention: queued rows, and launched rows whose session
// exists, has not failed, and is either working or has not yet settled a
// parent-initiated turn.
const liveErrandPredicate = `(e.lifecycle = 'queued' OR (
	e.lifecycle = 'launched' AND s.id IS NOT NULL AND s.state <> 'FAILED'
	AND (s.state IN ('STARTING', 'RUNNING') OR e.last_outcome = '')))`

type errandRowScanner interface {
	Scan(dest ...interface{}) error
}

// CreateErrand inserts a new errand row at version 1. Missing timestamps
// default to the repository clock.
func (r *Repository) CreateErrand(ctx context.Context, errand *models.Errand) error {
	if errand == nil {
		return fmt.Errorf("%w: errand is required", models.ErrInvalidErrand)
	}
	if err := errand.Validate(); err != nil {
		return err
	}
	mcpServerIDs, err := encodeErrandMCPServerIDs(errand.MCPServerIDs)
	if err != nil {
		return err
	}
	row := *errand
	row.Version = 1
	if row.CreatedAt.IsZero() {
		row.CreatedAt = r.nowUTC()
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = row.CreatedAt
	}
	row.CreatedAt, row.UpdatedAt = row.CreatedAt.UTC(), row.UpdatedAt.UTC()
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO task_session_errands (`+errandColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		row.ID, row.TaskID, row.ParentSessionID, row.Title, row.Objective, row.AgentProfileID, mcpServerIDs,
		row.FirstTurnID, row.SubmissionID, string(row.Lifecycle), string(row.CancelSource), string(row.LastOutcome),
		row.AwaitingResult, row.ResultText, row.ResultTruncated, row.ResultTurnID, string(row.ResultKind),
		string(row.ResultState), string(row.DeliveredRoute), row.DeliveryToken, utcTimeArg(row.DeliveredAt),
		row.LatestSettledTurnID, row.Version, row.CreatedAt, row.UpdatedAt, utcTimeArg(row.SettledAt))
	if err != nil {
		return fmt.Errorf("insert errand %s: %w", row.ID, err)
	}
	*errand = row
	return nil
}

// GetErrand returns one errand row or models.ErrErrandNotFound.
func (r *Repository) GetErrand(ctx context.Context, id string) (*models.Errand, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT `+errandColumns+` FROM task_session_errands WHERE id = ?`), id)
	errand, err := scanErrand(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", models.ErrErrandNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get errand %s: %w", id, err)
	}
	return errand, nil
}

// ListErrandsByTask returns a task's errands in creation order.
func (r *Repository) ListErrandsByTask(ctx context.Context, taskID string) ([]*models.Errand, error) {
	return r.listErrands(ctx, `task_id = ? ORDER BY created_at, id`, taskID)
}

// ListErrandsByParent returns a parent session's errands in creation order.
func (r *Repository) ListErrandsByParent(ctx context.Context, parentSessionID string) ([]*models.Errand, error) {
	return r.listErrands(ctx, `parent_session_id = ? ORDER BY created_at, id`, parentSessionID)
}

// ListErrands returns every errand row. Reconciliation and the delivery
// sweep use it; callers that need one parent or task use the filtered lists.
func (r *Repository) ListErrands(ctx context.Context) ([]*models.Errand, error) {
	return r.listErrands(ctx, "1 = 1")
}

// ListQueuedErrands returns every queued errand grouped by task and in
// creation order within a task.

func (r *Repository) ListQueuedErrands(ctx context.Context) ([]*models.Errand, error) {
	return r.listErrands(ctx, `lifecycle = 'queued' ORDER BY task_id, created_at, id`)
}

// CountLiveErrandsByTask counts a task's errands whose projected status is
// queued, running, or needs_attention.
func (r *Repository) CountLiveErrandsByTask(ctx context.Context, taskID string) (int, error) {
	var count int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT COUNT(*) FROM task_session_errands e
		LEFT JOIN task_sessions s ON s.id = e.id
		WHERE e.task_id = ? AND `+liveErrandPredicate), taskID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count live errands for task %s: %w", taskID, err)
	}
	return count, nil
}

// UpdateErrand writes the mutable errand fields only while the stored version
// still equals errand.Version. On success errand.Version and UpdatedAt carry
// the committed values. A stale version returns
// models.ErrErrandVersionConflict and changes nothing.
func (r *Repository) UpdateErrand(ctx context.Context, errand *models.Errand) error {
	if errand == nil {
		return fmt.Errorf("%w: errand is required", models.ErrInvalidErrand)
	}
	if err := errand.Validate(); err != nil {
		return err
	}
	updatedAt := r.nowUTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE task_session_errands SET
			lifecycle = ?, cancel_source = ?, last_outcome = ?, awaiting_result = ?,
			result_text = ?, result_truncated = ?, result_turn_id = ?, result_kind = ?, result_state = ?,
			delivered_route = ?, delivery_token = ?, delivered_at = ?, latest_settled_turn_id = ?,
			settled_at = ?, updated_at = ?, version = version + 1
		WHERE id = ? AND version = ?`),
		string(errand.Lifecycle), string(errand.CancelSource), string(errand.LastOutcome), errand.AwaitingResult,
		errand.ResultText, errand.ResultTruncated, errand.ResultTurnID, string(errand.ResultKind),
		string(errand.ResultState), string(errand.DeliveredRoute), errand.DeliveryToken, utcTimeArg(errand.DeliveredAt),
		errand.LatestSettledTurnID, utcTimeArg(errand.SettledAt), updatedAt, errand.ID, errand.Version)
	if err != nil {
		return fmt.Errorf("update errand %s: %w", errand.ID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update errand %s: %w", errand.ID, err)
	}
	if rows == 0 {
		return r.classifyErrandUpdateMiss(ctx, errand.ID)
	}
	errand.Version++
	errand.UpdatedAt = updatedAt
	return nil
}

// DeleteErrand removes an errand row. Deleting a missing row is not an error.
func (r *Repository) DeleteErrand(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_session_errands WHERE id = ?`), id); err != nil {
		return fmt.Errorf("delete errand %s: %w", id, err)
	}
	return nil
}

func deleteErrandForSessionTx(ctx context.Context, tx *sqlx.Tx, sessionID string) error {
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM task_session_errands WHERE id = ?`), sessionID); err != nil {
		return fmt.Errorf("delete errand for session %s: %w", sessionID, err)
	}
	return nil
}

func (r *Repository) classifyErrandUpdateMiss(ctx context.Context, id string) error {
	var exists bool
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT EXISTS (SELECT 1 FROM task_session_errands WHERE id = ?)`), id).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check errand %s: %w", id, err)
	}
	if !exists {
		return fmt.Errorf("%w: %s", models.ErrErrandNotFound, id)
	}
	return fmt.Errorf("%w: %s", models.ErrErrandVersionConflict, id)
}

func (r *Repository) listErrands(ctx context.Context, condition string, args ...interface{}) ([]*models.Errand, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`SELECT `+errandColumns+` FROM task_session_errands WHERE `+condition), args...)
	if err != nil {
		return nil, fmt.Errorf("list errands: %w", err)
	}
	defer func() { _ = rows.Close() }()
	errands := []*models.Errand{}
	for rows.Next() {
		errand, err := scanErrand(rows)
		if err != nil {
			return nil, fmt.Errorf("scan errand: %w", err)
		}
		errands = append(errands, errand)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate errands: %w", err)
	}
	return errands, nil
}

func scanErrand(row errandRowScanner) (*models.Errand, error) {
	var errand models.Errand
	var mcpServerIDs string
	var deliveredAt, settledAt sql.NullTime
	if err := row.Scan(
		&errand.ID, &errand.TaskID, &errand.ParentSessionID, &errand.Title, &errand.Objective,
		&errand.AgentProfileID, &mcpServerIDs, &errand.FirstTurnID, &errand.SubmissionID,
		&errand.Lifecycle, &errand.CancelSource, &errand.LastOutcome, &errand.AwaitingResult,
		&errand.ResultText, &errand.ResultTruncated, &errand.ResultTurnID, &errand.ResultKind, &errand.ResultState,
		&errand.DeliveredRoute, &errand.DeliveryToken, &deliveredAt, &errand.LatestSettledTurnID,
		&errand.Version, &errand.CreatedAt, &errand.UpdatedAt, &settledAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(mcpServerIDs), &errand.MCPServerIDs); err != nil {
		return nil, fmt.Errorf("decode errand %s MCP servers: %w", errand.ID, err)
	}
	if len(errand.MCPServerIDs) == 0 {
		errand.MCPServerIDs = nil
	}
	errand.CreatedAt, errand.UpdatedAt = errand.CreatedAt.UTC(), errand.UpdatedAt.UTC()
	errand.DeliveredAt = utcTimeFromNull(deliveredAt)
	errand.SettledAt = utcTimeFromNull(settledAt)
	return &errand, nil
}

func encodeErrandMCPServerIDs(ids []string) (string, error) {
	if len(ids) == 0 {
		return "[]", nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return "", fmt.Errorf("encode errand MCP servers: %w", err)
	}
	return string(encoded), nil
}

func utcTimeArg(value *time.Time) interface{} {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func utcTimeFromNull(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	at := value.Time.UTC()
	return &at
}
