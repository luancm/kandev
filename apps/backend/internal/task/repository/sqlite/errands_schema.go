package sqlite

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// task_session_errands has no foreign key to task_sessions: a queued errand
// reserves its session ID before the session row exists. Rows leave with
// their session through purgeTaskSessionStateTx and with their task through
// the task foreign key.
var errandsSchemaStatements = []struct{ name, sql string }{
	{"task_session_errands.table", `
		CREATE TABLE IF NOT EXISTS task_session_errands (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			parent_session_id TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			objective TEXT NOT NULL DEFAULT '',
			agent_profile_id TEXT NOT NULL DEFAULT '',
			mcp_server_ids TEXT NOT NULL DEFAULT '[]',
			first_turn_id TEXT NOT NULL DEFAULT '',
			submission_id TEXT NOT NULL DEFAULT '',
			lifecycle TEXT NOT NULL DEFAULT 'queued',
			cancel_source TEXT NOT NULL DEFAULT '',
			last_outcome TEXT NOT NULL DEFAULT '',
			awaiting_result BOOLEAN NOT NULL DEFAULT FALSE,
			result_text TEXT NOT NULL DEFAULT '',
			result_truncated BOOLEAN NOT NULL DEFAULT FALSE,
			result_turn_id TEXT NOT NULL DEFAULT '',
			result_kind TEXT NOT NULL DEFAULT '',
			result_state TEXT NOT NULL DEFAULT 'none',
			delivered_route TEXT NOT NULL DEFAULT '',
			delivery_token TEXT NOT NULL DEFAULT '',
			delivered_at TIMESTAMP,
			latest_settled_turn_id TEXT NOT NULL DEFAULT '',
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			settled_at TIMESTAMP,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)`},
	{"task_session_errands.task_lifecycle", `
		CREATE INDEX IF NOT EXISTS idx_task_session_errands_task_lifecycle
			ON task_session_errands(task_id, lifecycle)`},
	{"task_session_errands.parent", `
		CREATE INDEX IF NOT EXISTS idx_task_session_errands_parent
			ON task_session_errands(parent_session_id)`},
}

func (r *Repository) initErrandsSchema() error {
	for _, statement := range errandsSchemaStatements {
		if err := r.migrate.Apply(statement.name, statement.sql); err != nil {
			return fmt.Errorf("initialize errands schema: %w", err)
		}
	}
	if err := r.migrate.Err(); err != nil {
		return fmt.Errorf("initialize errands schema: %w", err)
	}
	return nil
}

// migrateTaskSessionErrandColumns adds the immutable session role and parent
// link. It must run after the task_sessions rebuilds, which copy an explicit
// column list and would drop columns added before them.
func (r *Repository) migrateTaskSessionErrandColumns() {
	_ = r.migrate.Apply("task_sessions.role", `ALTER TABLE task_sessions ADD COLUMN role TEXT NOT NULL DEFAULT ''`)
	_ = r.migrate.Apply("task_sessions.parent_session_id", `ALTER TABLE task_sessions ADD COLUMN parent_session_id TEXT NOT NULL DEFAULT ''`)
	_ = r.migrate.Apply("idx_task_sessions_parent_session_id",
		`CREATE INDEX IF NOT EXISTS idx_task_sessions_parent_session_id ON task_sessions(parent_session_id)`)
}

// migrateErrandResultRevisions preserves an assigned identity across schema replay.
func (r *Repository) migrateErrandResultRevisions(ctx context.Context) error {
	if err := r.migrate.Apply("task_session_errands.result_revision", `ALTER TABLE task_session_errands ADD COLUMN result_revision TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	return r.backfillErrandResultRevisions(ctx)
}

func (r *Repository) migrateErrandTeardownIntent() error {
	return r.migrate.Apply("task_session_errands.teardown_intent", `ALTER TABLE task_session_errands
		ADD COLUMN teardown_intent TEXT NOT NULL DEFAULT ''`)
}

func (r *Repository) backfillErrandResultRevisions(ctx context.Context) error {
	var rows []struct {
		ID     string
		TaskID string `db:"task_id"`
	}
	if err := r.db.SelectContext(ctx, &rows, `SELECT id, task_id FROM task_session_errands
		WHERE result_revision = '' AND (result_text <> '' OR result_kind <> '') ORDER BY task_id, id`); err != nil {
		return err
	}
	for _, row := range rows {
		revision := uuid.NewString()
		if err := r.withErrandTask(ctx, row.TaskID, func(tx *sqlx.Tx) error {
			_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_session_errands SET result_revision = ? WHERE id = ? AND result_revision = ''`), revision, row.ID)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}
