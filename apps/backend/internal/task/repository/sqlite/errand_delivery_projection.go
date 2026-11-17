package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func errandDeliveryIdentity(key string) (string, bool) {
	if !strings.HasPrefix(key, "errand:") {
		return "", false
	}
	return strings.TrimPrefix(key, "errand:"), true
}

type errandPromptProof struct {
	ErrandID          string `json:"errand_id"`
	Generation        uint64 `json:"generation"`
	ExecutionID       string `json:"execution_id"`
	IncarnationID     string `json:"incarnation_id"`
	HarnessGeneration int64  `json:"harness_generation"`
	StreamID          string `json:"stream_id"`
	NativeSessionID   string `json:"native_session_id"`
	PromptGeneration  uint64 `json:"prompt_generation"`
	TurnID            string `json:"turn_id"`
	SubmissionID      string `json:"submission_id"`
	PayloadHash       string `json:"payload_hash"`
	DispatchAttemptID string `json:"dispatch_attempt_id"`
}

func (r *Repository) errandDeliveryEventAdmissionTx(ctx context.Context, tx *sqlx.Tx, errandID string, event *models.AgentDeliveryEvent) (string, error) {
	if errandID == "" {
		return "", repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	if event.SubmissionID == "" {
		if event.EventType == streams.EventTypeToolUpdate {
			source, err := r.resolveErrandAdmittedToolSourceTx(ctx, tx, errandID, event)
			return source.TurnID, err
		}
		return "", r.validateErrandControlEventTx(ctx, tx, errandID, event)
	}
	var raw, taskID string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT effect.outcome,errand.task_id FROM agent_delivery_effects effect JOIN task_session_errands errand ON errand.id=? WHERE effect.effect_key=? AND effect.effect_type=?`), errandID, errandPromptAdmissionEffect+":"+event.SubmissionID, errandPromptAdmissionEffect).Scan(&raw, &taskID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", repoerrors.ErrAgentDeliveryIdentityMismatch
		}
		return "", err
	}
	var proof errandPromptProof
	if err = json.Unmarshal([]byte(raw), &proof); err != nil {
		return "", err
	}
	var agent streams.AgentEvent
	if err = json.Unmarshal(event.Payload, &agent); err != nil {
		return "", err
	}
	if err = validateErrandSubmittedEvent(errandID, taskID, event, agent, proof); err != nil {
		return "", err
	}

	var valid bool
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM errand_harness_bindings b JOIN errand_turns t ON t.errand_id=b.errand_id JOIN agent_delivery_submissions s ON s.id=? AND s.session_id=? AND s.incarnation_id=b.incarnation_id AND s.harness_generation=b.harness_generation WHERE b.errand_id=? AND b.owner_generation=? AND b.execution_id=? AND b.incarnation_id=? AND b.harness_generation=? AND b.stream_id=? AND t.id=? AND s.payload_hash=?)`), proof.SubmissionID, event.SessionID, errandID, int64(proof.Generation), proof.ExecutionID, proof.IncarnationID, proof.HarnessGeneration, proof.StreamID, proof.TurnID, proof.PayloadHash).Scan(&valid)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	if err = validateErrandNativeIdentityTx(ctx, tx, proof, agent.SessionID); err != nil {
		return "", err
	}
	return proof.TurnID, nil
}

func (r *Repository) persistErrandCanonicalMessageTx(ctx context.Context, tx *sqlx.Tx, errandID string, event *models.AgentDeliveryEvent, agent streams.AgentEvent) (bool, error) {
	content := agent.Text
	typ := models.MessageTypeMessage
	if agent.Type == streams.EventTypeReasoning {
		content = agent.ReasoningText
		typ = models.MessageTypeThinking
	}
	if content == "" {
		return false, nil
	}
	turn, err := r.errandDeliveryEventAdmissionTx(ctx, tx, errandID, event)
	if err != nil {
		return false, err
	}
	var taskID string
	if err = tx.GetContext(ctx, &taskID, tx.Rebind(`SELECT task_id FROM task_session_errands WHERE id=?`), errandID); err != nil {
		return false, err
	}
	id := agent.CanonicalMessageID
	if id == "" {
		id = fmt.Sprintf("agent-delivery-%s-%d", event.StreamID, event.Sequence)
	}
	var raw string
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT payload FROM errand_messages WHERE errand_id=? AND id=? AND turn_id=?`), errandID, id, turn).Scan(&raw)
	now := errandTranscriptTime(r.nowUTC())
	var message models.Message
	appended := err == nil
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	message, err = r.errandCanonicalMessageOwnerTx(ctx, tx, errandID, id, turn, taskID, typ, event, raw, now, appended)
	if err != nil {
		return false, err
	}

	if typ == models.MessageTypeThinking {
		old, _ := message.Metadata["thinking"].(string)
		message.Metadata["thinking"] = old + content
	} else {
		message.Content += content
	}
	message.UpdatedAt = now
	encoded, err := json.Marshal(message)
	if err != nil {
		return false, err
	}
	if appended {
		_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_messages SET payload=?,updated_at=? WHERE errand_id=? AND id=? AND turn_id=?`), string(encoded), now, errandID, id, turn)
	} else {
		_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO errand_messages (id,errand_id,turn_id,payload,created_at,updated_at) VALUES (?,?,?,?,?,?)`), id, errandID, turn, string(encoded), now, now)
	}
	return appended, err
}

func (r *Repository) validateErrandControlEventTx(ctx context.Context, tx *sqlx.Tx, errandID string, event *models.AgentDeliveryEvent) error {
	var agent streams.AgentEvent
	if err := json.Unmarshal(event.Payload, &agent); err != nil {
		return err
	}
	if event.Terminal || agent.Type != event.EventType || agent.TurnID != "" || agent.PromptGeneration != 0 || agent.DeliverySubmissionID != "" {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	switch agent.Type {
	case streams.EventTypeSessionStatus, streams.EventTypeSessionMode, streams.EventTypeSessionModels, streams.EventTypeSessionModelFallback, streams.EventTypeSessionModelSelectionWarning, streams.EventTypeSessionInfo, streams.EventTypeAgentCapabilities, streams.EventTypeMCPAttachment:
	default:
		return fmt.Errorf("unsupported unsubmitted errand delivery event %q", agent.Type)
	}
	var valid bool
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM errand_harness_bindings b JOIN task_session_errands e ON e.id=b.errand_id WHERE b.errand_id=? AND b.stream_id=? AND b.incarnation_id=? AND b.harness_generation=? AND (?='' OR b.execution_id=?) AND (?='' OR e.task_id=?))`), errandID, event.StreamID, event.IncarnationID, event.HarnessGeneration, event.ExecutionID, event.ExecutionID, event.TaskID, event.TaskID).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return nil
}

func validateErrandSubmittedEvent(errandID, taskID string, event *models.AgentDeliveryEvent, agent streams.AgentEvent, proof errandPromptProof) error {
	if agent.Type != event.EventType {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	switch agent.Type {
	case streams.EventTypeMessageChunk, streams.EventTypeReasoning, streams.EventTypeComplete, streams.EventTypeError, streams.EventTypeToolCall, streams.EventTypeToolUpdate:
	default:
		return fmt.Errorf("unsupported errand delivery event type %q", agent.Type)
	}
	if agent.ShellOutput != nil {
		return fmt.Errorf("unsupported errand shell output")
	}
	if (agent.Type == streams.EventTypeToolCall || agent.Type == streams.EventTypeToolUpdate) && (agent.ToolCallID == "" || (agent.NormalizedPayload != nil && agent.NormalizedPayload.ShellExec() != nil)) {
		return fmt.Errorf("unsupported errand tool event")
	}

	if !errandProofMatches(errandID, taskID, event, agent, proof) {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return nil
}

func (r *Repository) errandCanonicalMessageOwnerTx(ctx context.Context, tx *sqlx.Tx, errandID, id, turn, taskID string, typ models.MessageType, event *models.AgentDeliveryEvent, raw string, now time.Time, appended bool) (models.Message, error) {
	var message models.Message
	var err error
	if !appended {
		_, err = insertDeliveryEffectTx(ctx, tx, tx.Rebind, &models.AgentDeliveryEffect{EffectKey: "agent_delivery.errand_message_owner.v1:" + id, EffectType: "agent_delivery.errand_message_owner.v1", SessionID: event.SessionID, StreamID: event.StreamID, IncarnationID: event.IncarnationID, HarnessGeneration: event.HarnessGeneration, SubmissionID: event.SubmissionID, TurnID: turn})
		if err != nil {
			return message, err
		}
		message = models.Message{ID: id, TaskID: taskID, TurnID: turn, AuthorType: models.MessageAuthorAgent, AuthorID: event.SubmissionID, Type: typ, Metadata: map[string]interface{}{"durable_delivery": true, "delivery_stream_id": event.StreamID, "delivery_sequence": event.Sequence}, CreatedAt: now}
		return message, nil
	}

	if err = json.Unmarshal([]byte(raw), &message); err != nil {
		return message, err
	}
	if message.AuthorType != models.MessageAuthorAgent || message.AuthorID != event.SubmissionID || message.Type != typ {
		return message, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	var valid bool
	if err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM agent_delivery_effects WHERE effect_key=? AND effect_type=? AND session_id=? AND stream_id=? AND incarnation_id=? AND harness_generation=? AND submission_id=? AND turn_id=?)`), "agent_delivery.errand_message_owner.v1:"+id, "agent_delivery.errand_message_owner.v1", event.SessionID, event.StreamID, event.IncarnationID, event.HarnessGeneration, event.SubmissionID, turn).Scan(&valid); err != nil {
		return message, err
	}
	if !valid {
		return message, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	if message.Metadata == nil {
		return message, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return message, nil
}

func errandProofMatches(errandID, taskID string, event *models.AgentDeliveryEvent, agent streams.AgentEvent, proof errandPromptProof) bool {
	if proof.ErrandID != errandID || proof.ExecutionID == "" || proof.IncarnationID != event.IncarnationID || proof.HarnessGeneration != event.HarnessGeneration || proof.StreamID != event.StreamID || proof.SubmissionID != event.SubmissionID || proof.TurnID == "" {
		return false
	}
	return errandProofAttributionMatches(taskID, event, agent, proof)
}

func errandProofAttributionMatches(taskID string, event *models.AgentDeliveryEvent, agent streams.AgentEvent, proof errandPromptProof) bool {
	return (event.TaskID == "" || event.TaskID == taskID) && (event.ExecutionID == "" || event.ExecutionID == proof.ExecutionID) && (agent.TurnID == "" || agent.TurnID == proof.TurnID) && (agent.PromptGeneration == 0 || agent.PromptGeneration == proof.PromptGeneration)
}
