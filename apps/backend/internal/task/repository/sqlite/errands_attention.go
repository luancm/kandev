package sqlite

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
)

// recoveryAttentionPredicate is shared by status reads and capacity summaries.
// Recovery identity is admission evidence only; this observation never settles work.
func recoveryAttentionPredicate(driver string) string {
	recovery := func(key string) string {
		return "(" + dialect.JSONExtractPath(driver, "s.metadata", "agent_delivery_recovery", key) + ")"
	}
	lifecycle := dialect.JSONExtract(driver, "t.metadata", "lifecycle_only")
	return `(e.lifecycle = 'launched' AND s.state NOT IN ('STARTING', 'RUNNING', 'FAILED')
 AND ` + recovery("phase") + ` IN ('uncertain', 'recovered', 'settled', 'restored')
 AND CAST(` + recovery("revision") + ` AS BIGINT) > 0
 AND ` + recovery("session_id") + ` = s.id
 AND ` + recovery("incarnation_id") + ` = s.queue_incarnation_id
 AND COALESCE(` + recovery("submission_id") + `, '') <> ''
 AND EXISTS (SELECT 1 FROM task_session_turns t
 WHERE t.task_session_id = s.id AND t.completed_at IS NOT NULL
 AND CAST(COALESCE(` + lifecycle + `, '') AS TEXT) NOT IN ('true', '1')
 AND (e.result_turn_id = '' OR EXISTS (SELECT 1 FROM task_session_turns rt
   WHERE rt.id = e.result_turn_id AND rt.task_session_id = s.id AND rt.started_at < t.started_at))
 AND ((t.id = e.first_turn_id AND e.submission_id = ` + recovery("submission_id") + `)
 OR EXISTS (SELECT 1 FROM agent_delivery_effects a WHERE a.session_id = s.id
   AND a.incarnation_id = s.queue_incarnation_id AND a.turn_id = t.id
   AND a.submission_id = ` + recovery("submission_id") + `
   AND CAST(a.harness_generation AS TEXT) = CAST(` + recovery("harness_generation") + ` AS TEXT)
   AND a.effect_type IN ('errand.turn_settlement', 'agent_delivery.prompt_admission.v1', 'errand.interrupted_delegation.v1')))
 AND NOT ` + projectedErrandTerminalPredicate(recovery("submission_id"), recovery("harness_generation")) + `
 AND NOT EXISTS (SELECT 1 FROM agent_delivery_effects receipt WHERE receipt.session_id = s.id
   AND receipt.incarnation_id = s.queue_incarnation_id AND receipt.turn_id = t.id
   AND receipt.submission_id = ` + recovery("submission_id") + `
   AND receipt.effect_type = 'errand.turn_settlement'
   AND (SELECT COUNT(*) FROM agent_delivery_effects other_receipt
     WHERE other_receipt.session_id = s.id AND other_receipt.incarnation_id = s.queue_incarnation_id
     AND other_receipt.turn_id = t.id AND other_receipt.effect_type = 'errand.turn_settlement') = 1
   AND (receipt.state = 'pending' OR (receipt.state = 'completed' AND receipt.completed_at IS NOT NULL))
   AND receipt.outcome IN ('parent:done', 'parent:failed', 'parent:interrupted', 'user:done', 'user:failed', 'user:interrupted', 'parent:cancelled', 'user:cancelled'))))`
}

// ErrandRecoveryAttention reads the same exact observation used for capacity.
func (r *Repository) ErrandRecoveryAttention(ctx context.Context, id string) (bool, error) {
	var attention bool
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT EXISTS (SELECT 1
 FROM task_session_errands e JOIN task_sessions s ON s.id = e.id
 WHERE e.id = ? AND `+recoveryAttentionPredicate(r.ro.DriverName())+`)`), id).Scan(&attention)
	if err != nil {
		return false, fmt.Errorf("read errand recovery attention: %w", err)
	}
	return attention, nil
}

func liveErrandPredicate(driver string) string {
	return `(e.lifecycle = 'queued' OR (e.lifecycle = 'launched' AND s.id IS NOT NULL
 AND s.state <> 'FAILED' AND (s.state IN ('STARTING', 'RUNNING') OR e.last_outcome = '' OR ` + recoveryAttentionPredicate(driver) + `)))`
}

// Match the projected terminal evidence consumed by resolveErrandTerminalEvidence.
// Conflicting outcomes cannot turn unresolved recovery into ordinary reconciliation.
func projectedErrandTerminalPredicate(submission, generation string) string {
	return `EXISTS (SELECT 1 FROM agent_delivery_effects terminal
 WHERE terminal.effect_type = 'agent_delivery.terminal_settlement'
 AND terminal.session_id = s.id AND terminal.incarnation_id = s.queue_incarnation_id
 AND terminal.turn_id = t.id AND terminal.submission_id = ` + submission + `
 AND terminal.harness_generation > 0
 AND CAST(terminal.harness_generation AS TEXT) = CAST(` + generation + ` AS TEXT)
 AND terminal.sequence > 0 AND terminal.stream_id <> ''
 AND (terminal.state IN ('block_resolved', 'completed') OR (terminal.state = 'pending' AND EXISTS (
  SELECT 1 FROM agent_delivery_cursors c WHERE c.stream_id = terminal.stream_id
  AND c.session_id = terminal.session_id AND c.incarnation_id = terminal.incarnation_id
  AND c.harness_generation = terminal.harness_generation AND c.projected_sequence >= terminal.sequence)))
 AND terminal.outcome IN ('completed', 'failed', 'cancelled')
 GROUP BY terminal.turn_id HAVING COUNT(DISTINCT terminal.outcome) = 1)`
}
