package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func errandToolMessageID(event *models.AgentDeliveryEvent, turn string, agent streams.AgentEvent) string {
	raw, _ := json.Marshal([]string{event.SessionID, event.StreamID, event.SubmissionID, turn, agent.ToolCallID, agent.ParentToolCallID})
	digest := sha256.Sum256(raw)
	return fmt.Sprintf("errand-tool-%x", digest[:16])
}

func (r *Repository) persistErrandToolEventTx(ctx context.Context, tx *sqlx.Tx, errandID string, event *models.AgentDeliveryEvent) error {
	turn, agent, event, err := r.resolveErrandToolEventTx(ctx, tx, errandID, event)
	if err != nil {
		return err
	}

	id := errandToolMessageID(event, turn, agent)
	raw, exists, err := r.claimErrandToolMessageTx(ctx, tx, errandID, id, turn, event, agent)
	if err != nil {
		return err
	}

	now := errandTranscriptTime(r.nowUTC())
	message, err := r.loadErrandToolMessageTx(ctx, tx, errandID, id, turn, event, agent, raw, now, exists)
	if err != nil {
		return err
	}

	message.Metadata["status"] = agent.ToolStatus
	if agent.ToolTitle != "" {
		message.Content = agent.ToolTitle
		message.Metadata["title"] = agent.ToolTitle
	}
	if agent.NormalizedPayload != nil && !models.ToolPayloadRemoved(message.Metadata) {
		message.Metadata["normalized"] = agent.NormalizedPayload
		message.Type = models.MessageType(agent.NormalizedPayload.Kind().ToMessageType())
	}
	if exists && models.ToolPayloadRemoved(message.Metadata) {
		now = message.UpdatedAt
	}
	message.UpdatedAt = now
	encoded, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if exists {
		_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_messages SET payload=?,unread_candidate=?,updated_at=? WHERE errand_id=? AND id=? AND turn_id=?`), string(encoded), errandMessageUnreadCandidate(&message), now, errandID, id, turn)
	} else {
		_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO errand_messages (id,errand_id,turn_id,payload,unread_candidate,created_at,updated_at) VALUES (?,?,?,?,?,?,?)`), id, errandID, turn, string(encoded), errandMessageUnreadCandidate(&message), now, now)
	}
	return err
}

func (r *Repository) loadErrandToolMessageTx(ctx context.Context, tx *sqlx.Tx, errandID, id, turn string, event *models.AgentDeliveryEvent, agent streams.AgentEvent, raw string, now time.Time, exists bool) (models.Message, error) {
	var err error
	var message models.Message
	if exists {
		decoder := json.NewDecoder(bytes.NewBufferString(raw))
		decoder.UseNumber()
		if err = decoder.Decode(&message); err != nil {
			return message, err
		}
		parent, _ := message.Metadata["parent_tool_call_id"].(string)
		if message.AuthorType != models.MessageAuthorAgent || message.AuthorID != event.SubmissionID || message.Metadata == nil || message.Metadata["tool_call_id"] != agent.ToolCallID || parent != agent.ParentToolCallID {
			return message, repoerrors.ErrAgentDeliveryIdentityMismatch
		}
		return message, nil
	}

	var taskID string
	if err = tx.GetContext(ctx, &taskID, tx.Rebind(`SELECT task_id FROM task_session_errands WHERE id=?`), errandID); err != nil {
		return message, err
	}
	message = models.Message{ID: id, TaskID: taskID, TurnID: turn, AuthorType: models.MessageAuthorAgent, AuthorID: event.SubmissionID, Type: models.MessageTypeToolCall, CreatedAt: now, Metadata: map[string]interface{}{"tool_call_id": agent.ToolCallID}}
	if agent.ParentToolCallID != "" {
		message.Metadata["parent_tool_call_id"] = agent.ParentToolCallID
	}
	return message, nil
}

func (r *Repository) resolveErrandToolEventTx(ctx context.Context, tx *sqlx.Tx, errandID string, event *models.AgentDeliveryEvent) (string, streams.AgentEvent, *models.AgentDeliveryEvent, error) {
	turn, err := r.errandDeliveryEventAdmissionTx(ctx, tx, errandID, event)
	if err != nil {
		return "", streams.AgentEvent{}, nil, err
	}
	var agent streams.AgentEvent
	if err = json.Unmarshal(event.Payload, &agent); err != nil {
		return "", streams.AgentEvent{}, nil, err
	}
	if agent.Type == streams.EventTypeToolUpdate {
		source, sourceErr := r.resolveErrandAdmittedToolSourceTx(ctx, tx, errandID, event)
		if sourceErr != nil {
			return "", streams.AgentEvent{}, nil, sourceErr
		}
		bound := *event
		bound.SubmissionID = source.Admission.SubmissionID
		event = &bound
	}
	return turn, agent, event, nil
}

func (r *Repository) claimErrandToolMessageTx(ctx context.Context, tx *sqlx.Tx, errandID, id, turn string, event *models.AgentDeliveryEvent, agent streams.AgentEvent) (string, bool, error) {
	var err error
	var raw string
	query := `SELECT payload FROM errand_messages WHERE errand_id=? AND id=? AND turn_id=?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	err = tx.QueryRowContext(ctx, tx.Rebind(query), errandID, id, turn).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return "", false, err
	}
	exists := err == nil
	if exists && agent.Type == streams.EventTypeToolCall {
		return "", false, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	if !exists && agent.Type != streams.EventTypeToolCall {
		return "", false, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	owner := &models.AgentDeliveryEffect{EffectKey: "agent_delivery.errand_tool_owner.v1:" + id, EffectType: "agent_delivery.errand_tool_owner.v1", SessionID: event.SessionID, StreamID: event.StreamID, IncarnationID: event.IncarnationID, HarnessGeneration: event.HarnessGeneration, SubmissionID: event.SubmissionID, TurnID: turn}
	inserted, err := insertDeliveryEffectTx(ctx, tx, tx.Rebind, owner)
	if err != nil {
		return "", false, err
	}
	if exists && inserted {
		return "", false, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return raw, exists, nil
}
