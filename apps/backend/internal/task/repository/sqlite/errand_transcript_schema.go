package sqlite

func (r *Repository) initErrandTranscriptSchema() error {
	statements := []struct{ name, sql string }{
		{"errand_execution_generations.table", `CREATE TABLE IF NOT EXISTS errand_execution_generations (
 errand_id TEXT NOT NULL, generation BIGINT NOT NULL CHECK(generation > 0), execution_id TEXT NOT NULL DEFAULT '', active_submission_id TEXT NOT NULL DEFAULT '', active_turn_id TEXT NOT NULL DEFAULT '', prompt_generation BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY(errand_id,generation), FOREIGN KEY(errand_id) REFERENCES task_session_errands(id) ON DELETE CASCADE)`},
		{"errand_harness_bindings.table", `CREATE TABLE IF NOT EXISTS errand_harness_bindings (
 errand_id TEXT NOT NULL, owner_generation BIGINT NOT NULL, binding_sequence BIGINT NOT NULL CHECK(binding_sequence > 0),
 execution_id TEXT NOT NULL, incarnation_id TEXT NOT NULL, harness_generation BIGINT NOT NULL CHECK(harness_generation > 0),
 stream_id TEXT NOT NULL UNIQUE, native_session_id TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(errand_id,owner_generation,binding_sequence), UNIQUE(errand_id,owner_generation,incarnation_id,harness_generation),
 FOREIGN KEY(errand_id,owner_generation) REFERENCES errand_execution_generations(errand_id,generation) ON DELETE CASCADE)`},
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
