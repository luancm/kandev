package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) settleErrandDeliveryTerminal(ctx context.Context, tx *sqlx.Tx, errandID, streamID string, sequence int64, outcome models.DeliverySubmissionState, at time.Time) (bool, error) {
	ownerExists, err := r.lockErrandTerminalOwner(ctx, tx, errandID)
	if err != nil || !ownerExists {
		return false, err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{models.ErrandExecutionIdentity(errandID)}, []string{streamID}); err != nil {
		return false, err
	}
	state, err := r.loadTerminalDeliverySettlementTx(ctx, tx, streamID, sequence, outcome)
	if err != nil || state == nil {
		return false, err
	}
	if err = r.validateErrandTerminalEventTx(ctx, tx, errandID, streamID, sequence, state); err != nil {
		return false, err
	}

	if state.latestSequence > sequence {
		if err = completeSupersededDeliveryEffectTx(ctx, tx, r.db, state.effect.EffectKey, at); err != nil {
			return false, err
		}
		return false, tx.Commit()
	}
	var raw string
	if err = tx.GetContext(ctx, &raw, tx.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_key=? AND effect_type=?`), errandPromptAdmissionEffect+":"+state.effect.SubmissionID, errandPromptAdmissionEffect); err != nil {
		return false, err
	}
	var proof errandPromptProof
	if err = json.Unmarshal([]byte(raw), &proof); err != nil {
		return false, err
	}
	if err = settleErrandSubmissionOutcomeTx(ctx, tx, r.db, state, outcome, at); err != nil {
		return false, err
	}
	if err = closeErrandTerminalTurnTx(ctx, tx, proof, at); err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE agent_delivery_effects SET state=? WHERE effect_key=? AND state=?`), models.DeliveryEffectBlockResolved, state.effect.EffectKey, models.DeliveryEffectPending)
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) lockErrandTerminalOwner(ctx context.Context, tx *sqlx.Tx, errandID string) (bool, error) {
	var taskID string
	err := tx.GetContext(ctx, &taskID, tx.Rebind(`SELECT task_id FROM task_session_errands WHERE id=?`), errandID)
	if err == nil {
		err = r.lockTaskRowInTx(ctx, tx, taskID)
	}
	// Storage retirement can outlive the task and its Errand owner.
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrTaskNotFound) {
		return false, nil
	}
	return err == nil, err
}

func closeErrandTerminalTurnTx(ctx context.Context, tx *sqlx.Tx, proof errandPromptProof, at time.Time) error {
	turnTime := errandTranscriptTime(at)
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_turns SET completed_at=?,updated_at=? WHERE errand_id=? AND id=? AND completed_at IS NULL`),
		turnTime, turnTime, proof.ErrandID, proof.TurnID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_execution_generations SET active_submission_id='',active_turn_id='' WHERE errand_id=? AND generation=? AND execution_id=? AND active_submission_id=? AND active_turn_id=? AND prompt_generation=?`), proof.ErrandID, int64(proof.Generation), proof.ExecutionID, proof.SubmissionID, proof.TurnID, int64(proof.PromptGeneration))
	return err
}

func (r *Repository) releaseUnsentErrandClaimTx(ctx context.Context, tx *sqlx.Tx, errandID string, expected *models.AgentDeliverySubmission) error {
	var raw string
	if err := tx.GetContext(ctx, &raw, tx.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_key=? AND effect_type=?`), errandPromptAdmissionEffect+":"+expected.ID, errandPromptAdmissionEffect); err != nil {
		return err
	}
	var proof errandPromptProof
	if err := json.Unmarshal([]byte(raw), &proof); err != nil {
		return err
	}
	if proof.ErrandID != errandID || proof.SubmissionID != expected.ID || proof.ExecutionID != expected.DispatchAttemptID || proof.IncarnationID != expected.IncarnationID || proof.HarnessGeneration != expected.HarnessGeneration || proof.PayloadHash != expected.PayloadHash {
		return fmt.Errorf("errand unsent proof mismatch")
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_execution_generations SET active_submission_id='',active_turn_id='' WHERE errand_id=? AND generation=? AND execution_id=? AND active_submission_id=? AND active_turn_id=? AND prompt_generation=?`), errandID, int64(proof.Generation), proof.ExecutionID, proof.SubmissionID, proof.TurnID, int64(proof.PromptGeneration))
	return err
}

func settleErrandSubmissionOutcomeTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, state *terminalDeliverySettlement, outcome models.DeliverySubmissionState, at time.Time) error {
	if state.currentState != outcome {
		switch state.currentState {
		case models.DeliverySubmissionCompleted, models.DeliverySubmissionFailed, models.DeliverySubmissionCancelled:
			return fmt.Errorf("errand submission already settled")
		}
		changed, e := updateDeliverySubmissionOutcomeTx(ctx, tx, db, state.effect, outcome, at)
		if e != nil {
			return e
		}
		if !changed {
			return sql.ErrNoRows
		}
	}
	return nil
}

func (r *Repository) validateErrandTerminalEventTx(ctx context.Context, tx *sqlx.Tx, errandID, streamID string, sequence int64, state *terminalDeliverySettlement) error {
	event, _, err := r.loadAgentDeliveryEventTx(ctx, tx, streamID, sequence)
	if err != nil {
		return err
	}
	turn, err := r.errandDeliveryEventAdmissionTx(ctx, tx, errandID, &event)
	if err != nil {
		return err
	}
	if state.effect.TurnID != turn {
		return fmt.Errorf("errand terminal turn mismatch")
	}
	return nil
}
