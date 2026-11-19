package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

const ownedErrandInterruptionEffect = "errand.owned_interrupted_delegation.v1"

type ownedErrandInterruptionReceipt struct {
	errandPromptProof
	Version           int    `json:"version"`
	TaskID            string `json:"task_id"`
	ProjectedSequence int64  `json:"projected_sequence"`
}

func readOwnedErrandInterruptionReceipt(raw, taskID string, prompt errandPromptProof) (ownedErrandInterruptionReceipt, error) {
	var receipt ownedErrandInterruptionReceipt
	if err := json.Unmarshal([]byte(raw), &receipt); err != nil {
		return receipt, err
	}
	if receipt.Version != 1 || receipt.TaskID != taskID || receipt.errandPromptProof != prompt ||
		receipt.ProjectedSequence < 0 || prompt.OriginalRuntime.Validate() != nil {
		return receipt, models.ErrErrandVersionConflict
	}
	return receipt, nil
}

// ReadOwnedErrandInterruption supplies the exact journal entry eligible for
// retirement when an explicit continuation follows confirmed cleanup.
func (r *Repository) ReadOwnedErrandInterruption(ctx context.Context, target models.ErrandTeardownTarget) (*models.ErrandInterruptedAdmission, int64, error) {
	var admissionRaw, interruptionRaw string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`SELECT a.outcome,i.outcome
		FROM agent_delivery_effects a JOIN agent_delivery_effects i ON i.session_id=a.session_id
		 AND i.incarnation_id=a.incarnation_id AND i.harness_generation=a.harness_generation
		 AND i.stream_id=a.stream_id AND i.submission_id=a.submission_id AND i.turn_id=a.turn_id
		WHERE a.effect_key=? AND a.effect_type=? AND a.session_id=? AND a.turn_id=?
		 AND i.effect_key=? AND i.effect_type=? AND i.sequence=0 AND i.state='completed'`),
		errandPromptAdmissionEffect+":"+target.SubmissionID, errandPromptAdmissionEffect,
		models.ErrandExecutionIdentity(target.ErrandID), target.TurnID,
		ownedErrandInterruptionEffect+":"+target.SubmissionID, ownedErrandInterruptionEffect).
		Scan(&admissionRaw, &interruptionRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var prompt errandPromptProof
	if err := json.Unmarshal([]byte(admissionRaw), &prompt); err != nil {
		return nil, 0, err
	}
	receipt, err := readOwnedErrandInterruptionReceipt(interruptionRaw, target.TaskID, prompt)
	if err != nil {
		return nil, 0, err
	}
	if prompt.teardownTarget(receipt.TaskID) != target {
		return nil, 0, models.ErrErrandVersionConflict
	}
	admission := prompt.interruptedAdmission(target.TaskID)
	return &admission, receipt.ProjectedSequence, nil
}

const ownedErrandInterruptionAdmissions = `SELECT e.task_id, a.outcome
	FROM task_session_errands e
	JOIN errand_execution_generations g ON g.errand_id=e.id AND g.execution_id<>''
	JOIN agent_delivery_submissions s ON s.session_id=('errand:' || e.id) AND s.dispatch_attempt_id=g.execution_id
	JOIN agent_delivery_effects a ON a.effect_key=? || s.id AND a.effect_type=? AND a.session_id=s.session_id
	 AND a.incarnation_id=s.incarnation_id AND a.harness_generation=s.harness_generation
	JOIN errand_harness_bindings b ON b.errand_id=e.id AND b.owner_generation=g.generation AND b.execution_id=g.execution_id
	 AND b.stream_id=a.stream_id AND b.incarnation_id=a.incarnation_id AND b.harness_generation=a.harness_generation
	JOIN errand_turns t ON t.errand_id=e.id AND t.id=a.turn_id
	WHERE s.state IN ('accepted','dispatching','interrupted_unknown')
	 AND s.terminal_sequence=0 AND b.native_session_id<>''
	 AND s.payload_hash<>''
`

// ListOwnedErrandInterruptionAdmissions reads only active owned admissions.
// Missing original-process evidence leaves the admission unresolved.
func (r *Repository) ListOwnedErrandInterruptionAdmissions(ctx context.Context) ([]models.ErrandInterruptedAdmission, error) {
	query := ownedErrandInterruptionAdmissions + `
	  AND e.lifecycle<>'cancelled' AND g.active_submission_id=s.id AND g.active_turn_id=t.id AND t.completed_at IS NULL
	  AND g.generation=(SELECT MAX(latest.generation) FROM errand_execution_generations latest WHERE latest.errand_id=e.id)
	  ORDER BY e.task_id,e.id`
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), errandPromptAdmissionEffect+":", errandPromptAdmissionEffect)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var admissions []models.ErrandInterruptedAdmission
	for rows.Next() {
		var taskID, raw string
		if err := rows.Scan(&taskID, &raw); err != nil {
			return nil, err
		}
		var prompt errandPromptProof
		if err := json.Unmarshal([]byte(raw), &prompt); err != nil {
			return nil, err
		}
		if prompt.OriginalRuntime.Validate() == nil && prompt.NativeSessionID != "" {
			admissions = append(admissions, prompt.interruptedAdmission(taskID))
		}
	}
	return admissions, rows.Err()
}

func (p errandPromptProof) interruptedAdmission(taskID string) models.ErrandInterruptedAdmission {
	return models.ErrandInterruptedAdmission{
		TaskID: taskID, TurnID: p.TurnID, SubmissionID: p.SubmissionID,
		PromptGeneration: p.PromptGeneration, PayloadHash: p.PayloadHash, OriginalRuntime: p.OriginalRuntime,
		Binding: models.ErrandHarnessBinding{
			ErrandID: p.ErrandID, Generation: p.Generation, ExecutionID: p.ExecutionID,
			IncarnationID: p.IncarnationID, HarnessGeneration: p.HarnessGeneration,
			StreamID: p.StreamID, NativeSessionID: p.NativeSessionID,
		},
	}
}

// CommitOwnedErrandInterruption stores reconciled interruption separately from
// provider terminal outcomes and closes only the admitted turn.
func (r *Repository) CommitOwnedErrandInterruption(ctx context.Context, admission models.ErrandInterruptedAdmission, projected int64) (bool, error) {
	binding := admission.Binding
	if projected < 0 || admission.TaskID == "" || admission.SubmissionID == "" ||
		admission.OriginalRuntime.Validate() != nil || binding.NativeSessionID == "" || !validErrandHarnessBinding(&binding, 0) {
		return false, models.ErrInvalidErrand
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockTaskRowInTx(ctx, tx, admission.TaskID); err != nil {
		return false, err
	}
	sessionID := models.ErrandExecutionIdentity(binding.ErrandID)
	if err := lockAgentDeliveryWrites(ctx, tx, tx.DriverName(), []string{sessionID}, []string{binding.StreamID}); err != nil {
		return false, err
	}
	changed, err := r.commitOwnedErrandInterruptionTx(ctx, tx, admission, sessionID, projected)
	if err != nil || !changed {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) commitOwnedErrandInterruptionTx(ctx context.Context, tx *sqlx.Tx, admission models.ErrandInterruptedAdmission, sessionID string, projected int64) (bool, error) {
	b := admission.Binding
	query := ownedErrandInterruptionAdmissions + ` AND e.task_id=? AND e.id=? AND g.generation=? AND s.id=?
	 AND b.native_session_id=? AND s.payload_hash=? AND a.stream_id=?`
	var taskID, raw string
	err := tx.QueryRowxContext(ctx, tx.Rebind(query), errandPromptAdmissionEffect+":", errandPromptAdmissionEffect,
		admission.TaskID, b.ErrandID, b.Generation, admission.SubmissionID, b.NativeSessionID, admission.PayloadHash, b.StreamID).
		Scan(&taskID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var prompt errandPromptProof
	if err := json.Unmarshal([]byte(raw), &prompt); err != nil {
		return false, err
	}
	if prompt.interruptedAdmission(taskID) != admission || prompt.DispatchAttemptID != b.ExecutionID {
		return false, models.ErrErrandVersionConflict
	}
	if err := verifyOwnedInterruptionCursor(ctx, tx, admission, sessionID, projected); err != nil {
		return false, err
	}
	return r.recordOwnedInterruptionReceipt(ctx, tx, prompt, taskID, sessionID, projected)
}

func verifyOwnedInterruptionCursor(ctx context.Context, tx *sqlx.Tx, admission models.ErrandInterruptedAdmission, sessionID string, projected int64) error {
	var owner, incarnation string
	var harness, received, applied, remote int64
	err := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT session_id,incarnation_id,harness_generation,received_sequence,projected_sequence,remote_high_water
		FROM agent_delivery_cursors WHERE stream_id=?`), admission.Binding.StreamID).
		Scan(&owner, &incarnation, &harness, &received, &applied, &remote)
	if errors.Is(err, sql.ErrNoRows) && projected == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	if owner != sessionID || incarnation != admission.Binding.IncarnationID || harness != admission.Binding.HarnessGeneration ||
		received != projected || applied != projected || remote > projected {
		return models.ErrErrandVersionConflict
	}
	return nil
}

func (r *Repository) recordOwnedInterruptionReceipt(ctx context.Context, tx *sqlx.Tx, prompt errandPromptProof, taskID, sessionID string, projected int64) (bool, error) {
	var terminalPending bool
	err := tx.GetContext(ctx, &terminalPending, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM agent_delivery_effects
		WHERE effect_type=? AND session_id=? AND stream_id=? AND incarnation_id=? AND harness_generation=?
		  AND submission_id=? AND turn_id=? AND sequence>0)`),
		models.DeliveryTerminalEffectType, sessionID, prompt.StreamID, prompt.IncarnationID, prompt.HarnessGeneration, prompt.SubmissionID, prompt.TurnID)
	if err != nil || terminalPending {
		return false, err
	}
	raw, err := json.Marshal(ownedErrandInterruptionReceipt{errandPromptProof: prompt, Version: 1, TaskID: taskID, ProjectedSequence: projected})
	if err != nil {
		return false, err
	}
	at := r.nowUTC()
	created, err := insertDeliveryEffectTx(ctx, tx, tx.Rebind, &models.AgentDeliveryEffect{
		EffectKey: ownedErrandInterruptionEffect + ":" + prompt.SubmissionID, EffectType: ownedErrandInterruptionEffect,
		StreamID: prompt.StreamID, SessionID: sessionID, IncarnationID: prompt.IncarnationID, HarnessGeneration: prompt.HarnessGeneration,
		SubmissionID: prompt.SubmissionID, TurnID: prompt.TurnID, State: models.DeliveryEffectCompleted, Outcome: string(raw), CreatedAt: at, CompletedAt: &at,
	})
	if err != nil || !created {
		return false, err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE agent_delivery_submissions SET state=?,updated_at=?
		WHERE id=? AND state IN ('accepted','dispatching')`), models.DeliverySubmissionInterruptedUnknown, at, prompt.SubmissionID)
	if err != nil {
		return false, err
	}
	return true, closeErrandTerminalTurnTx(ctx, tx, prompt, at)
}
