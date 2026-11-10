package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const interruptedDelegationProvenance = "original_owner_terminated"

// PreserveErrandInterruptedDelegation must precede removal of the running row.
// Unsupported historical evidence cannot authorize a settlement.
func (r *Repository) PreserveErrandInterruptedDelegation(ctx context.Context, running *models.ExecutorRunning) error {
	if running == nil {
		return nil
	}
	session, err := r.GetTaskSession(ctx, running.SessionID)
	if err != nil {
		return err
	}
	if !models.IsErrandSession(session) {
		return nil
	}
	raw, err := json.Marshal(running.Metadata["delivery_prompt_turn_binding"])
	if err != nil {
		return err
	}
	var proof models.InterruptedDelegationAssessment
	if json.Unmarshal(raw, &proof) != nil || !completeInterruptedDelegation(proof) {
		return nil
	}
	if proof.TaskID != running.TaskID || proof.SessionID != running.SessionID || proof.IncarnationID != session.QueueIncarnationID {
		return models.ErrErrandVersionConflict
	}
	dead, err := processidentity.OriginalOwnerTerminated(proof.OriginalRuntime)
	if errors.Is(err, processidentity.ErrUnverifiableIdentity) {
		// Unsupported historical proof cannot certify death or create an assessment.
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect original errand owner: %w", err)
	}
	if !dead {
		return nil
	}
	return r.preserveInterruptedSubmission(ctx, proof)
}

func completeInterruptedDelegation(p models.InterruptedDelegationAssessment) bool {
	return p.TaskID != "" && p.SessionID != "" && p.ExecutionID != "" && p.IncarnationID != "" && p.HarnessGeneration > 0 && p.PromptGeneration > 0 && p.StreamID != "" && p.SubmissionID != "" && p.TurnID != "" && p.NativeSessionID != "" && p.OriginalRuntime.Validate() == nil
}

func (r *Repository) saveInterruptedDelegation(ctx context.Context, p models.InterruptedDelegationAssessment) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return r.withErrandTask(ctx, p.TaskID, func(tx *sqlx.Tx) error {
		if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), p.SessionID); err != nil {
			return err
		}
		if err := verifyInterruptedAdmissionTx(ctx, tx, p); err != nil {
			return err
		}
		intent, err := errandSettlementEffectTx(ctx, tx, p.SessionID, p.IncarnationID, p.TurnID)
		if err != nil {
			return err
		}
		if !interruptedIntentMatches(intent, p) {
			return db.ErrErrandSettlementPending
		}
		if err := r.validateErrandSettlementOwner(ctx, tx, p.TaskID, intent); err != nil {
			return err
		}
		if intent.State == models.DeliveryEffectCompleted || strings.Contains(intent.Outcome, ":") {
			return nil
		}
		terminal, err := interruptedDelegationTerminalTx(ctx, tx, intent)
		if err != nil || terminal {
			return err
		}
		key := "errand.interruption:" + intent.EffectKey
		_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO agent_delivery_effects
   (effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome, created_at)
   VALUES (?, ?, 0, ?, 'pending', ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (effect_key) DO NOTHING`), key, p.StreamID, models.InterruptedDelegationEffectType, p.SessionID, p.IncarnationID, p.HarnessGeneration, p.SubmissionID, p.TurnID, string(raw), r.nowUTC())
		if err != nil {
			return err
		}
		return verifyInterruptedAssessmentTx(ctx, tx, key, string(raw), p)
	})
}

func interruptedIntentMatches(intent *models.AgentDeliveryEffect, p models.InterruptedDelegationAssessment) bool {
	return intent != nil && intent.SubmissionID == p.SubmissionID && intent.HarnessGeneration == int64(p.HarnessGeneration) && intent.StreamID == p.StreamID && (settlementOrigin(intent.Outcome) == errandTerminalParentOrigin || settlementOrigin(intent.Outcome) == authorKindUser)
}

// ConsumeErrandInterruptedDelegations verifies a fresh authenticated snapshot;
// the pending effect itself cannot supply recovery completion authority.
func (r *Repository) ConsumeErrandInterruptedDelegations(ctx context.Context, proof *models.NativeResumeDeliveryEvidence) error {
	if !validNativeResumeProof(proof) {
		return models.ErrInvalidErrand
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`SELECT effect_key, outcome FROM agent_delivery_effects WHERE effect_type = ? AND session_id = ? AND state = 'pending'`), models.InterruptedDelegationEffectType, proof.SessionID)
	if err != nil {
		return err
	}
	type candidate struct{ key, raw string }
	var pending []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.key, &c.raw); err != nil {
			_ = rows.Close()
			return err
		}
		pending = append(pending, c)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, c := range pending {
		p, err := decodeInterruptedDelegation(c.raw)
		if err != nil {
			return err
		}
		if err := r.consumeInterruptedDelegation(ctx, c.key, c.raw, p, proof); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) consumeInterruptedDelegation(ctx context.Context, key, raw string, p models.InterruptedDelegationAssessment, proof *models.NativeResumeDeliveryEvidence) error {
	if p.SessionID != proof.SessionID || p.IncarnationID != proof.IncarnationID || p.NativeSessionID != proof.NativeSessionID || p.ExecutionID == proof.ExecutionID {
		return models.ErrErrandVersionConflict
	}
	if !interruptedSubmissionCovered(p, proof) {
		return db.ErrErrandSettlementPending
	}
	return r.withErrandTask(ctx, p.TaskID, func(tx *sqlx.Tx) error {
		if err := r.lockNativeResumeProofTx(ctx, tx, proof); err != nil {
			return err
		}
		valid, err := r.verifyNativeResumeSnapshotTx(ctx, tx, proof)
		if err != nil {
			return err
		}
		if !valid {
			return db.ErrErrandSettlementPending
		}
		intent, err := errandSettlementEffectTx(ctx, tx, p.SessionID, p.IncarnationID, p.TurnID)
		if err != nil {
			return err
		}
		if !interruptedIntentMatches(intent, p) {
			return db.ErrErrandSettlementPending
		}
		if err := r.validateErrandSettlementOwner(ctx, tx, p.TaskID, intent); err != nil {
			return err
		}
		if key != "errand.interruption:"+intent.EffectKey {
			return models.ErrErrandVersionConflict
		}
		if err := verifyInterruptedAssessmentTx(ctx, tx, key, raw, p); err != nil {
			return err
		}
		return r.sealInterruptedDelegationTx(ctx, tx, key, raw, intent)
	})
}

func (r *Repository) sealInterruptedDelegationTx(ctx context.Context, tx *sqlx.Tx, key, raw string, intent *models.AgentDeliveryEffect) error {
	terminal, err := interruptedDelegationTerminalTx(ctx, tx, intent)
	if err != nil {
		return err
	}
	if terminal {
		return db.ErrErrandSettlementPending
	}
	outcome := settlementOrigin(intent.Outcome) + ":interrupted"
	if strings.Contains(intent.Outcome, ":") && intent.Outcome != outcome {
		return models.ErrErrandVersionConflict
	}
	now := r.nowUTC()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_session_turns SET completed_at = COALESCE(completed_at, ?), updated_at = ? WHERE id = ? AND task_session_id = ?`), now, now, intent.TurnID, intent.SessionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE agent_delivery_effects SET outcome = ? WHERE effect_key = ? AND state = 'pending'`), outcome, intent.EffectKey); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE agent_delivery_effects SET state = 'completed', completed_at = ? WHERE effect_key = ? AND outcome = ? AND state = 'pending'`), now, key, raw)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("%w: recovery assessment changed", models.ErrErrandVersionConflict)
	}
	return nil
}

func verifyInterruptedAdmissionTx(ctx context.Context, tx *sqlx.Tx, p models.InterruptedDelegationAssessment) error {
	var raw string
	query := `SELECT metadata FROM executors_running WHERE session_id = ? AND task_id = ?`
	if dialect.IsPostgres(tx.DriverName()) {
		query += forUpdateClause
	}
	if err := tx.QueryRowContext(ctx, tx.Rebind(query), p.SessionID, p.TaskID).Scan(&raw); err != nil {
		return err
	}
	var metadata map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &metadata) != nil {
		return models.ErrErrandVersionConflict
	}
	var binding models.InterruptedDelegationAssessment
	if json.Unmarshal(metadata["delivery_prompt_turn_binding"], &binding) != nil {
		return models.ErrErrandVersionConflict
	}
	binding.Version, binding.Provenance, binding.PayloadHash = p.Version, p.Provenance, p.PayloadHash
	if binding != p {
		return models.ErrErrandVersionConflict
	}
	var valid bool
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM agent_delivery_submissions WHERE id = ? AND session_id = ? AND incarnation_id = ? AND harness_generation = ? AND payload_hash = ?)`), p.SubmissionID, p.SessionID, p.IncarnationID, p.HarnessGeneration, p.PayloadHash).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return models.ErrErrandVersionConflict
	}
	return nil
}

func (r *Repository) HasPendingInterruptedDelegation(ctx context.Context, sessionID string) (bool, error) {
	var pending bool
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT EXISTS (SELECT 1 FROM agent_delivery_effects WHERE effect_type = ? AND session_id = ? AND state = 'pending')`), models.InterruptedDelegationEffectType, sessionID).Scan(&pending)
	return pending, err
}

// A consumed assessment can still await its exact result receipt after a
// result-write failure. That retry no longer requires a live native peer.
func (r *Repository) HasUnsettledInterruptedDelegation(ctx context.Context, sessionID string) (bool, error) {
	var pending bool
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT EXISTS (
 SELECT 1 FROM agent_delivery_effects assessment WHERE assessment.effect_type = ? AND assessment.session_id = ?
 AND (assessment.state = 'pending' OR (assessment.state = 'completed' AND EXISTS (
 SELECT 1 FROM agent_delivery_effects intent WHERE intent.effect_type = ? AND intent.session_id = assessment.session_id
 AND intent.incarnation_id = assessment.incarnation_id AND intent.turn_id = assessment.turn_id
 AND intent.submission_id = assessment.submission_id AND intent.state = 'pending'))))`), models.InterruptedDelegationEffectType, sessionID, db.ErrandTurnSettlementEffect).Scan(&pending)
	return pending, err
}

// GetInterruptedDelegationAssessment restores only the exact saved original
// owner. It supplies no proof that native retirement has completed.
func (r *Repository) GetInterruptedDelegationAssessment(ctx context.Context, sessionID, submissionID string) (*models.InterruptedDelegationAssessment, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`SELECT outcome, incarnation_id, harness_generation, stream_id, turn_id FROM agent_delivery_effects WHERE effect_type = ? AND session_id = ? AND submission_id = ? AND state = 'pending' AND sequence = 0`), models.InterruptedDelegationEffectType, sessionID, submissionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var found *models.InterruptedDelegationAssessment
	for rows.Next() {
		if found != nil {
			return nil, db.ErrErrandSettlementPending
		}
		var raw, incarnation, stream, turn string
		var generation uint64
		if err := rows.Scan(&raw, &incarnation, &generation, &stream, &turn); err != nil {
			return nil, err
		}
		p, err := decodeInterruptedDelegation(raw)
		if err != nil {
			return nil, err
		}
		if p.SessionID != sessionID || p.SubmissionID != submissionID || p.IncarnationID != incarnation || p.HarnessGeneration != generation || p.StreamID != stream || p.TurnID != turn {
			return nil, models.ErrErrandVersionConflict
		}
		found = &p
	}
	return found, rows.Err()
}

func (r *Repository) preserveInterruptedSubmission(ctx context.Context, proof models.InterruptedDelegationAssessment) error {
	submission, err := r.GetAgentDeliverySubmission(ctx, proof.SubmissionID)
	if err != nil {
		return err
	}
	if submission.SessionID != proof.SessionID || submission.IncarnationID != proof.IncarnationID || submission.HarnessGeneration != int64(proof.HarnessGeneration) {
		return models.ErrErrandVersionConflict
	}
	if submission.State != models.DeliverySubmissionInterruptedUnknown && submission.State != models.DeliverySubmissionDispatching {
		return nil
	}
	proof.PayloadHash = submission.PayloadHash
	proof.Version, proof.Provenance = 1, interruptedDelegationProvenance
	return r.saveInterruptedDelegation(ctx, proof)
}

func interruptedSubmissionCovered(p models.InterruptedDelegationAssessment, proof *models.NativeResumeDeliveryEvidence) bool {
	for _, s := range proof.Submissions {
		if s.ID == p.SubmissionID && s.HarnessGeneration == int64(p.HarnessGeneration) && s.StreamID == p.StreamID && s.PayloadHash == p.PayloadHash && s.State == models.DeliverySubmissionInterruptedUnknown && s.TerminalSequence == 0 && s.TerminalStreamID == "" {
			return true
		}
	}
	return false
}

func decodeInterruptedDelegation(raw string) (models.InterruptedDelegationAssessment, error) {
	var p models.InterruptedDelegationAssessment
	if json.Unmarshal([]byte(raw), &p) != nil || !completeInterruptedDelegation(p) || p.Version != 1 || p.Provenance != interruptedDelegationProvenance || p.PayloadHash == "" {
		return p, models.ErrInvalidErrand
	}
	return p, nil
}

func verifyInterruptedAssessmentTx(ctx context.Context, tx *sqlx.Tx, key, raw string, p models.InterruptedDelegationAssessment) error {
	var exact bool
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM agent_delivery_effects WHERE effect_key = ? AND effect_type = ? AND sequence = 0 AND stream_id = ? AND session_id = ? AND incarnation_id = ? AND harness_generation = ? AND submission_id = ? AND turn_id = ? AND outcome = ? AND state = 'pending')`), key, models.InterruptedDelegationEffectType, p.StreamID, p.SessionID, p.IncarnationID, p.HarnessGeneration, p.SubmissionID, p.TurnID, raw).Scan(&exact)
	if err != nil {
		return err
	}
	if !exact {
		return models.ErrErrandVersionConflict
	}

	return nil
}

func interruptedDelegationTerminalTx(ctx context.Context, tx *sqlx.Tx, intent *models.AgentDeliveryEffect) (bool, error) {
	var terminal bool
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM agent_delivery_effects WHERE effect_type = ? AND session_id = ? AND incarnation_id = ? AND submission_id = ? AND turn_id = ?)`), models.DeliveryTerminalEffectType, intent.SessionID, intent.IncarnationID, intent.SubmissionID, intent.TurnID).Scan(&terminal)

	return terminal, err
}
