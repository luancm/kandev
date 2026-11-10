package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Source records survive inbox payload pruning and contain no terminal authority.
const unownedToolSourceEffect = "agent_delivery.unowned_tool_source.v1"

type unownedToolSource struct {
	Sequence         int64  `json:"sequence"`
	ToolID           string `json:"tool_id"`
	ParentID         string `json:"parent_id"`
	PromptGeneration uint64 `json:"prompt_generation"`
	ProtocolID       string `json:"protocol_id"`
	ControlTurnID    int64  `json:"control_turn_id"`
	TurnID           string `json:"turn_id"`
}

func (r *Repository) retainUnownedToolSourceTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent, turn string, errand bool) error {
	if !errand || event.SubmissionID != "" || event.EventType != streams.EventTypeToolCall {
		return nil
	}
	var agent streams.AgentEvent
	if err := json.Unmarshal(event.Payload, &agent); err != nil {
		return err
	}
	if agent.ToolCallID == "" {
		return nil
	}
	source := unownedToolSource{Sequence: event.Sequence, ToolID: agent.ToolCallID, ParentID: agent.ParentToolCallID, PromptGeneration: agent.PromptGeneration, ProtocolID: agent.ProtocolMessageID, ControlTurnID: agent.ControlTurnID, TurnID: turn}
	raw, err := json.Marshal(source)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("%s:%s:%d", unownedToolSourceEffect, event.StreamID, event.Sequence)
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO agent_delivery_effects(effect_key,stream_id,sequence,effect_type,state,session_id,incarnation_id,harness_generation,submission_id,turn_id,outcome,created_at) VALUES(?,?,0,?,'pending',?,?,?,'',?,?,?) ON CONFLICT(effect_key) DO NOTHING`), key, event.StreamID, unownedToolSourceEffect, event.SessionID, event.IncarnationID, event.HarnessGeneration, turn, string(raw), r.nowUTC())
	return err
}

func unownedToolUpdateTurnTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent, agent streams.AgentEvent) (string, error) {
	rows, err := tx.QueryContext(ctx, tx.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_type=? AND session_id=? AND incarnation_id=? AND harness_generation=? AND stream_id=?`), unownedToolSourceEffect, event.SessionID, event.IncarnationID, event.HarnessGeneration, event.StreamID)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	var candidates, exact []unownedToolSource
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return "", err
		}
		var source unownedToolSource
		if err := json.Unmarshal([]byte(raw), &source); err != nil {
			return "", err
		}
		if !source.matches(event, agent) {
			continue
		}
		candidates = append(candidates, source)
		if agent.ProtocolMessageID != "" && source.ProtocolID == agent.ProtocolMessageID {
			exact = append(exact, source)
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	// Protocol message IDs may change between start and streaming updates.
	// A unique source tuple is sufficient; multiple sources require exact evidence.
	if len(exact) > 0 {
		candidates = exact
	}
	if len(candidates) == 0 {
		return "", repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	turn := candidates[0].TurnID
	for _, candidate := range candidates {
		if turn == "" || candidate.TurnID != turn {
			return "", repoerrors.ErrAgentDeliveryIdentityMismatch
		}
	}
	return turn, nil
}

func (source unownedToolSource) matches(event *models.AgentDeliveryEvent, agent streams.AgentEvent) bool {
	return source.Sequence < event.Sequence && source.ToolID == agent.ToolCallID &&
		source.ParentID == agent.ParentToolCallID && source.PromptGeneration == agent.PromptGeneration &&
		(source.ControlTurnID == 0 || agent.ControlTurnID == 0 || source.ControlTurnID == agent.ControlTurnID)
}

func (r *Repository) unownedErrandEventTurnTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent, payload streams.AgentEvent) (string, error) {
	if payload.TurnID != "" {
		return "", repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	if err := r.validateAdmissionNativeSessionTx(ctx, tx, event, payload.SessionID, ""); err != nil {
		return "", err
	}
	if payload.Type == streams.EventTypeShellOutputUpdate || payload.Type == streams.EventTypeToolUpdate {
		return unownedToolUpdateTurnTx(ctx, tx, event, payload)
	}
	return unownedDeliveryTurnID(event, payload), nil
}
