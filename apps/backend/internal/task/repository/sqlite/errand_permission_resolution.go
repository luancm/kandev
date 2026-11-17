package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (r *Repository) ClaimErrandPermissionResolution(ctx context.Context, owner models.ErrandExecutionDescriptor, audit models.PermissionResolutionAudit, automaticOnly bool) (*models.PermissionResolutionClaimResult, error) {
	tx, message, err := r.beginErrandPermissionResolution(ctx, owner, audit, true)
	if errors.Is(err, sql.ErrNoRows) {
		return &models.PermissionResolutionClaimResult{Outcome: models.PermissionClaimNotFound}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	status, _ := message.Metadata["status"].(string)
	presentation, _ := message.Metadata[models.PermissionPresentationMetadataKey].(string)
	humanInput, _ := message.Metadata["permission_human_input"].(bool)
	existing, hasAudit := permissionAuditFromMetadata(message.Metadata)
	if _, present := message.Metadata["permission_resolution"]; present && !hasAudit {
		return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	if status != "" || hasAudit || (automaticOnly && (humanInput || (presentation != "" && presentation != string(models.PermissionPresentationAutomatic)))) {
		outcome := models.PermissionClaimAlreadyFinal
		if hasAudit && existing.Result == models.PermissionResolutionDispatching {
			outcome = models.PermissionClaimInProgress
		}
		return &models.PermissionResolutionClaimResult{Outcome: outcome, Message: message}, nil
	}
	audit.Result = models.PermissionResolutionDispatching
	message.Metadata["permission_resolution"] = audit
	if err = r.writeErrandPermissionTx(ctx, tx, owner.ErrandID, message); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &models.PermissionResolutionClaimResult{Outcome: models.PermissionClaimed, Message: message}, nil
}

func (r *Repository) FinalizeErrandPermissionResolution(ctx context.Context, owner models.ErrandExecutionDescriptor, final models.ErrandPermissionFinalization) (*models.PermissionResolutionFinalizeResult, error) {
	if final.FinalizedAt.IsZero() || final.Result == "" || final.Result == models.PermissionResolutionDispatching {
		return nil, fmt.Errorf("invalid permission resolution result")
	}
	audit := models.PermissionResolutionAudit{RequestID: final.RequestID, PendingID: final.PendingID, ClaimID: final.ClaimID}
	tx, message, err := r.beginErrandPermissionResolution(ctx, owner, audit, false)
	if errors.Is(err, sql.ErrNoRows) {
		return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalizeNotFound}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	existing, ok := permissionAuditFromMetadata(message.Metadata)
	if !ok || existing.ClaimID != audit.ClaimID {
		return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalizeClaimMismatch, Message: message}, nil
	}
	if existing.Result != models.PermissionResolutionDispatching {
		return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalizeAlreadyFinal, Message: message}, nil
	}
	finalizedAt := final.FinalizedAt.UTC()
	existing.Result, existing.FinalizedAt = final.Result, &finalizedAt
	message.Metadata["permission_resolution"] = existing
	message.Metadata["status"] = string(final.Status)
	if err = r.writeErrandPermissionTx(ctx, tx, owner.ErrandID, message); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalized, Message: message}, nil
}

func (r *Repository) beginErrandPermissionResolution(ctx context.Context, owner models.ErrandExecutionDescriptor, audit models.PermissionResolutionAudit, current bool) (*sqlx.Tx, *models.Message, error) {
	if !validErrandPermissionOwner(owner) || audit.RequestID == "" || audit.PendingID == "" || audit.ClaimID == "" {
		return nil, nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (*sqlx.Tx, *models.Message, error) { _ = tx.Rollback(); return nil, nil, err }
	var taskID string
	if err = tx.GetContext(ctx, &taskID, tx.Rebind(`SELECT task_id FROM task_session_errands WHERE id=?`), owner.ErrandID); err != nil {
		return fail(err)
	}
	if owner.TaskID != "" && owner.TaskID != taskID {
		return fail(repoerrors.ErrAgentDeliveryIdentityMismatch)
	}
	if err = r.lockTaskRowInTx(ctx, tx, taskID); err != nil {
		return fail(err)
	}
	if current {
		if err = checkCurrentErrandPermissionOwnerTx(ctx, tx, owner); err != nil {
			return fail(err)
		}
	}
	message, err := r.loadErrandPermissionForResolutionTx(ctx, tx, owner, audit, taskID)
	if err != nil {
		return fail(err)
	}
	return tx, message, nil
}

func validErrandPermissionOwner(owner models.ErrandExecutionDescriptor) bool {
	return owner.ErrandID != "" && owner.Generation > 0 && owner.Generation <= math.MaxInt64 && owner.ExecutionID != "" && owner.ActiveTurnID != "" && owner.ActiveSubmissionID != "" && owner.PromptGeneration > 0 && owner.PromptGeneration <= math.MaxInt64
}

func checkCurrentErrandPermissionOwnerTx(ctx context.Context, tx *sqlx.Tx, owner models.ErrandExecutionDescriptor) error {
	var lifecycle models.ErrandLifecycle
	if err := tx.GetContext(ctx, &lifecycle, tx.Rebind(`SELECT lifecycle FROM task_session_errands WHERE id=?`), owner.ErrandID); err != nil {
		return err
	}
	if lifecycle == models.ErrandLifecycleCancelled {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	var valid bool
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS(SELECT 1 FROM errand_execution_generations g WHERE g.errand_id=? AND g.generation=? AND g.execution_id=? AND g.active_turn_id=? AND g.active_submission_id=? AND g.prompt_generation=? AND g.generation=(SELECT MAX(generation) FROM errand_execution_generations WHERE errand_id=g.errand_id))`), owner.ErrandID, int64(owner.Generation), owner.ExecutionID, owner.ActiveTurnID, owner.ActiveSubmissionID, int64(owner.PromptGeneration)).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return nil
}

func (r *Repository) loadErrandPermissionForResolutionTx(ctx context.Context, tx *sqlx.Tx, owner models.ErrandExecutionDescriptor, audit models.PermissionResolutionAudit, taskID string) (*models.Message, error) {
	driver := r.db.DriverName()
	request := dialect.JSONExtractPath(driver, "m.payload", "metadata", "request_id")
	pending := dialect.JSONExtractPath(driver, "m.payload", "metadata", "pending_id")
	typ := dialect.JSONExtract(driver, "m.payload", "type")
	query := fmt.Sprintf(`SELECT m.id,m.payload,admission.outcome,effect.stream_id,effect.incarnation_id,effect.harness_generation FROM errand_messages m
 JOIN agent_delivery_effects effect ON effect.effect_key=('agent_delivery.errand_permission_owner.v1:' || m.id) AND effect.effect_type='agent_delivery.errand_permission_owner.v1'
 JOIN agent_delivery_effects admission ON admission.effect_key=('agent_delivery.errand_prompt_admission.v1:' || effect.submission_id) AND admission.effect_type='agent_delivery.errand_prompt_admission.v1'
 JOIN errand_harness_bindings b ON b.errand_id=m.errand_id AND b.owner_generation=? AND b.execution_id=? AND b.stream_id=effect.stream_id AND b.incarnation_id=effect.incarnation_id AND b.harness_generation=effect.harness_generation
 WHERE m.errand_id=? AND m.turn_id=? AND effect.session_id=? AND effect.submission_id=? AND effect.turn_id=m.turn_id AND %s='permission_request' AND %s=? AND %s=?`, typ, request, pending)
	if dialect.IsPostgres(driver) {
		query += ` FOR UPDATE OF m`
	}
	rows, err := tx.QueryContext(ctx, tx.Rebind(query), int64(owner.Generation), owner.ExecutionID, owner.ErrandID, owner.ActiveTurnID, models.ErrandExecutionIdentity(owner.ErrandID), owner.ActiveSubmissionID, audit.RequestID, audit.PendingID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	evidence, err := scanErrandPermissionEvidence(rows)
	if err != nil {
		return nil, err
	}
	return decodeErrandPermissionEvidence(evidence, owner, audit.RequestID, taskID)
}

type errandPermissionEvidence struct {
	id, raw, proofRaw, stream, incarnation string
	harness                                int64
}

func scanErrandPermissionEvidence(rows *sql.Rows) (errandPermissionEvidence, error) {
	var evidence errandPermissionEvidence
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return evidence, err
		}
		return evidence, sql.ErrNoRows
	}
	if err := rows.Scan(&evidence.id, &evidence.raw, &evidence.proofRaw, &evidence.stream, &evidence.incarnation, &evidence.harness); err != nil {
		return evidence, err
	}
	if rows.Next() {
		return evidence, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return evidence, rows.Err()
}

func decodeErrandPermissionEvidence(evidence errandPermissionEvidence, owner models.ErrandExecutionDescriptor, requestID, taskID string) (*models.Message, error) {
	var proof errandPromptProof
	if err := json.Unmarshal([]byte(evidence.proofRaw), &proof); err != nil {
		return nil, err
	}
	if !errandPermissionProofMatches(proof, owner, evidence) {
		return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	var message models.Message
	decoder := json.NewDecoder(bytes.NewBufferString(evidence.raw))
	decoder.UseNumber()
	if err := decoder.Decode(&message); err != nil {
		return nil, err
	}
	if !errandPermissionMessageMatches(message, evidence, owner, requestID, taskID) {
		return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return &message, nil
}

func errandPermissionProofMatches(proof errandPromptProof, owner models.ErrandExecutionDescriptor, evidence errandPermissionEvidence) bool {
	return proof.ErrandID == owner.ErrandID && proof.Generation == owner.Generation && proof.ExecutionID == owner.ExecutionID && proof.PromptGeneration == owner.PromptGeneration && proof.SubmissionID == owner.ActiveSubmissionID && proof.TurnID == owner.ActiveTurnID && proof.StreamID == evidence.stream && proof.IncarnationID == evidence.incarnation && proof.HarnessGeneration == evidence.harness
}

func errandPermissionMessageMatches(message models.Message, evidence errandPermissionEvidence, owner models.ErrandExecutionDescriptor, requestID, taskID string) bool {
	return evidence.id == errandPermissionMessageID(owner.ErrandID, evidence.stream, owner.ActiveSubmissionID, owner.ActiveTurnID, requestID) && message.ID == evidence.id && message.AuthorType == models.MessageAuthorAgent && message.AuthorID == owner.ActiveSubmissionID && message.TurnID == owner.ActiveTurnID && message.TaskID == taskID && message.TaskSessionID == ""
}

func (r *Repository) writeErrandPermissionTx(ctx context.Context, tx *sqlx.Tx, errandID string, message *models.Message) error {
	message.UpdatedAt = errandTranscriptTime(r.nowUTC())
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_messages SET payload=?,updated_at=? WHERE errand_id=? AND id=? AND turn_id=?`), string(payload), message.UpdatedAt, errandID, message.ID, message.TurnID)
	if err != nil {
		return err
	}
	return errandTranscriptUpdated(result)
}
