package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// RepairAgentDeliveryTerminalIdentity fills only an empty projected identity
// from the retained terminal and its immutable prompt admission.
func (r *Repository) RepairAgentDeliveryTerminalIdentity(
	ctx context.Context,
	expected *models.AgentDeliveryEffect,
) (*models.AgentDeliveryEffect, error) {
	if expected == nil || expected.StreamID == "" || expected.Sequence <= 0 || expected.SessionID == "" {
		return nil, fmt.Errorf("terminal delivery identity is required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := r.loadTerminalDeliveryEffectTx(ctx, tx, expected.StreamID, expected.Sequence)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, repoerrors.ErrAgentDeliveryEffectNotFound
	}
	if !sameTerminalDeliveryEffectIdentity(current, expected) {
		return nil, repoerrors.ErrAgentDeliveryEffectConflict
	}
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), current.SessionID); err != nil {
		return nil, err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{current.SessionID}, []string{current.StreamID}); err != nil {
		return nil, err
	}
	current, err = r.repairAgentDeliveryTerminalIdentityTx(ctx, tx, current.StreamID, current.Sequence, expected, false)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return current, nil
}

func (r *Repository) repairAgentDeliveryTerminalIdentityTx(
	ctx context.Context,
	tx *sqlx.Tx,
	streamID string,
	sequence int64,
	expected *models.AgentDeliveryEffect,
	allowMissing bool,
) (*models.AgentDeliveryEffect, error) {
	current, err := r.loadTerminalDeliveryEffectTx(ctx, tx, streamID, sequence)
	if err != nil {
		return nil, err
	}
	if current == nil {
		if allowMissing {
			return nil, nil
		}
		return nil, repoerrors.ErrAgentDeliveryEffectNotFound
	}
	if expected != nil && !sameTerminalDeliveryEffectIdentity(current, expected) {
		return nil, repoerrors.ErrAgentDeliveryEffectConflict
	}
	payloadTurn, admittedTurn, err := r.agentDeliveryTerminalIdentityEvidenceTx(ctx, tx, current)
	if err != nil {
		return nil, err
	}
	if current.TurnID != "" {
		if err := validateExistingTerminalTurnIdentity(current.TurnID, payloadTurn, admittedTurn, expected); err != nil {
			return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
		}
		return current, nil
	}
	if !terminalIdentityCanBeRepaired(current, payloadTurn, admittedTurn, expected) {
		return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return r.setAgentDeliveryTerminalTurnTx(ctx, tx, current, admittedTurn)
}

func (r *Repository) agentDeliveryTerminalIdentityEvidenceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	current *models.AgentDeliveryEffect,
) (string, string, error) {
	terminal, projectedAt, err := r.loadAgentDeliveryEventTx(ctx, tx, current.StreamID, current.Sequence)
	if err != nil {
		return "", "", err
	}
	if !projectedAt.Valid || !terminal.Terminal || terminal.SessionID != current.SessionID ||
		terminal.IncarnationID != current.IncarnationID || terminal.HarnessGeneration != current.HarnessGeneration ||
		terminal.SubmissionID != current.SubmissionID {
		return "", "", repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	derived, err := deliveryTerminalSettlementEffect(&terminal)
	if err != nil {
		return "", "", err
	}
	if !sameTerminalDeliveryEffectIdentity(current, derived) {
		return "", "", repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	var payload streams.AgentEvent
	if err := json.Unmarshal(terminal.Payload, &payload); err != nil {
		return "", "", fmt.Errorf("decode retained terminal for identity repair: %w", err)
	}
	admittedTurn, _, err := r.agentDeliveryEventAdmissionTx(ctx, tx, &terminal)
	if err != nil {
		return "", "", err
	}
	return payload.TurnID, admittedTurn, nil
}

func validateExistingTerminalTurnIdentity(currentTurn, payloadTurn, admittedTurn string, expected *models.AgentDeliveryEffect) error {
	if payloadTurn != "" && payloadTurn != currentTurn || admittedTurn != "" && admittedTurn != currentTurn ||
		payloadTurn == "" && admittedTurn == "" || expected != nil && expected.TurnID != "" && expected.TurnID != currentTurn {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return nil
}

func terminalIdentityCanBeRepaired(current *models.AgentDeliveryEffect, payloadTurn, admittedTurn string, expected *models.AgentDeliveryEffect) bool {
	return admittedTurn != "" && (payloadTurn == "" || payloadTurn == admittedTurn) &&
		(expected == nil || expected.TurnID == "" || expected.TurnID == admittedTurn) &&
		current.State != models.DeliveryEffectCompleted
}

func (r *Repository) setAgentDeliveryTerminalTurnTx(
	ctx context.Context,
	tx *sqlx.Tx,
	current *models.AgentDeliveryEffect,
	admittedTurn string,
) (*models.AgentDeliveryEffect, error) {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_effects SET turn_id = ?
		WHERE effect_key = ? AND stream_id = ? AND sequence = ? AND effect_type = ?
		  AND state != ? AND session_id = ? AND incarnation_id = ? AND harness_generation = ?
		  AND submission_id = ? AND turn_id = '' AND outcome = ?`),
		admittedTurn, current.EffectKey, current.StreamID, current.Sequence, models.DeliveryTerminalEffectType,
		models.DeliveryEffectCompleted, current.SessionID, current.IncarnationID, current.HarnessGeneration,
		current.SubmissionID, current.Outcome)
	if err != nil {
		return nil, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if updated != 1 {
		return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	current.TurnID = admittedTurn
	return current, nil
}

func sameTerminalDeliveryEffectIdentity(left, right *models.AgentDeliveryEffect) bool {
	return left != nil && right != nil && left.EffectKey == right.EffectKey &&
		left.StreamID == right.StreamID && left.Sequence == right.Sequence &&
		left.EffectType == right.EffectType && left.SessionID == right.SessionID &&
		left.IncarnationID == right.IncarnationID && left.HarnessGeneration == right.HarnessGeneration &&
		left.SubmissionID == right.SubmissionID && left.Outcome == right.Outcome
}
