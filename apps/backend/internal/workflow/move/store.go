package move

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jmoiron/sqlx"
)

// ErrEntryNotFound is returned when a persisted move entry cannot be removed.
var ErrEntryNotFound = errors.New("workflow move entry not found")

// Entry is the durable, private part of a move request. It is looked up by
// move_id after the task.moved notification arrives; the notification itself
// never carries the option payload.
type Entry struct {
	ID              string
	TaskID          string
	Options         EntryOptions
	Phase           EntryPhase
	TargetSessionID string
}

// EntryPhase is the durable dispatch state of one private move entry.
type EntryPhase string

const (
	EntryPhaseTransitionCommitted EntryPhase = "transition_committed"
	EntryPhaseExitApplied         EntryPhase = "exit_applied"
	EntryPhaseProfileApplied      EntryPhase = "profile_applied"
	EntryPhaseResetApplied        EntryPhase = "reset_applied"
	EntryPhaseConfigApplied       EntryPhase = "config_applied"
	EntryPhaseActionsApplied      EntryPhase = "actions_applied"
	EntryPhaseDispatchReady       EntryPhase = "dispatch_ready"
	EntryPhaseDispatchClaimed     EntryPhase = "dispatch_claimed"
	EntryPhaseDispatchAccepted    EntryPhase = "dispatch_accepted"
)

// EntryStore persists transition-local options independently of the public
// task.moved event. Implementations must be safe to call from both the task
// service and the orchestrator.
type EntryStore interface {
	Save(context.Context, *Entry) error
	Load(context.Context, string) (*Entry, error)
	Delete(context.Context, string) error
}

// LifecycleStore adds durable compare-and-set dispatch and atomic cleanup to
// an EntryStore. Test-only in-memory stores may keep the simpler interface.
type LifecycleStore interface {
	EntryStore
	ClaimPhase(context.Context, string, EntryPhase, EntryPhase, string) (bool, error)
	Finalize(context.Context, string, string) error
}

// TransactionOwnerStore identifies the database connection pool that owns
// direct workflow-move entry transactions. Callers compare this narrow token
// with the task repository before asking either side to admit a move.
type TransactionOwnerStore interface {
	EntryStore
	WorkflowMoveTransactionOwner() *sql.DB
}

type sqliteEntryStore struct {
	db *sqlx.DB
	ro *sqlx.DB
}

func (s *sqliteEntryStore) WorkflowMoveTransactionOwner() *sql.DB { return s.db.DB }

// NewSQLiteEntryStore creates the private move-entry table on the shared task
// database. The table is intentionally independent from workflow policy and
// queue rows so a missed notification can be recovered without exposing the
// instructions to workspace event subscribers.
func NewSQLiteEntryStore(writer, reader *sqlx.DB) (EntryStore, error) {
	if writer == nil || reader == nil {
		return nil, errors.New("workflow move entry store requires writer and reader")
	}
	store := &sqliteEntryStore{db: writer, ro: reader}
	if _, err := writer.Exec(`
		CREATE TABLE IF NOT EXISTS workflow_move_entries (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL DEFAULT '',
			options_json TEXT NOT NULL DEFAULT '{}',
			phase TEXT NOT NULL DEFAULT 'transition_committed',
			target_session_id TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_workflow_move_entries_task_id
			ON workflow_move_entries(task_id);
	`); err != nil {
		return nil, fmt.Errorf("create workflow move entry schema: %w", err)
	}
	if _, err := writer.Exec(`ALTER TABLE workflow_move_entries ADD COLUMN phase TEXT NOT NULL DEFAULT 'transition_committed'`); err != nil && !isDuplicateColumnError(err) {
		return nil, fmt.Errorf("add workflow move entry phase: %w", err)
	}
	if _, err := writer.Exec(`ALTER TABLE workflow_move_entries ADD COLUMN target_session_id TEXT NOT NULL DEFAULT ''`); err != nil && !isDuplicateColumnError(err) {
		return nil, fmt.Errorf("add workflow move entry target session: %w", err)
	}
	return store, nil
}

func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "duplicate column") || strings.Contains(message, "already exists")
}

func (s *sqliteEntryStore) Save(ctx context.Context, entry *Entry) error {
	if entry == nil || entry.ID == "" {
		return errors.New("workflow move entry id is required")
	}
	options, err := json.Marshal(entry.Options)
	if err != nil {
		return fmt.Errorf("marshal workflow move entry options: %w", err)
	}
	_, err = s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO workflow_move_entries (id, task_id, options_json, phase, target_session_id)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET task_id = excluded.task_id, options_json = excluded.options_json
		WHERE workflow_move_entries.phase = 'transition_committed'
	`), entry.ID, entry.TaskID, options, EntryPhaseTransitionCommitted, entry.TargetSessionID)
	if err != nil {
		return fmt.Errorf("save workflow move entry: %w", err)
	}
	return nil
}

func (s *sqliteEntryStore) Load(ctx context.Context, id string) (*Entry, error) {
	if id == "" {
		return nil, nil
	}
	var taskID, encoded, phase, targetSessionID string
	err := s.ro.QueryRowxContext(ctx, s.ro.Rebind(`
		SELECT task_id, options_json, phase, target_session_id FROM workflow_move_entries WHERE id = ?
	`), id).Scan(&taskID, &encoded, &phase, &targetSessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load workflow move entry: %w", err)
	}
	var options EntryOptions
	decoded, err := DecodeEntryOptionsJSON([]byte(encoded))
	if err != nil {
		return nil, fmt.Errorf("decode workflow move entry options: %w", err)
	}
	if decoded != nil {
		options = *decoded
	}
	return &Entry{ID: id, TaskID: taskID, Options: options, Phase: EntryPhase(phase), TargetSessionID: targetSessionID}, nil
}

// DecodeEntryOptionsJSON decodes a persisted entry payload strictly. Unknown
// fields and trailing JSON are rejected so corrupted or newer payloads cannot
// be silently treated as an option-less move.
func DecodeEntryOptionsJSON(encoded []byte) (*EntryOptions, error) {
	trimmed := bytes.TrimSpace(encoded)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("{}")) {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, errors.New("workflow move entry options must be a JSON object")
	}
	for _, field := range []string{"reset_context", "instructions", "agent_profile_id"} {
		if raw, ok := fields[field]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("workflow move entry option %s cannot be null", field)
		}
	}
	var options EntryOptions
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&options); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	if !options.HasOverrides() {
		return nil, nil
	}
	return &options, nil
}

func (s *sqliteEntryStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM workflow_move_entries WHERE id = ?`), id)
	if err != nil {
		return fmt.Errorf("delete workflow move entry: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect workflow move entry deletion: %w", err)
	}
	if rows == 0 {
		return ErrEntryNotFound
	}
	return nil
}

func (s *sqliteEntryStore) ClaimPhase(
	ctx context.Context,
	id string,
	expected EntryPhase,
	next EntryPhase,
	targetSessionID string,
) (bool, error) {
	if id == "" || expected == "" || next == "" {
		return false, errors.New("workflow move phase transition is incomplete")
	}
	if !validEntryPhaseTransition(expected, next) {
		return false, nil
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE workflow_move_entries
		SET phase = ?, target_session_id = ?
		WHERE id = ? AND phase = ?
	`), next, targetSessionID, id, expected)
	if err != nil {
		return false, fmt.Errorf("claim workflow move phase: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect workflow move phase claim: %w", err)
	}
	return rows == 1, nil
}

var validEntryPhaseTransitions = map[[2]EntryPhase]struct{}{
	{EntryPhaseTransitionCommitted, EntryPhaseExitApplied}:    {},
	{EntryPhaseTransitionCommitted, EntryPhaseProfileApplied}: {},
	{EntryPhaseTransitionCommitted, EntryPhaseDispatchReady}:  {},
	{EntryPhaseExitApplied, EntryPhaseProfileApplied}:         {},
	{EntryPhaseProfileApplied, EntryPhaseResetApplied}:        {},
	{EntryPhaseResetApplied, EntryPhaseConfigApplied}:         {},
	{EntryPhaseConfigApplied, EntryPhaseActionsApplied}:       {},
	{EntryPhaseActionsApplied, EntryPhaseDispatchReady}:       {},
	{EntryPhaseDispatchReady, EntryPhaseDispatchReady}:        {},
	{EntryPhaseDispatchReady, EntryPhaseDispatchClaimed}:      {},
	{EntryPhaseDispatchReady, EntryPhaseDispatchAccepted}:     {},
	{EntryPhaseDispatchClaimed, EntryPhaseDispatchReady}:      {},
	{EntryPhaseDispatchClaimed, EntryPhaseDispatchAccepted}:   {},
}

func validEntryPhaseTransition(expected, next EntryPhase) bool {
	_, valid := validEntryPhaseTransitions[[2]EntryPhase{expected, next}]
	return valid
}

func (s *sqliteEntryStore) Finalize(ctx context.Context, taskID, moveID string) error {
	if taskID == "" || moveID == "" {
		return errors.New("workflow move finalization identity is incomplete")
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workflow move finalization: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if s.db.DriverName() == "pgx" {
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			UPDATE tasks
			SET metadata = (CASE WHEN metadata IS NULL OR metadata = '' OR metadata = 'null' THEN '{}'::jsonb ELSE metadata::jsonb END #- ARRAY['workflow_move_pending']::text[])::text,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
			  AND jsonb_extract_path_text(CASE WHEN metadata IS NULL OR metadata = '' OR metadata = 'null' THEN '{}'::jsonb ELSE metadata::jsonb END, 'workflow_move_pending', 'move_id') = ?
		`), taskID, moveID); err != nil {
			return fmt.Errorf("clear workflow move marker: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			UPDATE tasks
			SET metadata = json_remove(CASE WHEN metadata IS NULL OR metadata = '' OR metadata = 'null' THEN '{}' ELSE metadata END, '$.workflow_move_pending'),
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
			  AND json_extract(CASE WHEN metadata IS NULL OR metadata = '' OR metadata = 'null' THEN '{}' ELSE metadata END, '$.workflow_move_pending.move_id') = ?
		`), taskID, moveID); err != nil {
			return fmt.Errorf("clear workflow move marker: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM workflow_move_entries WHERE id = ? AND task_id = ?
	`), moveID, taskID); err != nil {
		return fmt.Errorf("delete workflow move entry: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workflow move finalization: %w", err)
	}
	return nil
}
