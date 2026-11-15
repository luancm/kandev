package sqlite

import "fmt"

// Delivery records survive result replacement and session removal. Only task
// deletion removes this recovery evidence.
var errandDeliverySchema = []struct{ name, sql string }{
	{"errand_parent_admission.table", `CREATE TABLE IF NOT EXISTS errand_parent_admission (
		task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		parent_session_id TEXT NOT NULL, errand_id TEXT NOT NULL,
		sender_session_id TEXT NOT NULL, turn_id TEXT NOT NULL, submission_id TEXT NOT NULL,
		state TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL,
		PRIMARY KEY (errand_id, submission_id))`},
	{"errand_parent_admission.unresolved", `CREATE UNIQUE INDEX IF NOT EXISTS idx_errand_parent_admission_unresolved
		ON errand_parent_admission(errand_id) WHERE state IN ('reserved', 'unknown')`},
	{"errand_result_claim.table", `CREATE TABLE IF NOT EXISTS errand_result_claim (
		token TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		parent_session_id TEXT NOT NULL, route TEXT NOT NULL, created_at TIMESTAMP NOT NULL)`},
	{"errand_result_claim_item.table", `CREATE TABLE IF NOT EXISTS errand_result_claim_item (
		token TEXT NOT NULL REFERENCES errand_result_claim(token) ON DELETE CASCADE,
		errand_id TEXT NOT NULL, result_revision TEXT NOT NULL,
		parent_admission_generation BIGINT NOT NULL, state TEXT NOT NULL DEFAULT 'claimed',
		updated_at TIMESTAMP NOT NULL, PRIMARY KEY (token, errand_id, result_revision))`},
	{"errand_result_claim_item.deferred", `CREATE INDEX IF NOT EXISTS idx_errand_result_claim_item_deferred
		ON errand_result_claim_item(errand_id, state)`},
	{"errand_await_attempt.table", `CREATE TABLE IF NOT EXISTS errand_await_attempt (
		token TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		parent_session_id TEXT NOT NULL, state TEXT NOT NULL,
		parent_incarnation_id TEXT NOT NULL DEFAULT '', harness_generation BIGINT NOT NULL DEFAULT 0,
		turn_id TEXT NOT NULL DEFAULT '', submission_id TEXT NOT NULL DEFAULT '',
		receipt_stream_id TEXT NOT NULL DEFAULT '', receipt_sequence BIGINT NOT NULL DEFAULT 0,
		receipt_tool_call_id TEXT NOT NULL DEFAULT '', receipt_payload_hash TEXT NOT NULL DEFAULT '',
		receipt_hash_version BIGINT NOT NULL DEFAULT 0, received_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`},
	{"errand_await_attempt.task_state", `CREATE INDEX IF NOT EXISTS idx_errand_await_attempt_task_state
		ON errand_await_attempt(task_id, state)`},
	{"errand_await_attempt_item.table", `CREATE TABLE IF NOT EXISTS errand_await_attempt_item (
		token TEXT NOT NULL REFERENCES errand_await_attempt(token) ON DELETE CASCADE,
		errand_id TEXT NOT NULL, result_revision TEXT NOT NULL,
		parent_admission_generation BIGINT NOT NULL,
		PRIMARY KEY (token, errand_id, result_revision))`},
}

func (r *Repository) migrateErrandDelivery() error {
	if err := r.migrate.Apply("task_session_errands.parent_admission_generation", `ALTER TABLE task_session_errands
		ADD COLUMN parent_admission_generation BIGINT NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	for _, statement := range errandDeliverySchema {
		if err := r.migrate.Apply(statement.name, statement.sql); err != nil {
			return fmt.Errorf("initialize errand delivery: %w", err)
		}
	}
	if err := r.migrate.Apply("errand_result_claim.snapshot", `ALTER TABLE errand_result_claim ADD COLUMN snapshot TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	return nil
}
