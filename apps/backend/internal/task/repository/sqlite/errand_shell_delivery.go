package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

// GetErrandMessageDetail loads shell output only after resolving its Errand owner.
func (r *Repository) GetErrandMessageDetail(ctx context.Context, errandID, messageID string) (*models.Message, error) {
	tx, err := r.ro.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	message, err := scanErrandMessage(tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+errandMessageColumns+` FROM errand_messages m JOIN task_session_errands e ON e.id=m.errand_id WHERE m.errand_id=? AND m.id=?`), errandID, messageID))
	if err != nil {
		return nil, err
	}
	if message.Metadata[shellOutputStorageKey] == nil {
		if err = rehydrateMessagePayload(ctx, tx, message); err != nil {
			return nil, err
		}
		return message, tx.Commit()
	}
	if models.ToolPayloadRemoved(message.Metadata) {
		return nil, fmt.Errorf("tool_payload_removed")
	}
	head, err := loadShellOutputHead(ctx, tx, errandShellOutputQueries, messageID)
	if err != nil {
		return nil, err
	}
	if head == nil {
		return nil, streams.ErrShellOutputBaseline
	}
	state, err := shellOutputStateTx(ctx, tx, errandShellOutputQueries, messageID, head)
	if err != nil {
		return nil, err
	}
	var ok bool
	message.Metadata, ok = models.RehydrateShellOutput(message.Metadata, models.ShellExecOutputSnapshot{Stdout: state.Output.Stdout, Stderr: state.Output.Stderr, ExitCode: state.Output.ExitCode, Truncated: state.Output.Truncated})
	if !ok {
		return nil, streams.ErrShellOutputBaseline
	}
	return message, tx.Commit()
}

func (r *Repository) persistErrandShellOutputTx(ctx context.Context, tx *sqlx.Tx, errandID string, event *models.AgentDeliveryEvent) error {
	var agent streams.AgentEvent
	if err := json.Unmarshal(event.Payload, &agent); err != nil {
		return err
	}
	if agent.Type != streams.EventTypeShellOutputUpdate || agent.ShellOutput == nil || agent.ToolCallID == "" || agent.Role == authorKindUser {
		return streams.ErrShellOutputBaseline
	}
	source, err := r.resolveErrandAdmittedToolSourceTx(ctx, tx, errandID, event)
	if err != nil {
		return err
	}
	bound := *event
	bound.SubmissionID = source.Admission.SubmissionID
	id := errandToolMessageID(&bound, source.TurnID, agent)
	raw, exists, err := r.claimErrandToolMessageTx(ctx, tx, errandID, id, source.TurnID, &bound, agent)
	if err != nil {
		return err
	}
	message, err := r.loadErrandToolMessageTx(ctx, tx, errandID, id, source.TurnID, &bound, agent, raw, r.nowUTC(), exists)
	if err != nil {
		return err
	}
	if message.Type != models.MessageTypeToolExecute {
		return streams.ErrShellOutputBaseline
	}
	if models.ToolPayloadRemoved(message.Metadata) {
		return nil
	}
	return r.persistErrandShellChunksTx(ctx, tx, errandID, event, agent, source, &message)
}

func (r *Repository) persistErrandShellChunksTx(ctx context.Context, tx *sqlx.Tx, errandID string, event *models.AgentDeliveryEvent, agent streams.AgentEvent, source admittedToolSource, message *models.Message) error {
	id := message.ID
	head, err := loadShellOutputHead(ctx, tx, errandShellOutputQueries, id)
	if err != nil {
		return err
	}
	identity, err := json.Marshal([]any{event.StreamID, source.Admission.SubmissionID, agent.ToolCallID, agent.ParentToolCallID, source.PromptGeneration})
	if err != nil {
		return err
	}
	if (head != nil && head.Identity != string(identity)) || (head == nil && agent.ShellOutput.BaseRevision != 0) {
		return streams.ErrShellOutputBaseline
	}
	base, err := shellOutputStateTx(ctx, tx, errandShellOutputQueries, id, head)
	if err != nil {
		return err
	}
	output, err := persistShellChunksTx(ctx, tx, errandShellOutputQueries, id, event, agent, head, base, string(identity))
	if err != nil {
		return err
	}
	message.Metadata, err = projectShellOutputSummary(message.Metadata, agent, output)
	if err != nil {
		return err
	}
	if agent.ToolTitle != "" {
		message.Content = agent.ToolTitle
	}
	message.UpdatedAt = errandTranscriptTime(r.nowUTC())
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_messages SET payload=?,updated_at=? WHERE errand_id=? AND id=? AND turn_id=?`), string(payload), message.UpdatedAt, errandID, id, source.TurnID)
	return err
}
