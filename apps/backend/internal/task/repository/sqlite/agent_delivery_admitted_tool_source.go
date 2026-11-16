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

const admittedToolSourceEffect = "agent_delivery.admitted_tool_source.v1"

type admittedToolSource struct {
	unownedToolSource
	Admission       promptAdmission `json:"admission"`
	NativeSessionID string          `json:"native_session_id"`
	PayloadHash     string          `json:"payload_hash"`
}

func (r *Repository) retainAdmittedToolSourceTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent, turn string, errand bool) error {
	if !errand {
		return nil
	}
	if event.SubmissionID == "" && (event.EventType == streams.EventTypeToolUpdate || event.EventType == streams.EventTypeShellOutputUpdate) {
		sources, err := r.legacyAdmittedToolSourcesTx(ctx, tx, event)
		if err != nil {
			return err
		}
		for _, source := range sources {
			if err := r.storeAdmittedToolSourceTx(ctx, tx, source); err != nil {
				return err
			}
		}
		return nil
	}
	if event.SubmissionID == "" || event.EventType != streams.EventTypeToolCall {
		return nil
	}
	source, err := admittedToolSourceTx(ctx, tx, event)
	if err != nil || source.ToolID == "" {
		return err
	}
	if source.TurnID != turn {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return r.storeAdmittedToolSourceTx(ctx, tx, source)
}

func (r *Repository) storeAdmittedToolSourceTx(ctx context.Context, tx *sqlx.Tx, source admittedToolSource) error {
	raw, err := json.Marshal(source)
	if err != nil {
		return err
	}
	p := source.Admission
	key := fmt.Sprintf("%s:%s:%d", admittedToolSourceEffect, p.StreamID, source.Sequence)
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO agent_delivery_effects
 (effect_key,stream_id,sequence,effect_type,state,session_id,incarnation_id,harness_generation,submission_id,turn_id,outcome,created_at)
 VALUES(?,?,0,?,'pending',?,?,?,?,?,?,?) ON CONFLICT(effect_key) DO NOTHING`),
		key, p.StreamID, admittedToolSourceEffect, p.SessionID, p.IncarnationID,
		p.HarnessGeneration, p.SubmissionID, p.TurnID, string(raw), r.nowUTC())
	if err != nil {
		return err
	}
	var stored string
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_key=?`), key).Scan(&stored); err != nil {
		return err
	}
	if stored != string(raw) {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return nil
}

func admittedToolSourceTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent) (admittedToolSource, error) {
	var source admittedToolSource
	var agent streams.AgentEvent
	if err := json.Unmarshal(event.Payload, &agent); err != nil {
		return source, err
	}
	if agent.ToolCallID == "" {
		return source, nil
	}
	var raw string
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_key=? AND effect_type=?`),
		promptAdmissionEffect+":"+event.SubmissionID, promptAdmissionEffect).Scan(&raw); err != nil {
		return source, err
	}
	if err := json.Unmarshal([]byte(raw), &source.Admission); err != nil {
		return source, err
	}
	source.unownedToolSource = unownedToolSource{Sequence: event.Sequence, ToolID: agent.ToolCallID,
		ParentID: agent.ParentToolCallID, PromptGeneration: source.Admission.PromptGeneration,
		ProtocolID: agent.ProtocolMessageID, ControlTurnID: agent.ControlTurnID, TurnID: source.Admission.TurnID}
	source.NativeSessionID, source.PayloadHash = agent.SessionID, agentDeliveryEventHash(event)
	return source, nil
}

func (r *Repository) admittedToolUpdateSourcesTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent, agent streams.AgentEvent) ([]unownedToolSource, error) {
	var raws []string
	err := tx.SelectContext(ctx, &raws, tx.Rebind(`SELECT outcome FROM agent_delivery_effects
 WHERE effect_type=? AND session_id=? AND incarnation_id=? AND harness_generation=? AND stream_id=?`),
		admittedToolSourceEffect, event.SessionID, event.IncarnationID, event.HarnessGeneration, event.StreamID)
	if err != nil {
		return nil, err
	}
	sources, err := r.legacyAdmittedToolSourcesTx(ctx, tx, event)
	if err != nil {
		return nil, err
	}
	for _, raw := range raws {
		var source admittedToolSource
		if err := json.Unmarshal([]byte(raw), &source); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	var candidates []unownedToolSource
	for _, source := range sources {
		probe := agent
		if probe.PromptGeneration == 0 {
			probe.PromptGeneration = source.PromptGeneration
		}
		if !source.matches(event, probe) {
			continue
		}
		if err := r.validateAdmittedToolSourceTx(ctx, tx, event, agent, source); err != nil {
			return nil, err
		}
		candidates = append(candidates, source.unownedToolSource)
	}
	return candidates, nil
}

func (r *Repository) validateAdmittedToolSourceTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent, agent streams.AgentEvent, source admittedToolSource) error {
	var raw string
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_key=? AND effect_type=?`),
		promptAdmissionEffect+":"+source.Admission.SubmissionID, promptAdmissionEffect).Scan(&raw); err != nil {
		return err
	}
	var admitted promptAdmission
	if err := json.Unmarshal([]byte(raw), &admitted); err != nil {
		return err
	}
	if admitted != source.Admission || source.TurnID != admitted.TurnID || source.PromptGeneration != admitted.PromptGeneration ||
		source.PayloadHash == "" || source.NativeSessionID != "" && agent.SessionID != "" && source.NativeSessionID != agent.SessionID {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	// Validate a copy against the retained admission; received bytes and the
	// update's missing submission remain immutable.
	bound := *event
	bound.SubmissionID = admitted.SubmissionID
	var err error
	bound.Payload, err = json.Marshal(agent)
	if err != nil {
		return err
	}
	turn, _, err := r.agentDeliveryEventAdmissionTx(ctx, tx, &bound)
	if err != nil {
		return err
	}
	if turn != source.TurnID {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return nil
}
