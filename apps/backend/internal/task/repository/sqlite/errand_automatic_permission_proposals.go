package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// ListErrandAutomaticPermissionProposals reads unclaimed proposals for one
// current, admitted turn. Approval still requires a live selection and claim.
func (r *Repository) ListErrandAutomaticPermissionProposals(ctx context.Context, owner models.ErrandPermissionOwner, turnID, submissionID string) ([]streams.AgentEvent, error) {
	if !validErrandPermissionOwner(owner) || turnID == "" || submissionID == "" {
		return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	driver := r.ro.DriverName()
	typ := dialect.JSONExtract(driver, "m.payload", "type")
	presentation := dialect.JSONExtractPath(driver, "m.payload", "metadata", models.PermissionPresentationMetadataKey)
	query := fmt.Sprintf(`SELECT m.payload,g.prompt_generation
 FROM task_session_errands e
 JOIN errand_execution_generations g ON g.errand_id=e.id AND g.generation=(
  SELECT MAX(latest.generation) FROM errand_execution_generations latest WHERE latest.errand_id=e.id)
 JOIN errand_harness_bindings b ON b.errand_id=g.errand_id AND b.owner_generation=g.generation
  AND b.execution_id=g.execution_id AND b.binding_sequence=(SELECT MAX(latest.binding_sequence)
   FROM errand_harness_bindings latest WHERE latest.errand_id=g.errand_id AND latest.owner_generation=g.generation)
 JOIN errand_messages m ON m.errand_id=g.errand_id AND m.turn_id=g.active_turn_id
 JOIN agent_delivery_effects effect ON effect.effect_key=('agent_delivery.errand_permission_owner.v1:' || m.id)
  AND effect.effect_type='agent_delivery.errand_permission_owner.v1' AND effect.session_id=('errand:' || e.id)
  AND effect.turn_id=m.turn_id AND effect.submission_id=g.active_submission_id
  AND effect.stream_id=b.stream_id AND effect.incarnation_id=b.incarnation_id AND effect.harness_generation=b.harness_generation
 WHERE e.id=? AND e.task_id=? AND e.lifecycle<>'cancelled' AND g.generation=? AND g.execution_id=?
  AND g.active_turn_id=? AND g.active_submission_id=? AND %s='permission_request' AND %s='automatic'
 ORDER BY m.created_at,m.id`, typ, presentation)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), owner.ErrandID, owner.TaskID, int64(owner.Generation), owner.ExecutionID, turnID, submissionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var proposals []streams.AgentEvent
	for rows.Next() {
		var payload []byte
		var generation uint64
		if err := rows.Scan(&payload, &generation); err != nil {
			return nil, err
		}
		proposal, err := decodeErrandAutomaticPermissionProposal(payload, turnID, submissionID, generation)
		if err != nil {
			return nil, err
		}
		if proposal.RequestID != "" {
			proposals = append(proposals, proposal)
		}
	}
	return proposals, rows.Err()
}

func decodeErrandAutomaticPermissionProposal(payload []byte, turnID, submissionID string, generation uint64) (streams.AgentEvent, error) {
	var message models.Message
	if err := json.Unmarshal(payload, &message); err != nil {
		return streams.AgentEvent{}, err
	}
	_, eligible, err := errandPermissionClaimEligibility(&message, true)
	if err != nil || !eligible {
		return streams.AgentEvent{}, err
	}
	optionID, _ := message.Metadata["auto_approved_option_id"].(string)
	optionKind, _ := message.Metadata["auto_approved_option_kind"].(string)
	requestID, _ := message.Metadata["request_id"].(string)
	pendingID, _ := message.Metadata["pending_id"].(string)
	proposal := models.PermissionResolutionAudit{OptionID: optionID, OptionKind: optionKind, Source: models.PermissionSourceAutoApprove}
	if generation == 0 || requestID == "" || pendingID == "" || !matchesErrandAutomaticPermissionProposal(&message, proposal) {
		return streams.AgentEvent{}, nil
	}
	return streams.AgentEvent{
		Type: streams.EventTypePermissionRequest, RequestID: requestID, PendingID: pendingID,
		TurnID: turnID, DeliverySubmissionID: submissionID, PromptGeneration: generation,
		AutoApprovalPending: true, AutoApprovedOptionID: optionID, AutoApprovedOptionKind: optionKind,
		AutoApprovalSource: streams.PermissionDecisionSourceAutoApprove,
	}, nil
}
