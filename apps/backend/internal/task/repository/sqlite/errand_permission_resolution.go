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
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (r *Repository) ClaimErrandPermissionResolution(ctx context.Context, source models.ErrandPermissionSource, audit models.PermissionResolutionAudit, automaticOnly bool) (*models.PermissionResolutionClaimResult, error) {
	if audit.ClaimID == "" || audit.RequestID != source.RequestID || audit.PendingID != source.PendingID {
		return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	tx, message, err := r.beginErrandPermissionResolution(ctx, source, true)
	if errors.Is(err, sql.ErrNoRows) {
		return &models.PermissionResolutionClaimResult{Outcome: models.PermissionClaimNotFound}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	outcome, eligible, err := errandPermissionClaimEligibility(message, automaticOnly)
	if err != nil {
		return nil, err
	}
	if !eligible {
		return &models.PermissionResolutionClaimResult{Outcome: outcome, Message: message}, nil
	}
	if automaticOnly && !matchesErrandAutomaticPermissionProposal(message, audit) {
		return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	audit.Result = models.PermissionResolutionDispatching
	message.Metadata["permission_resolution"] = audit
	if err = r.writeErrandPermissionTx(ctx, tx, source.Owner.ErrandID, message); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &models.PermissionResolutionClaimResult{Outcome: models.PermissionClaimed, Message: message}, nil
}

func matchesErrandAutomaticPermissionProposal(message *models.Message, audit models.PermissionResolutionAudit) bool {
	metadata := message.Metadata
	return audit.Source == models.PermissionSourceAutoApprove && audit.OptionID != "" &&
		(audit.OptionKind == string(streams.PermissionOptionKindAllowOnce) || audit.OptionKind == string(streams.PermissionOptionKindAllowAlways)) &&
		metadata["auto_approval_pending"] == true && metadata["auto_approved_option_id"] == audit.OptionID &&
		metadata["auto_approved_option_kind"] == audit.OptionKind && metadata["auto_approval_source"] == streams.PermissionDecisionSourceAutoApprove
}

func errandPermissionClaimEligibility(message *models.Message, automaticOnly bool) (models.PermissionResolutionClaimOutcome, bool, error) {
	status, _ := message.Metadata["status"].(string)
	presentation, _ := message.Metadata[models.PermissionPresentationMetadataKey].(string)
	humanInput, _ := message.Metadata["permission_human_input"].(bool)
	existing, hasAudit := permissionAuditFromMetadata(message.Metadata)
	if _, present := message.Metadata["permission_resolution"]; present && !hasAudit {
		return "", false, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	if status != "" || hasAudit || (automaticOnly && (humanInput || (presentation != "" && presentation != string(models.PermissionPresentationAutomatic)))) {
		if hasAudit && existing.Result == models.PermissionResolutionDispatching {
			return models.PermissionClaimInProgress, false, nil
		}
		return models.PermissionClaimAlreadyFinal, false, nil
	}
	return "", true, nil
}

func (r *Repository) FinalizeErrandPermissionResolution(ctx context.Context, source models.ErrandPermissionSource, final models.ErrandPermissionFinalization) (*models.PermissionResolutionFinalizeResult, error) {
	if final.FinalizedAt.IsZero() || final.Result == "" || final.Result == models.PermissionResolutionDispatching {
		return nil, fmt.Errorf("invalid permission resolution result")
	}
	if final.ClaimID == "" {
		return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	tx, message, err := r.beginErrandPermissionResolution(ctx, source, false)
	if errors.Is(err, sql.ErrNoRows) {
		return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalizeNotFound}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	existing, ok := permissionAuditFromMetadata(message.Metadata)
	if !ok || existing.ClaimID != final.ClaimID {
		return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalizeClaimMismatch, Message: message}, nil
	}
	if existing.Result != models.PermissionResolutionDispatching {
		return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalizeAlreadyFinal, Message: message}, nil
	}
	finalizedAt := final.FinalizedAt.UTC()
	existing.Result, existing.FinalizedAt = final.Result, &finalizedAt
	message.Metadata["permission_resolution"] = existing
	message.Metadata["status"] = string(final.Status)
	if existing.Source == models.PermissionSourceAutoApprove && final.Status == models.PermissionStatusExpired {
		message.Metadata[models.PermissionPresentationMetadataKey] = string(models.PermissionPresentationUnavailable)
	}
	if err = r.writeErrandPermissionTx(ctx, tx, source.Owner.ErrandID, message); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalized, Message: message}, nil
}

func (r *Repository) beginErrandPermissionResolution(ctx context.Context, source models.ErrandPermissionSource, current bool) (*sqlx.Tx, *models.Message, error) {
	owner := source.Owner
	if !validErrandPermissionSource(source) {
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
	var exact *models.ErrandPermissionSource
	if !current {
		exact = &source
	}
	message, actual, err := r.loadErrandPermissionSourceTx(ctx, tx, owner, source.RequestID, source.PendingID, exact, true)
	if err != nil {
		return fail(err)
	}
	if actual != source {
		return fail(repoerrors.ErrAgentDeliveryIdentityMismatch)
	}
	return tx, message, nil
}

func validErrandPermissionOwner(owner models.ErrandPermissionOwner) bool {
	return owner.ErrandID != "" && owner.TaskID != "" && owner.Generation > 0 && owner.Generation <= math.MaxInt64 && owner.ExecutionID != ""
}

func validErrandPermissionSource(source models.ErrandPermissionSource) bool {
	return validErrandPermissionOwner(source.Owner) && source.IncarnationID != "" && source.HarnessGeneration > 0 && source.StreamID != "" && source.SubmissionID != "" && source.TurnID != "" && source.PromptGeneration > 0 && source.PromptGeneration <= math.MaxInt64 && source.RequestID != "" && source.PendingID != ""
}

func checkCurrentErrandPermissionOwnerTx(ctx context.Context, tx *sqlx.Tx, owner models.ErrandPermissionOwner) error {
	var lifecycle models.ErrandLifecycle
	if err := tx.GetContext(ctx, &lifecycle, tx.Rebind(`SELECT lifecycle FROM task_session_errands WHERE id=?`), owner.ErrandID); err != nil {
		return err
	}
	if lifecycle == models.ErrandLifecycleCancelled {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	var valid bool
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS(SELECT 1 FROM errand_execution_generations g WHERE g.errand_id=? AND g.generation=? AND g.execution_id=? AND g.generation=(SELECT MAX(generation) FROM errand_execution_generations WHERE errand_id=g.errand_id))`), owner.ErrandID, int64(owner.Generation), owner.ExecutionID).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return nil
}

func (r *Repository) loadErrandPermissionSourceTx(ctx context.Context, tx *sqlx.Tx, owner models.ErrandPermissionOwner, requestID, pendingID string, exact *models.ErrandPermissionSource, forUpdate bool) (*models.Message, models.ErrandPermissionSource, error) {
	driver := r.db.DriverName()
	request := dialect.JSONExtractPath(driver, "m.payload", "metadata", "request_id")
	pending := dialect.JSONExtractPath(driver, "m.payload", "metadata", "pending_id")
	typ := dialect.JSONExtract(driver, "m.payload", "type")
	query := fmt.Sprintf(`SELECT m.id,m.payload,admission.outcome,effect.stream_id,effect.incarnation_id,effect.harness_generation,effect.submission_id,effect.turn_id FROM errand_messages m
 JOIN agent_delivery_effects effect ON effect.effect_key=('agent_delivery.errand_permission_owner.v1:' || m.id) AND effect.effect_type='agent_delivery.errand_permission_owner.v1'
 JOIN agent_delivery_effects admission ON admission.effect_key=('agent_delivery.errand_prompt_admission.v1:' || effect.submission_id) AND admission.effect_type='agent_delivery.errand_prompt_admission.v1'
 JOIN errand_harness_bindings b ON b.errand_id=m.errand_id AND b.owner_generation=? AND b.execution_id=? AND b.stream_id=effect.stream_id AND b.incarnation_id=effect.incarnation_id AND b.harness_generation=effect.harness_generation
 WHERE m.errand_id=? AND effect.session_id=? AND effect.turn_id=m.turn_id AND %s='permission_request' AND %s=? AND %s=?`, typ, request, pending)
	args := []any{int64(owner.Generation), owner.ExecutionID, owner.ErrandID, models.ErrandExecutionIdentity(owner.ErrandID), requestID, pendingID}
	if exact != nil {
		query += ` AND m.id=?`
		args = append(args, errandPermissionMessageID(owner.ErrandID, exact.StreamID, exact.SubmissionID, exact.TurnID, requestID))
	}
	if forUpdate && dialect.IsPostgres(driver) {
		query += ` FOR UPDATE OF m`
	}
	rows, err := tx.QueryContext(ctx, tx.Rebind(query), args...)
	if err != nil {
		return nil, models.ErrandPermissionSource{}, err
	}
	defer func() { _ = rows.Close() }()
	evidence, err := scanErrandPermissionEvidence(rows)
	if err != nil {
		return nil, models.ErrandPermissionSource{}, err
	}
	return decodeErrandPermissionEvidence(evidence, owner, requestID, pendingID)
}

type errandPermissionEvidence struct {
	id, raw, proofRaw, stream, incarnation, submission, turn string
	harness                                                  int64
}

func scanErrandPermissionEvidence(rows *sql.Rows) (errandPermissionEvidence, error) {
	var evidence errandPermissionEvidence
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return evidence, err
		}
		return evidence, sql.ErrNoRows
	}
	if err := rows.Scan(&evidence.id, &evidence.raw, &evidence.proofRaw, &evidence.stream, &evidence.incarnation, &evidence.harness, &evidence.submission, &evidence.turn); err != nil {
		return evidence, err
	}
	if rows.Next() {
		return evidence, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return evidence, rows.Err()
}

func decodeErrandPermissionEvidence(evidence errandPermissionEvidence, owner models.ErrandPermissionOwner, requestID, pendingID string) (*models.Message, models.ErrandPermissionSource, error) {
	var source models.ErrandPermissionSource
	var proof errandPromptProof
	if err := json.Unmarshal([]byte(evidence.proofRaw), &proof); err != nil {
		return nil, source, err
	}
	if !errandPermissionProofMatches(proof, owner, evidence) {
		return nil, source, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	source = models.ErrandPermissionSource{Owner: owner, IncarnationID: proof.IncarnationID, HarnessGeneration: proof.HarnessGeneration, StreamID: proof.StreamID, SubmissionID: proof.SubmissionID, TurnID: proof.TurnID, PromptGeneration: proof.PromptGeneration, RequestID: requestID, PendingID: pendingID}
	if !validErrandPermissionSource(source) {
		return nil, source, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	var message models.Message
	decoder := json.NewDecoder(bytes.NewBufferString(evidence.raw))
	decoder.UseNumber()
	if err := decoder.Decode(&message); err != nil {
		return nil, source, err
	}
	if !errandPermissionMessageMatches(message, evidence, source) {
		return nil, source, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return &message, source, nil
}

func errandPermissionProofMatches(proof errandPromptProof, owner models.ErrandPermissionOwner, evidence errandPermissionEvidence) bool {
	return proof.ErrandID == owner.ErrandID && proof.Generation == owner.Generation && proof.ExecutionID == owner.ExecutionID && proof.StreamID == evidence.stream && proof.IncarnationID == evidence.incarnation && proof.HarnessGeneration == evidence.harness && proof.SubmissionID == evidence.submission && proof.TurnID == evidence.turn
}

func errandPermissionMessageMatches(message models.Message, evidence errandPermissionEvidence, source models.ErrandPermissionSource) bool {
	return evidence.id == errandPermissionMessageID(source.Owner.ErrandID, evidence.stream, source.SubmissionID, source.TurnID, source.RequestID) && message.ID == evidence.id && message.AuthorType == models.MessageAuthorAgent && message.AuthorID == source.SubmissionID && message.TurnID == source.TurnID && message.TaskID == source.Owner.TaskID && message.TaskSessionID == ""
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
