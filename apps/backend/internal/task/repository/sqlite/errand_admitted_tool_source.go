package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (r *Repository) retainErrandAdmittedToolSourceTx(ctx context.Context, tx *sqlx.Tx, errandID string, event *models.AgentDeliveryEvent, turn string) error {
	var raw, taskID string
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT effect.outcome,e.task_id FROM agent_delivery_effects effect JOIN task_session_errands e ON e.id=? WHERE effect.effect_key=? AND effect.effect_type=?`), errandID, errandPromptAdmissionEffect+":"+event.SubmissionID, errandPromptAdmissionEffect).Scan(&raw, &taskID); err != nil {
		return err
	}
	var proof errandPromptProof
	if err := json.Unmarshal([]byte(raw), &proof); err != nil {
		return err
	}
	var agent streams.AgentEvent
	if err := json.Unmarshal(event.Payload, &agent); err != nil {
		return err
	}
	if agent.ToolCallID == "" || proof.TurnID != turn {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	source := admittedToolSource{toolSource: toolSource{Sequence: event.Sequence, ToolID: agent.ToolCallID, ParentID: agent.ParentToolCallID, PromptGeneration: proof.PromptGeneration, ProtocolID: agent.ProtocolMessageID, ControlTurnID: agent.ControlTurnID, TurnID: turn}, Admission: promptAdmission{ErrandGeneration: proof.Generation, NativeSessionID: proof.NativeSessionID, TaskID: taskID, SessionID: event.SessionID, ExecutionID: proof.ExecutionID, IncarnationID: proof.IncarnationID, HarnessGeneration: uint64(proof.HarnessGeneration), PromptGeneration: proof.PromptGeneration, StreamID: proof.StreamID, SubmissionID: proof.SubmissionID, TurnID: proof.TurnID}, NativeSessionID: agent.SessionID, PayloadHash: agentDeliveryEventHash(event)}
	return r.storeAdmittedToolSourceTx(ctx, tx, source)
}

func (r *Repository) resolveErrandAdmittedToolSourceTx(ctx context.Context, tx *sqlx.Tx, errandID string, event *models.AgentDeliveryEvent) (admittedToolSource, error) {
	var agent streams.AgentEvent
	if err := json.Unmarshal(event.Payload, &agent); err != nil {
		return admittedToolSource{}, err
	}
	var raws []string
	if err := tx.SelectContext(ctx, &raws, tx.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_type=? AND session_id=? AND incarnation_id=? AND harness_generation=? AND stream_id=?`), admittedToolSourceEffect, event.SessionID, event.IncarnationID, event.HarnessGeneration, event.StreamID); err != nil {
		return admittedToolSource{}, err
	}
	var candidates []admittedToolSource
	for _, raw := range raws {
		var source admittedToolSource
		if err := json.Unmarshal([]byte(raw), &source); err != nil {
			return admittedToolSource{}, err
		}
		probe := agent
		if probe.PromptGeneration == 0 {
			probe.PromptGeneration = source.PromptGeneration
		}
		if event.SubmissionID != "" && event.SubmissionID != source.Admission.SubmissionID {
			continue
		}
		if !source.matches(event, probe) {
			continue
		}
		if err := r.validateErrandAdmittedToolSourceTx(ctx, tx, errandID, event, agent, source); err != nil {
			return admittedToolSource{}, err
		}
		candidates = append(candidates, source)
	}
	var exact []admittedToolSource
	if agent.ProtocolMessageID != "" {
		for _, source := range candidates {
			if source.ProtocolID == agent.ProtocolMessageID {
				exact = append(exact, source)
			}
		}
	}
	if len(exact) > 0 {
		candidates = exact
	}
	if len(candidates) != 1 {
		return admittedToolSource{}, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return candidates[0], nil
}

func (r *Repository) validateErrandAdmittedToolSourceTx(ctx context.Context, tx *sqlx.Tx, errandID string, event *models.AgentDeliveryEvent, agent streams.AgentEvent, source admittedToolSource) error {
	if source.Admission.ErrandGeneration == 0 || source.PayloadHash == "" || source.TurnID != source.Admission.TurnID || source.PromptGeneration != source.Admission.PromptGeneration || (source.NativeSessionID != "" && agent.SessionID != "" && source.NativeSessionID != agent.SessionID) {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	var raw string
	if err := tx.GetContext(ctx, &raw, tx.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_key=? AND effect_type=?`), errandPromptAdmissionEffect+":"+source.Admission.SubmissionID, errandPromptAdmissionEffect); err != nil {
		if err == sql.ErrNoRows {
			return repoerrors.ErrAgentDeliveryIdentityMismatch
		}
		return err
	}
	var proof errandPromptProof
	if err := json.Unmarshal([]byte(raw), &proof); err != nil {
		return err
	}
	if !errandAdmittedToolProofMatches(errandID, proof, source) {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}

	bound := *event
	bound.SubmissionID = source.Admission.SubmissionID
	turn, err := r.errandDeliveryEventAdmissionTx(ctx, tx, errandID, &bound)
	if err != nil {
		return err
	}
	if turn != source.TurnID {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return nil
}

func validateErrandNativeIdentityTx(ctx context.Context, tx *sqlx.Tx, proof errandPromptProof, nativeID string) error {
	if nativeID == "" {
		return nil
	}
	var native string
	err := tx.GetContext(ctx, &native, tx.Rebind(`SELECT native_session_id FROM errand_harness_bindings WHERE errand_id=? AND owner_generation=? AND execution_id=? AND incarnation_id=? AND harness_generation=? AND stream_id=?`), proof.ErrandID, int64(proof.Generation), proof.ExecutionID, proof.IncarnationID, proof.HarnessGeneration, proof.StreamID)
	if err != nil {
		return err
	}
	if (proof.NativeSessionID != "" && proof.NativeSessionID != nativeID) || (native != "" && native != nativeID) {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return nil
}

func (r *Repository) ResolveErrandToolOrigin(ctx context.Context, event *models.AgentDeliveryEvent) (*models.ErrandToolOrigin, error) {
	if event == nil || (event.EventType != streams.EventTypeToolUpdate && event.EventType != streams.EventTypeToolCall && event.EventType != streams.EventTypeShellOutputUpdate) {
		return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	errandID, ok := errandDeliveryIdentity(event.SessionID)
	if !ok || errandID == "" {
		return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	tx, err := r.ro.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if event.EventType == streams.EventTypeToolCall {
		turn, err := r.errandDeliveryEventAdmissionTx(ctx, tx, errandID, event)
		if err != nil {
			return nil, err
		}
		var raw string
		if err = tx.GetContext(ctx, &raw, tx.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_key=? AND effect_type=?`), errandPromptAdmissionEffect+":"+event.SubmissionID, errandPromptAdmissionEffect); err != nil {
			return nil, err
		}
		var proof errandPromptProof
		if err = json.Unmarshal([]byte(raw), &proof); err != nil {
			return nil, err
		}
		return &models.ErrandToolOrigin{SubmissionID: proof.SubmissionID, PromptGeneration: proof.PromptGeneration, TurnID: turn}, nil
	}
	source, err := r.resolveErrandAdmittedToolSourceTx(ctx, tx, errandID, event)
	if err != nil {
		return nil, err
	}
	return &models.ErrandToolOrigin{SubmissionID: source.Admission.SubmissionID, PromptGeneration: source.PromptGeneration, TurnID: source.TurnID}, nil
}

func errandAdmittedToolProofMatches(errandID string, proof errandPromptProof, source admittedToolSource) bool {
	return proof.ErrandID == errandID && proof.Generation == source.Admission.ErrandGeneration && proof.ExecutionID == source.Admission.ExecutionID && proof.IncarnationID == source.Admission.IncarnationID && uint64(proof.HarnessGeneration) == source.Admission.HarnessGeneration && proof.PromptGeneration == source.PromptGeneration && proof.StreamID == source.Admission.StreamID && proof.SubmissionID == source.Admission.SubmissionID && proof.TurnID == source.TurnID
}
