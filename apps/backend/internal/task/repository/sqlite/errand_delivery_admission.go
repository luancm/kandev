package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

const errandPromptAdmissionEffect = "agent_delivery.errand_prompt_admission.v1"

func (r *Repository) PrepareErrandDeliverySubmission(ctx context.Context, a *models.ErrandDeliveryAdmission) (bool, error) {
	if !validErrandPromptAdmission(a) {
		return false, fmt.Errorf("invalid errand prompt admission")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var taskID string
	if err = tx.GetContext(ctx, &taskID, tx.Rebind(`SELECT task_id FROM task_session_errands WHERE id=?`), a.ErrandID); err != nil {
		return false, err
	}
	if err = r.taskCleanupBarrierLocked(ctx, tx, taskID); err != nil {
		return false, err
	}
	if err = validateErrandNewWorkTx(ctx, tx, a.ErrandID); err != nil {
		return false, err
	}
	key := models.ErrandExecutionIdentity(a.ErrandID)
	if err = lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{key}, []string{a.StreamID}); err != nil {
		return false, err
	}
	if err = r.ensureAgentDeliveryNoRetirementJob(ctx, tx, key); err != nil {
		return false, err
	}
	if err = r.validateErrandPromptAdmissionTx(ctx, tx, a); err != nil {
		return false, err
	}
	created, err := r.persistErrandPromptAdmissionTx(ctx, tx, a, key)
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return created, nil
}

func (r *Repository) validateErrandPromptAdmissionTx(ctx context.Context, tx *sqlx.Tx, a *models.ErrandDeliveryAdmission) error {
	var valid bool
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM errand_execution_generations g JOIN errand_harness_bindings b ON b.errand_id=g.errand_id AND b.owner_generation=g.generation JOIN errand_turns t ON t.errand_id=g.errand_id WHERE g.errand_id=? AND g.generation=? AND g.execution_id=? AND g.generation=(SELECT MAX(generation) FROM errand_execution_generations WHERE errand_id=g.errand_id) AND b.execution_id=? AND b.incarnation_id=? AND b.harness_generation=? AND b.stream_id=? AND (b.native_session_id=? OR ?='') AND b.binding_sequence=(SELECT MAX(binding_sequence) FROM errand_harness_bindings WHERE errand_id=b.errand_id AND owner_generation=b.owner_generation) AND t.id=? AND ((g.active_submission_id='' AND g.prompt_generation<?) OR (g.active_submission_id=? AND g.active_turn_id=? AND g.prompt_generation=?)))`), a.ErrandID, int64(a.Generation), a.ExecutionID, a.ExecutionID, a.IncarnationID, a.HarnessGeneration, a.StreamID, a.NativeSessionID, a.NativeSessionID, a.TurnID, int64(a.PromptGeneration), a.Submission.ID, a.TurnID, int64(a.PromptGeneration)).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("errand prompt admission identity conflict")
	}
	return nil
}

func (r *Repository) persistErrandPromptAdmissionTx(ctx context.Context, tx *sqlx.Tx, a *models.ErrandDeliveryAdmission, key string) (bool, error) {
	proof := errandPromptProof{ErrandID: a.ErrandID, Generation: a.Generation, ExecutionID: a.ExecutionID, IncarnationID: a.IncarnationID, HarnessGeneration: a.HarnessGeneration, StreamID: a.StreamID, NativeSessionID: a.NativeSessionID, PromptGeneration: a.PromptGeneration, TurnID: a.TurnID, SubmissionID: a.Submission.ID, PayloadHash: a.Submission.PayloadHash, DispatchAttemptID: a.Submission.DispatchAttemptID}
	proof.OriginalRuntime = a.OriginalRuntime
	raw, err := json.Marshal(proof)
	if err != nil {
		return false, err
	}
	effect := &models.AgentDeliveryEffect{EffectKey: errandPromptAdmissionEffect + ":" + a.Submission.ID, StreamID: a.StreamID, EffectType: errandPromptAdmissionEffect, SessionID: key, IncarnationID: a.IncarnationID, HarnessGeneration: a.HarnessGeneration, SubmissionID: a.Submission.ID, TurnID: a.TurnID, Outcome: string(raw)}
	created, err := insertDeliveryEffectTx(ctx, tx, tx.Rebind, effect)
	if err != nil {
		return false, err
	}
	if !created {
		return false, nil
	}
	now := r.nowUTC()
	state := a.Submission.State
	if state == "" {
		state = models.DeliverySubmissionPrepared
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO agent_delivery_submissions (id,session_id,incarnation_id,harness_generation,owner_generation,dispatch_attempt_id,payload_hash,payload,state,outcome,payload_bytes,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`), a.Submission.ID, key, a.IncarnationID, a.HarnessGeneration, a.HarnessGeneration, a.Submission.DispatchAttemptID, a.Submission.PayloadHash, a.Submission.Payload, state, a.Submission.Outcome, len(a.Submission.Payload), now, now)
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_execution_generations SET active_submission_id=?,active_turn_id=?,prompt_generation=? WHERE errand_id=? AND generation=? AND (active_submission_id='' OR active_submission_id=?)`), a.Submission.ID, a.TurnID, int64(a.PromptGeneration), a.ErrandID, int64(a.Generation), a.Submission.ID)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if n != 1 {
		return false, sql.ErrNoRows
	}
	return true, nil
}

func validErrandPromptAdmission(a *models.ErrandDeliveryAdmission) bool {
	if a == nil || a.Submission == nil {
		return false
	}
	return a.ErrandID != "" && a.Generation > 0 && a.Generation <= math.MaxInt64 && a.ExecutionID != "" && a.IncarnationID != "" && a.HarnessGeneration > 0 && a.StreamID != "" && a.PromptGeneration > 0 && a.PromptGeneration <= math.MaxInt64 && a.TurnID != "" && a.Submission.ID != "" && a.Submission.PayloadHash == journal.SubmissionHash(a.Submission.Payload)
}
