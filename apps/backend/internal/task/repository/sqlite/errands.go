package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

const errandColumns = `id, task_id, parent_session_id, title, objective, agent_profile_id, mcp_server_ids,
	first_turn_id, submission_id, lifecycle, cancel_source, last_outcome, awaiting_result,
	result_text, result_truncated, result_turn_id, result_kind, result_state,
	delivered_route, delivery_token, delivered_at, latest_settled_turn_id,
	version, created_at, updated_at, settled_at, result_revision, parent_admission_generation, teardown_intent`

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
	if errand.TeardownIntent != nil {
		return fmt.Errorf("%w: teardown intent must be committed after errand creation", models.ErrInvalidErrand)
	}
	if errand.ParentAdmissionGeneration != 0 {
		return models.ErrInvalidErrand
	}
	mcpServerIDs, err := encodeErrandMCPSnapshot(errand)
	if err != nil {
		return err
	}
	teardownIntent, err := encodeErrandTeardownIntent(errand)
	if err != nil {
		return err
	}
	row := *errand
	row.Version = 1
	if row.ResultText != "" || row.ResultKind != "" {
		row.ResultRevision = uuid.NewString()
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = r.nowUTC()
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = row.CreatedAt
	}
	row.CreatedAt, row.UpdatedAt = row.CreatedAt.UTC(), row.UpdatedAt.UTC()
	err = r.withErrandTask(ctx, row.TaskID, func(tx *sqlx.Tx) error {
		_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO task_session_errands (`+errandColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			row.ID, row.TaskID, row.ParentSessionID, row.Title, row.Objective, row.AgentProfileID, mcpServerIDs,
			row.FirstTurnID, row.SubmissionID, string(row.Lifecycle), string(row.CancelSource), string(row.LastOutcome),
			row.AwaitingResult, row.ResultText, row.ResultTruncated, row.ResultTurnID, string(row.ResultKind),
			string(row.ResultState), string(row.DeliveredRoute), row.DeliveryToken, utcTimeArg(row.DeliveredAt),
			row.LatestSettledTurnID, row.Version, row.CreatedAt, row.UpdatedAt, utcTimeArg(row.SettledAt), row.ResultRevision, row.ParentAdmissionGeneration, teardownIntent)
		return err
	})
	if err != nil {
		return fmt.Errorf("insert errand %s: %w", row.ID, err)
	}
	*errand = row
	return nil
}

// GetErrand returns one errand row or models.ErrErrandNotFound.
func (r *Repository) GetErrand(ctx context.Context, id string) (*models.Errand, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT `+errandColumns+`, `+errandAwaitProjection+` FROM task_session_errands WHERE id = ?`), id)
	errand, err := scanProjectedErrand(row)
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
		WHERE e.task_id = ? AND `+liveErrandPredicate(r.ro.DriverName())), taskID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count live errands for task %s: %w", taskID, err)
	}
	return count, nil
}

// outstandingErrandPredicate mirrors errands.IsOutstanding: a queued errand,
// one awaiting a parent-initiated turn, or one holding a pending result.
func outstandingErrandPredicate(driver string) string {
	return `(e.lifecycle = 'queued' OR e.awaiting_result OR e.result_state = 'pending' OR ` + recoveryAttentionPredicate(driver) + `)`
}

// ErrandsSummaryByTask returns the bounded errand projection for one task. It
// is a single aggregate over the task's errand rows, joined by primary key to
// each errand's session and parent, so its size does not depend on how many
// errands the task has. Waiting is true when an idle parent owes an outcome.
func (r *Repository) ErrandsSummaryByTask(ctx context.Context, taskID string) (statussummary.ErrandsSummary, error) {
	var live, waiting int64
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT
		COALESCE(SUM(CASE WHEN `+liveErrandPredicate(r.ro.DriverName())+` THEN 1 ELSE 0 END), 0),
		COALESCE(MAX(CASE WHEN ps.state = ? AND `+outstandingErrandPredicate(r.ro.DriverName())+` THEN 1 ELSE 0 END), 0)
		FROM task_session_errands e
		LEFT JOIN task_sessions s ON s.id = e.id
		LEFT JOIN task_sessions ps ON ps.id = e.parent_session_id
		WHERE e.task_id = ?`), string(models.TaskSessionStateWaitingForInput), taskID).Scan(&live, &waiting)
	if err != nil {
		return statussummary.ErrandsSummary{}, fmt.Errorf("summarize errands for task %s: %w", taskID, err)
	}
	return statussummary.ErrandsSummary{Live: int(live), Waiting: waiting > 0}, nil
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
	taskID, err := r.errandTaskID(ctx, errand.ID)
	if err != nil {
		return err
	}
	var updated *models.Errand
	err = r.withErrandTask(ctx, taskID, func(tx *sqlx.Tx) error {
		current, err := currentErrandForUpdate(ctx, tx, errand)
		if err != nil {
			return err
		}
		if err := validateErrandTeardownUpdate(current, errand); err != nil {
			return err
		}
		revision := errandUpdateRevision(current, errand)
		if current.ResultState == models.ErrandResultDelivered && errand.ResultState == models.ErrandResultPending && revision == current.ResultRevision {
			updated, err = legacyErrandRelease(ctx, tx, current, r.nowUTC())
			return err
		}
		row := *errand
		preserveErrandIdentity(&row, current)
		row.ResultRevision, row.UpdatedAt = revision, r.nowUTC()
		if err := writeErrandUpdate(ctx, tx, &row); err != nil {
			return err
		}
		if err := readWrittenErrandDeliveryState(ctx, tx, &row); err != nil {
			return err
		}
		row.Version++
		updated = &row
		return nil
	})
	if err != nil {
		return err
	}
	*errand = *updated
	return nil
}

// TransitionErrandTeardownIntent settles one pending intent without reloading
// or replacing any other mutable errand state. Callers re-read after conflicts.
func (r *Repository) TransitionErrandTeardownIntent(
	ctx context.Context,
	errandID string,
	expectedIntentID string,
	expectedVersion int64,
	state models.ErrandTeardownIntentState,
) (*models.Errand, error) {
	if errandID == "" || expectedIntentID == "" || expectedVersion < 1 ||
		(state != models.ErrandTeardownIntentClosed && state != models.ErrandTeardownIntentCompleted && state != models.ErrandTeardownIntentSuperseded) {
		return nil, models.ErrInvalidErrand
	}
	taskID, err := r.errandTaskID(ctx, errandID)
	if err != nil {
		return nil, err
	}
	var updated *models.Errand
	err = r.withErrandTask(ctx, taskID, func(tx *sqlx.Tx) error {
		var transitionErr error
		updated, transitionErr = r.transitionErrandTeardownIntent(ctx, tx, errandID, expectedIntentID, expectedVersion, state)
		return transitionErr
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (r *Repository) transitionErrandTeardownIntent(
	ctx context.Context,
	tx *sqlx.Tx,
	errandID string,
	expectedIntentID string,
	expectedVersion int64,
	state models.ErrandTeardownIntentState,
) (*models.Errand, error) {
	current, err := scanErrand(tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+errandColumns+` FROM task_session_errands WHERE id = ?`), errandID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.ErrErrandNotFound
	}
	if err != nil {
		return nil, err
	}
	if current.Version != expectedVersion || current.TeardownIntent == nil || current.TeardownIntent.ID != expectedIntentID ||
		!validErrandTeardownTransition(current.TeardownIntent, state) {
		return nil, models.ErrErrandVersionConflict
	}
	row := *current
	intent := *current.TeardownIntent
	intent.State = state
	row.TeardownIntent = &intent
	row.UpdatedAt = r.nowUTC()
	if err := row.Validate(); err != nil {
		return nil, err
	}
	if err := writeErrandUpdate(ctx, tx, &row); err != nil {
		return nil, err
	}
	if err := readWrittenErrandDeliveryState(ctx, tx, &row); err != nil {
		return nil, err
	}
	row.Version++
	return &row, nil
}

func validateErrandTeardownUpdate(current, next *models.Errand) error {
	before, after := current.TeardownIntent, next.TeardownIntent
	if before == nil {
		if after == nil || after.State == models.ErrandTeardownIntentPending && teardownPurpose(after.Purpose) == models.ErrandTeardownPurposeCancellation {
			return nil
		}
		return models.ErrInvalidErrand
	}
	if after == nil {
		return models.ErrErrandVersionConflict
	}
	if before.ID == after.ID {
		return validateSameTeardownIntent(before, after)
	}
	if cancellationReplacesCompletion(before, after, next) {
		return nil
	}
	return validateResolvedTeardownReplacement(before, after, next)
}

func validateSameTeardownIntent(before, after *models.ErrandTeardownIntent) error {
	if before.Version != after.Version || !before.RequestedAt.Equal(after.RequestedAt) || before.Target != after.Target || teardownPurpose(before.Purpose) != teardownPurpose(after.Purpose) {
		return models.ErrInvalidErrand
	}
	if before.State != after.State {
		return models.ErrErrandVersionConflict
	}
	return nil
}

func cancellationReplacesCompletion(before, after *models.ErrandTeardownIntent, next *models.Errand) bool {
	return (before.Purpose == models.ErrandTeardownPurposeCompletion || before.Purpose == "" && after.Target == before.Target) && (before.State == models.ErrandTeardownIntentPending || before.State == models.ErrandTeardownIntentClosed) && after.Purpose == models.ErrandTeardownPurposeCancellation && after.State == models.ErrandTeardownIntentPending && next.Lifecycle == models.ErrandLifecycleCancelled && next.CancelSource != ""
}

func validateResolvedTeardownReplacement(before, after *models.ErrandTeardownIntent, next *models.Errand) error {
	if before.State != models.ErrandTeardownIntentCompleted && before.State != models.ErrandTeardownIntentSuperseded {
		return models.ErrErrandVersionConflict
	}
	if after.State != models.ErrandTeardownIntentPending || (after.Purpose != models.ErrandTeardownPurposeCancellation && after.Purpose != models.ErrandTeardownPurposeCompletion) {
		return models.ErrInvalidErrand
	}
	if after.Purpose == models.ErrandTeardownPurposeCompletion && (next.Lifecycle != models.ErrandLifecycleLaunched || before.Target.TurnID == after.Target.TurnID) {
		return models.ErrInvalidErrand
	}
	if after.Purpose == models.ErrandTeardownPurposeCancellation && (next.Lifecycle != models.ErrandLifecycleCancelled || next.CancelSource == "") {
		return models.ErrInvalidErrand
	}
	return nil
}

func teardownPurpose(purpose models.ErrandTeardownPurpose) models.ErrandTeardownPurpose {
	if purpose == "" {
		return models.ErrandTeardownPurposeCancellation
	}
	return purpose
}

func validErrandTeardownTransition(intent *models.ErrandTeardownIntent, state models.ErrandTeardownIntentState) bool {
	if intent.State == models.ErrandTeardownIntentPending {
		if state == models.ErrandTeardownIntentSuperseded {
			return true
		}
		if teardownPurpose(intent.Purpose) == models.ErrandTeardownPurposeCompletion {
			return state == models.ErrandTeardownIntentClosed
		}
		return state == models.ErrandTeardownIntentCompleted
	}
	return intent.Purpose == models.ErrandTeardownPurposeCompletion &&
		intent.State == models.ErrandTeardownIntentClosed && state == models.ErrandTeardownIntentCompleted
}

func writeErrandUpdate(ctx context.Context, tx *sqlx.Tx, errand *models.Errand) error {
	teardownIntent, err := encodeErrandTeardownIntent(errand)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_session_errands SET
			lifecycle = ?, cancel_source = ?, last_outcome = ?, awaiting_result = ?,
			result_text = ?, result_truncated = ?, result_turn_id = ?, result_kind = ?, result_state = ?,
			delivered_route = ?, delivery_token = ?, delivered_at = ?, latest_settled_turn_id = ?,
			settled_at = ?, updated_at = ?, result_revision = ?, teardown_intent = ?, version = version + 1
		WHERE id = ? AND version = ?`),
		string(errand.Lifecycle), string(errand.CancelSource), string(errand.LastOutcome), errand.AwaitingResult,
		errand.ResultText, errand.ResultTruncated, errand.ResultTurnID, string(errand.ResultKind),
		string(errand.ResultState), string(errand.DeliveredRoute), errand.DeliveryToken, utcTimeArg(errand.DeliveredAt),
		errand.LatestSettledTurnID, utcTimeArg(errand.SettledAt), errand.UpdatedAt, errand.ResultRevision, teardownIntent, errand.ID, errand.Version)
	if err != nil {
		return fmt.Errorf("update errand %s: %w", errand.ID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return models.ErrErrandVersionConflict
	}
	return nil
}

// DeleteErrand removes an errand row. Deleting a missing row is not an error.
func (r *Repository) DeleteErrand(ctx context.Context, id string) error {
	taskID, err := r.errandTaskID(ctx, id)
	if errors.Is(err, models.ErrErrandNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.withErrandTask(ctx, taskID, func(tx *sqlx.Tx) error {
		return deleteErrandForSessionTx(ctx, tx, id)
	})
}

func deleteErrandForSessionTx(ctx context.Context, tx *sqlx.Tx, sessionID string) error {
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM task_session_errands WHERE id = ?`), sessionID); err != nil {
		return fmt.Errorf("delete errand for session %s: %w", sessionID, err)
	}
	return nil
}

func (r *Repository) listErrands(ctx context.Context, condition string, args ...interface{}) ([]*models.Errand, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`SELECT `+errandColumns+`, `+errandAwaitProjection+` FROM task_session_errands WHERE `+condition), args...)
	if err != nil {
		return nil, fmt.Errorf("list errands: %w", err)
	}
	defer func() { _ = rows.Close() }()
	errands := []*models.Errand{}
	for rows.Next() {
		errand, err := scanProjectedErrand(rows)
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

func scanErrand(row errandRowScanner, delivery ...*models.ErrandDeliveryState) (*models.Errand, error) {
	var errand models.Errand
	var mcpServerIDs, teardownIntent string
	var deliveredAt, settledAt sql.NullTime
	fields := []any{
		&errand.ID, &errand.TaskID, &errand.ParentSessionID, &errand.Title, &errand.Objective,
		&errand.AgentProfileID, &mcpServerIDs, &errand.FirstTurnID, &errand.SubmissionID,
		&errand.Lifecycle, &errand.CancelSource, &errand.LastOutcome, &errand.AwaitingResult,
		&errand.ResultText, &errand.ResultTruncated, &errand.ResultTurnID, &errand.ResultKind, &errand.ResultState,
		&errand.DeliveredRoute, &errand.DeliveryToken, &deliveredAt, &errand.LatestSettledTurnID,
		&errand.Version, &errand.CreatedAt, &errand.UpdatedAt, &settledAt, &errand.ResultRevision, &errand.ParentAdmissionGeneration, &teardownIntent,
	}
	for _, state := range delivery {
		fields = append(fields, state)
	}
	if err := row.Scan(fields...); err != nil {
		return nil, err
	}
	if err := decodeErrandMCPSnapshot(mcpServerIDs, &errand); err != nil {
		return nil, fmt.Errorf("decode errand %s MCP servers: %w", errand.ID, err)
	}
	if len(errand.MCPServerIDs) == 0 {
		errand.MCPServerIDs = nil
	}
	errand.CreatedAt, errand.UpdatedAt = errand.CreatedAt.UTC(), errand.UpdatedAt.UTC()
	errand.DeliveredAt = utcTimeFromNull(deliveredAt)
	errand.SettledAt = utcTimeFromNull(settledAt)
	if err := decodeErrandTeardownIntent(teardownIntent, &errand); err != nil {
		return nil, fmt.Errorf("decode errand %s teardown intent: %w", errand.ID, err)
	}
	return &errand, nil
}

func encodeErrandTeardownIntent(errand *models.Errand) (string, error) {
	if errand.TeardownIntent == nil {
		return "", nil
	}
	if err := errand.TeardownIntent.ValidateFor(errand); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(errand.TeardownIntent)
	if err != nil {
		return "", fmt.Errorf("encode errand teardown intent: %w", err)
	}
	return string(encoded), nil
}

func decodeErrandTeardownIntent(encoded string, errand *models.Errand) error {
	if encoded == "" {
		return nil
	}
	var intent models.ErrandTeardownIntent
	if err := json.Unmarshal([]byte(encoded), &intent); err != nil {
		return err
	}
	intent.RequestedAt = intent.RequestedAt.UTC()
	if err := intent.ValidateFor(errand); err != nil {
		return err
	}
	errand.TeardownIntent = &intent
	return nil
}

func encodeErrandMCPSnapshot(row *models.Errand) (string, error) {
	ids := row.MCPServerIDs
	if len(ids) == 0 && len(row.MCPExcludedServerIDs) == 0 {
		return "[]", nil
	}
	var value any = ids
	if len(row.MCPExcludedServerIDs) > 0 {
		value = errandMCPSnapshot{ServerIDs: ids, ExcludedServerIDs: row.MCPExcludedServerIDs}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode errand MCP servers: %w", err)
	}
	return string(encoded), nil
}

type errandMCPSnapshot struct {
	ServerIDs         []string `json:"server_ids"`
	ExcludedServerIDs []string `json:"excluded_server_ids"`
}

func decodeErrandMCPSnapshot(encoded string, row *models.Errand) error {
	encoded = strings.TrimSpace(encoded)
	if len(encoded) > 0 && encoded[0] == '{' {
		var snapshot errandMCPSnapshot
		if err := json.Unmarshal([]byte(encoded), &snapshot); err != nil {
			return err
		}
		row.MCPServerIDs, row.MCPExcludedServerIDs = snapshot.ServerIDs, snapshot.ExcludedServerIDs
		return nil
	}
	return json.Unmarshal([]byte(encoded), &row.MCPServerIDs)
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

func currentErrandForUpdate(ctx context.Context, tx *sqlx.Tx, errand *models.Errand) (*models.Errand, error) {
	current, err := scanErrand(tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+errandColumns+` FROM task_session_errands WHERE id = ?`), errand.ID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.ErrErrandNotFound
	}
	if err != nil {
		return nil, err
	}
	if current.Version != errand.Version {
		return nil, models.ErrErrandVersionConflict
	}
	if current.ParentAdmissionGeneration != errand.ParentAdmissionGeneration {
		return nil, models.ErrInvalidErrand
	}
	return current, nil
}

// This locator is immutable; result and admission reads follow the task guard.
func (r *Repository) errandTaskID(ctx context.Context, id string) (string, error) {
	var taskID string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT task_id FROM task_session_errands WHERE id = ?`), id).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", models.ErrErrandNotFound
	}
	return taskID, err
}

func preserveErrandIdentity(row, current *models.Errand) {
	row.TaskID, row.ParentSessionID = current.TaskID, current.ParentSessionID
	row.Title, row.Objective, row.AgentProfileID = current.Title, current.Objective, current.AgentProfileID
	row.MCPServerIDs, row.MCPExcludedServerIDs = current.MCPServerIDs, current.MCPExcludedServerIDs
	row.FirstTurnID, row.SubmissionID, row.CreatedAt = current.FirstTurnID, current.SubmissionID, current.CreatedAt
}

func errandUpdateRevision(current, errand *models.Errand) string {
	if current.ResultText != errand.ResultText || current.ResultKind != errand.ResultKind || current.ResultTurnID != errand.ResultTurnID || current.ResultTruncated != errand.ResultTruncated {
		return uuid.NewString()
	}
	return current.ResultRevision
}
