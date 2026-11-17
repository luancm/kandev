package sqlite

func (r *Repository) initErrandTranscriptSchema() error {
	statements := []struct{ name, sql string }{
		{"errand_turns.table", `CREATE TABLE IF NOT EXISTS errand_turns (
 id TEXT PRIMARY KEY, errand_id TEXT NOT NULL, execution_profile_id TEXT NOT NULL DEFAULT '',
 route_generation BIGINT NOT NULL DEFAULT 0, started_at TIMESTAMP NOT NULL, completed_at TIMESTAMP,
 metadata TEXT NOT NULL DEFAULT '{}', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL,
 UNIQUE(errand_id,id), FOREIGN KEY(errand_id) REFERENCES task_session_errands(id) ON DELETE CASCADE)`},
		{"errand_turns.order", `CREATE INDEX IF NOT EXISTS idx_errand_turns_order ON errand_turns(errand_id,created_at,id)`},
		{"errand_messages.table", `CREATE TABLE IF NOT EXISTS errand_messages (
 id TEXT PRIMARY KEY, errand_id TEXT NOT NULL, turn_id TEXT NOT NULL, payload TEXT NOT NULL,
 created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL,
 FOREIGN KEY(errand_id) REFERENCES task_session_errands(id) ON DELETE CASCADE,
 FOREIGN KEY(errand_id,turn_id) REFERENCES errand_turns(errand_id,id) ON DELETE CASCADE)`},
		{"errand_messages.order", `CREATE INDEX IF NOT EXISTS idx_errand_messages_order ON errand_messages(errand_id,created_at,id)`},
		{"errand_messages.turn", `CREATE INDEX IF NOT EXISTS idx_errand_messages_turn ON errand_messages(errand_id,turn_id,created_at,id)`},
	}
	for _, stmt := range statements {
		if err := r.migrate.Apply(stmt.name, stmt.sql); err != nil {
			return err
		}
	}
	return r.migrate.Err()
}
