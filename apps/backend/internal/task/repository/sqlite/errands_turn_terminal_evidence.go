package sqlite

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

// A projected terminal can survive a crash before the capture intent is sealed.
// Only matching, resolved terminal identities can supply the missing outcome.
func (r *Repository) resolveErrandTerminalEvidence(ctx context.Context, intent *models.AgentDeliveryEffect) error {
	if intent.State == models.DeliveryEffectCompleted || strings.Contains(intent.Outcome, ":") {
		return nil
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`SELECT terminal.stream_id, terminal.sequence, terminal.harness_generation, terminal.outcome
		FROM agent_delivery_effects terminal WHERE terminal.effect_type = ? AND terminal.session_id = ? AND terminal.incarnation_id = ?
		AND terminal.turn_id = ? AND terminal.submission_id = ? AND (? = 0 OR terminal.harness_generation = ?)
		AND terminal.harness_generation > 0 AND terminal.sequence > 0 AND terminal.stream_id <> ''
		AND (terminal.state IN (?, ?) OR (terminal.state = ? AND EXISTS (
			SELECT 1 FROM agent_delivery_cursors c WHERE c.stream_id = terminal.stream_id AND c.session_id = terminal.session_id
			AND c.incarnation_id = terminal.incarnation_id AND c.harness_generation = terminal.harness_generation
			AND c.projected_sequence >= terminal.sequence)))
		AND terminal.outcome IN (?, ?, ?) ORDER BY terminal.sequence DESC, terminal.stream_id`),
		models.DeliveryTerminalEffectType, intent.SessionID, intent.IncarnationID, intent.TurnID, intent.SubmissionID,
		intent.HarnessGeneration, intent.HarnessGeneration, models.DeliveryEffectBlockResolved, models.DeliveryEffectCompleted, models.DeliveryEffectPending,
		models.DeliverySubmissionCompleted, models.DeliverySubmissionFailed, models.DeliverySubmissionCancelled)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	var selected *models.AgentDeliveryEffect
	for rows.Next() {
		var terminal models.AgentDeliveryEffect
		if err := rows.Scan(&terminal.StreamID, &terminal.Sequence, &terminal.HarnessGeneration, &terminal.Outcome); err != nil {
			return err
		}
		if selected != nil && (selected.Outcome != terminal.Outcome || selected.HarnessGeneration != terminal.HarnessGeneration) {
			return nil
		}
		if selected == nil {
			selected = &terminal
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if selected != nil {
		bindErrandTerminalEvidence(intent, selected)
	}
	return nil
}

func bindErrandTerminalEvidence(intent, terminal *models.AgentDeliveryEffect) {
	cause := "done"
	switch models.DeliverySubmissionState(terminal.Outcome) {
	case models.DeliverySubmissionFailed:
		cause = "failed"
	case models.DeliverySubmissionCancelled:
		cause = "interrupted"
	}
	intent.Outcome = settlementOrigin(intent.Outcome) + ":" + cause
	if intent.HarnessGeneration == 0 {
		intent.HarnessGeneration = terminal.HarnessGeneration
		intent.StreamID, intent.Sequence = terminal.StreamID, terminal.Sequence
	}
}
