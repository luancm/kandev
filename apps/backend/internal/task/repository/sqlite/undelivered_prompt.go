package sqlite

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const unsentSubmissionOwnerSQL = `id = ? AND session_id = ? AND incarnation_id = ?
 AND harness_generation = ? AND owner_generation = ? AND payload_hash = ? AND dispatch_attempt_id = ?`

const unsentSubmissionTerminalSQL = `terminal_stream_id = '' AND terminal_sequence = 0 AND terminal_turn_id = ''
 AND NOT EXISTS (SELECT 1 FROM agent_delivery_inbox sent
   WHERE sent.submission_id = agent_delivery_submissions.id AND sent.terminal <> 0)
 AND NOT EXISTS (SELECT 1 FROM agent_delivery_effects sent
   WHERE sent.effect_type = 'agent_delivery.terminal_settlement' AND sent.submission_id = agent_delivery_submissions.id)`

func validUnsentSubmissionOwner(expected *models.AgentDeliverySubmission) bool {
	return expected != nil && expected.ID != "" && expected.SessionID != "" && expected.IncarnationID != "" &&
		expected.HarnessGeneration > 0 && expected.OwnerGeneration > 0 && expected.PayloadHash != "" && expected.DispatchAttemptID != ""
}

// AgentDeliverySubmissionWasUnsent reads the retained receipt for the captured owner.
// Payload pruning does not erase this identity or authorize a provider terminal outcome.
func (r *Repository) AgentDeliverySubmissionWasUnsent(ctx context.Context, expected *models.AgentDeliverySubmission) (bool, error) {
	if !validUnsentSubmissionOwner(expected) {
		return false, nil
	}
	var found bool
	err := r.db.GetContext(ctx, &found, r.db.Rebind(`SELECT EXISTS (SELECT 1 FROM agent_delivery_submissions WHERE `+
		unsentSubmissionOwnerSQL+` AND state = 'failed' AND outcome = 'failed' AND fully_settled_at IS NOT NULL AND `+
		unsentSubmissionTerminalSQL+`)`), expected.ID, expected.SessionID, expected.IncarnationID,
		expected.HarnessGeneration, expected.OwnerGeneration, expected.PayloadHash, expected.DispatchAttemptID)
	return found, err
}

// FailUnsentAgentDeliverySubmission settles only the observed dispatch reservation.
// The caller must prove that no transport write was attempted.
func (r *Repository) FailUnsentAgentDeliverySubmission(ctx context.Context, expected *models.AgentDeliverySubmission, at time.Time) (bool, error) {
	if !validUnsentSubmissionOwner(expected) {
		return false, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	errandID, isErrand := errandDeliveryIdentity(expected.SessionID)
	if isErrand {
		var taskID string
		if err := tx.GetContext(ctx, &taskID, tx.Rebind(`SELECT task_id FROM task_session_errands WHERE id=?`), errandID); err != nil {
			return false, err
		}
		if err := r.lockTaskRowInTx(ctx, tx, taskID); err != nil {
			return false, err
		}
	} else if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), expected.SessionID); err != nil {
		return false, err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{expected.SessionID}, nil); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE agent_delivery_submissions
 SET state = ?, outcome = ?, fully_settled_at = ?, updated_at = ?
 WHERE `+unsentSubmissionOwnerSQL+` AND state = ? AND `+unsentSubmissionTerminalSQL),
		models.DeliverySubmissionFailed, "failed", at, at, expected.ID, expected.SessionID,
		expected.IncarnationID, expected.HarnessGeneration, expected.OwnerGeneration, expected.PayloadHash, expected.DispatchAttemptID, models.DeliverySubmissionDispatching)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if isErrand && count == 1 {
		if err := r.releaseUnsentErrandClaimTx(ctx, tx, errandID, expected); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return count == 1, nil
}
