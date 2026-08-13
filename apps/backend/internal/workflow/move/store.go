package move

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// ErrEntryNotFound is returned when a persisted move entry cannot be removed.
var ErrEntryNotFound = errors.New("workflow move entry not found")

// Entry is the durable, private part of a move request. It is looked up by
// move_id after the task.moved notification arrives; the notification itself
// never carries the option payload.
type Entry struct {
	ID      string
	TaskID  string
	Options EntryOptions
}

// EntryStore persists transition-local options independently of the public
// task.moved event. Implementations must be safe to call from both the task
// service and the orchestrator.
type EntryStore interface {
	Save(context.Context, *Entry) error
	Load(context.Context, string) (*Entry, error)
	Delete(context.Context, string) error
}

type sqliteEntryStore struct {
	db *sqlx.DB
	ro *sqlx.DB
}

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
			options_json TEXT NOT NULL DEFAULT '{}'
		);
		CREATE INDEX IF NOT EXISTS idx_workflow_move_entries_task_id
			ON workflow_move_entries(task_id);
	`); err != nil {
		return nil, fmt.Errorf("create workflow move entry schema: %w", err)
	}
	return store, nil
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
		INSERT INTO workflow_move_entries (id, task_id, options_json)
		VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET task_id = excluded.task_id, options_json = excluded.options_json
	`), entry.ID, entry.TaskID, options)
	if err != nil {
		return fmt.Errorf("save workflow move entry: %w", err)
	}
	return nil
}

func (s *sqliteEntryStore) Load(ctx context.Context, id string) (*Entry, error) {
	if id == "" {
		return nil, nil
	}
	var taskID, encoded string
	err := s.ro.QueryRowxContext(ctx, s.ro.Rebind(`
		SELECT task_id, options_json FROM workflow_move_entries WHERE id = ?
	`), id).Scan(&taskID, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load workflow move entry: %w", err)
	}
	var options EntryOptions
	if err := json.Unmarshal([]byte(encoded), &options); err != nil {
		return nil, fmt.Errorf("decode workflow move entry options: %w", err)
	}
	return &Entry{ID: id, TaskID: taskID, Options: options}, nil
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
