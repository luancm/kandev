package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func errandPermissionMessageID(errandID, streamID, submissionID, turnID, requestID string) string {
	raw, _ := json.Marshal([]string{errandID, streamID, submissionID, turnID, requestID})
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("errand-permission-%x", sum[:16])
}

func (r *Repository) persistErrandPermissionEventTx(ctx context.Context, tx *sqlx.Tx, errandID string, event *models.AgentDeliveryEvent) error {
	turn, err := r.errandDeliveryEventAdmissionTx(ctx, tx, errandID, event)
	if err != nil {
		return err
	}
	var agent streams.AgentEvent
	if err = json.Unmarshal(event.Payload, &agent); err != nil {
		return err
	}
	if agent.Type != streams.EventTypePermissionRequest || event.Terminal || agent.RequestID == "" || agent.PendingID == "" {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	var taskID string
	if err = tx.GetContext(ctx, &taskID, tx.Rebind(`SELECT task_id FROM task_session_errands WHERE id=?`), errandID); err != nil {
		return err
	}
	id := errandPermissionMessageID(errandID, event.StreamID, event.SubmissionID, turn, agent.RequestID)
	inserted, err := insertDeliveryEffectTx(ctx, tx, tx.Rebind, &models.AgentDeliveryEffect{EffectKey: "agent_delivery.errand_permission_owner.v1:" + id, EffectType: "agent_delivery.errand_permission_owner.v1", SessionID: event.SessionID, StreamID: event.StreamID, IncarnationID: event.IncarnationID, HarnessGeneration: event.HarnessGeneration, SubmissionID: event.SubmissionID, TurnID: turn, Outcome: agentDeliveryEventHash(event)})
	if err != nil {
		return err
	}
	if !inserted {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	metadata := map[string]interface{}{"request_id": agent.RequestID, "pending_id": agent.PendingID, "tool_call_id": agent.ToolCallID, "options": agent.PermissionOptions, "action_type": agent.ActionType, "action_details": agent.ActionDetails, "permission_human_input": agent.PermissionHumanInput, models.PermissionPresentationMetadataKey: string(models.PermissionPresentationInteractive)}
	if agent.AutoApprovedOptionID != "" {
		metadata["auto_approved_option_id"] = agent.AutoApprovedOptionID
		metadata["auto_approval_pending"] = agent.AutoApprovalPending
		metadata["auto_approved_option_kind"] = agent.AutoApprovedOptionKind
		metadata["auto_approval_source"] = agent.AutoApprovalSource
	}
	requestsInput := true
	if !agent.PermissionHumanInput && agent.AutoApprovedOptionID != "" {
		if agent.AutoApprovalPending {
			metadata[models.PermissionPresentationMetadataKey] = string(models.PermissionPresentationAutomatic)
		} else {
			metadata["permission_decision"] = models.PermissionDecision{OptionID: agent.AutoApprovedOptionID, OptionKind: agent.AutoApprovedOptionKind, Source: agent.AutoApprovalSource}
			metadata["status"] = string(models.PermissionStatusApproved)
		}
		requestsInput = false
	}
	now := errandTranscriptTime(r.nowUTC())
	message := models.Message{ID: id, TaskID: taskID, TurnID: turn, AuthorType: models.MessageAuthorAgent, AuthorID: event.SubmissionID, Type: models.MessageTypePermissionRequest, Content: agent.PermissionTitle, Metadata: metadata, RequestsInput: requestsInput, CreatedAt: now, UpdatedAt: now}
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO errand_messages (id,errand_id,turn_id,payload,created_at,updated_at) VALUES (?,?,?,?,?,?)`), id, errandID, turn, string(payload), now, now)
	return err
}
