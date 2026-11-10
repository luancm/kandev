package sqlite

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

// A resolved assessment retains its original proof but delegates outcome
// authority to the completed exact terminal receipt in the same transaction.
func supersedeInterruptedAssessmentTx(ctx context.Context, tx *sqlx.Tx, taskID string, intent *models.AgentDeliveryEffect) error {
	key := "errand.interruption:" + intent.EffectKey
	var raw string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_key = ? AND state = 'pending'`), key).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	p, err := decodeInterruptedDelegation(raw)
	if err != nil {
		return err
	}
	if !interruptedAssessmentOwnerMatches(p, taskID, intent) {
		return models.ErrErrandVersionConflict
	}
	if err := verifyInterruptedAssessmentTx(ctx, tx, key, raw, p); err != nil {
		return err
	}
	var count int
	var outcome string
	var sequence int64
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT count(*), COALESCE(MIN(outcome), ''), COALESCE(MIN(sequence), 0)
 FROM agent_delivery_effects WHERE effect_type = ? AND session_id = ? AND incarnation_id = ? AND submission_id = ? AND turn_id = ?`),
		models.DeliveryTerminalEffectType, p.SessionID, p.IncarnationID, p.SubmissionID, p.TurnID).Scan(&count, &outcome, &sequence)
	if err != nil {
		return err
	}
	if count != 1 || sequence <= 0 {
		return db.ErrErrandSettlementPending
	}
	expected := *intent
	bindErrandTerminalEvidence(&expected, &models.AgentDeliveryEffect{Outcome: outcome})
	if expected.Outcome != intent.Outcome {
		return db.ErrErrandSettlementPending
	}
	var exact bool
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (
 SELECT 1 FROM agent_delivery_effects terminal JOIN agent_delivery_submissions s ON s.id = terminal.submission_id
 JOIN agent_delivery_effects receipt ON receipt.effect_key = ?
 WHERE terminal.effect_type = ? AND terminal.session_id = ? AND terminal.incarnation_id = ?
 AND terminal.submission_id = ? AND terminal.turn_id = ? AND terminal.harness_generation = ? AND terminal.stream_id = ?
 AND terminal.sequence = ? AND terminal.outcome IN ('completed', 'failed', 'cancelled')
 AND (terminal.state IN ('completed', 'block_resolved') OR (terminal.state = 'pending' AND EXISTS (
 SELECT 1 FROM agent_delivery_cursors c WHERE c.stream_id = terminal.stream_id AND c.session_id = terminal.session_id
 AND c.incarnation_id = terminal.incarnation_id AND c.harness_generation = terminal.harness_generation AND c.projected_sequence >= terminal.sequence)))
 AND s.session_id = terminal.session_id AND s.incarnation_id = terminal.incarnation_id
 AND s.harness_generation = terminal.harness_generation AND s.payload_hash = ?
 AND ((s.state = 'interrupted_unknown' AND s.terminal_sequence = 0 AND s.terminal_stream_id = '' AND s.terminal_turn_id = '')
 OR (s.state = terminal.outcome AND s.terminal_sequence = terminal.sequence AND s.terminal_stream_id = terminal.stream_id AND s.terminal_turn_id = terminal.turn_id))
 AND receipt.state = 'completed' AND receipt.outcome = ?
 )`), intent.EffectKey, models.DeliveryTerminalEffectType, p.SessionID, p.IncarnationID, p.SubmissionID, p.TurnID, p.HarnessGeneration, p.StreamID, sequence, p.PayloadHash, intent.Outcome).Scan(&exact)
	if err != nil {
		return err
	}
	if !exact {
		return db.ErrErrandSettlementPending
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE agent_delivery_effects SET state = 'block_resolved', completed_at = ? WHERE effect_key = ? AND state = 'pending'`), intent.CompletedAt, key)
	return err
}

// The assessment and receipt must name the same original admission.
func interruptedAssessmentOwnerMatches(p models.InterruptedDelegationAssessment, taskID string, intent *models.AgentDeliveryEffect) bool {
	return p.TaskID == taskID && p.SessionID == intent.SessionID && p.IncarnationID == intent.IncarnationID && p.TurnID == intent.TurnID && interruptedIntentMatches(intent, p)
}

// ReconcileErrandInterruptedTerminalReceipts repairs assessments left pending
// after an authoritative receipt was committed by an earlier reconciliation.
func (r *Repository) ReconcileErrandInterruptedTerminalReceipts(ctx context.Context, sessionID string) error {
	var saved []string
	if err := r.ro.SelectContext(ctx, &saved, r.ro.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_type = ? AND session_id = ? AND state = 'pending'`), models.InterruptedDelegationEffectType, sessionID); err != nil {
		return err
	}
	for _, raw := range saved {
		p, err := decodeInterruptedDelegation(raw)
		if err != nil {
			return err
		}
		if p.SessionID != sessionID {
			return models.ErrErrandVersionConflict
		}
		if err := r.reconcileInterruptedTerminalReceipt(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) reconcileInterruptedTerminalReceipt(ctx context.Context, p models.InterruptedDelegationAssessment) error {
	return r.withErrandTask(ctx, p.TaskID, func(tx *sqlx.Tx) error {
		if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), p.SessionID); err != nil {
			return err
		}
		intent, err := errandSettlementEffectTx(ctx, tx, p.SessionID, p.IncarnationID, p.TurnID)
		if err != nil {
			return err
		}
		if intent == nil || intent.State != models.DeliveryEffectCompleted {
			return nil
		}
		if err := r.validateErrandSettlementOwner(ctx, tx, p.TaskID, intent); err != nil {
			return err
		}
		return supersedeInterruptedAssessmentTx(ctx, tx, p.TaskID, intent)
	})
}
