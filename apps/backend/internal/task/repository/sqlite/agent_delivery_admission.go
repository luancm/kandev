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

// Admission records survive running-row replacement and journal compaction.
// Session deletion owns their lifetime. They confer no terminal authority.
const promptAdmissionEffect = "agent_delivery.prompt_admission.v1"

type promptAdmission struct {
	ErrandGeneration  uint64 `json:"errand_generation,omitempty"`
	NativeSessionID   string `json:"native_session_id,omitempty"`
	TaskID            string `json:"task_id"`
	SessionID         string `json:"session_id"`
	ExecutionID       string `json:"execution_id"`
	IncarnationID     string `json:"incarnation_id"`
	HarnessGeneration uint64 `json:"harness_generation"`
	PromptGeneration  uint64 `json:"prompt_generation"`
	StreamID          string `json:"stream_id"`
	SubmissionID      string `json:"submission_id"`
	TurnID            string `json:"turn_id"`
}

func (r *Repository) retainPromptAdmissionTx(ctx context.Context, tx *sqlx.Tx, running *models.ExecutorRunning) error {
	value := running.Metadata["delivery_prompt_turn_binding"]
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var p promptAdmission
	if err = json.Unmarshal(raw, &p); err != nil {
		return err
	}
	// Legacy bindings and predecessor metadata carried into a replacement are
	// not new admissions.
	if p.PromptGeneration == 0 || p.ExecutionID != running.AgentExecutionID {
		return nil
	}
	if p.invalidRunningBinding(running) {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	var valid bool
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM task_sessions s JOIN task_session_turns t ON t.task_session_id=s.id WHERE s.id=? AND s.task_id=? AND s.queue_incarnation_id=? AND t.id=? AND t.task_id=s.task_id)`), p.SessionID, p.TaskID, p.IncarnationID, p.TurnID).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	raw, err = json.Marshal(p)
	if err != nil {
		return err
	}
	key := promptAdmissionEffect + ":" + p.SubmissionID
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO agent_delivery_effects (effect_key,stream_id,sequence,effect_type,state,session_id,incarnation_id,harness_generation,submission_id,turn_id,outcome,created_at) VALUES (?,?,0,?,'pending',?,?,?,?,?,?,?) ON CONFLICT(effect_key) DO NOTHING`), key, p.StreamID, promptAdmissionEffect, p.SessionID, p.IncarnationID, p.HarnessGeneration, p.SubmissionID, p.TurnID, string(raw), r.nowUTC())
	if err != nil {
		return err
	}
	var stored string
	if err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_key=?`), key).Scan(&stored); err != nil {
		return err
	}
	if stored != string(raw) {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return nil
}

// agentDeliveryEventAdmissionTx resolves the exact admitted turn for either owner.
func (r *Repository) agentDeliveryEventAdmissionTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent) (string, error) {
	if errandID, ok := errandDeliveryIdentity(event.SessionID); ok {
		return r.errandDeliveryEventAdmissionTx(ctx, tx, errandID, event)
	}
	return r.taskDeliveryEventAdmissionTx(ctx, tx, event)
}

func (r *Repository) taskDeliveryEventAdmissionTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent) (string, error) {
	var taskID string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT task_id FROM task_sessions WHERE id=?`), event.SessionID).Scan(&taskID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var payload streams.AgentEvent
	if err = json.Unmarshal(event.Payload, &payload); err != nil {
		return "", r.validateOpaqueOrdinaryPayloadTx(ctx, tx, event, err)
	}
	conflict := repoerrors.ErrAgentDeliveryIdentityMismatch
	if conflictingAdmissionEnvelope(taskID, event, payload) {
		return "", conflict
	}
	if event.SubmissionID == "" {
		return "", nil
	}
	var raw string
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_key=? AND effect_type=?`), promptAdmissionEffect+":"+event.SubmissionID, promptAdmissionEffect).Scan(&raw)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var p promptAdmission
	if err = json.Unmarshal([]byte(raw), &p); err != nil {
		return "", err
	}
	if !p.matchesEvent(taskID, event, payload) {
		return "", conflict
	}
	var valid bool
	err = tx.QueryRowContext(ctx, tx.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM task_sessions ts
			JOIN task_session_turns t ON t.task_session_id = ts.id AND t.task_id = ts.task_id
			JOIN agent_delivery_submissions s ON s.id = ? AND s.session_id = ts.id
				AND s.incarnation_id = ts.queue_incarnation_id AND s.harness_generation = ?
			WHERE ts.id = ? AND ts.task_id = ? AND ts.queue_incarnation_id = ? AND t.id = ?
		)`), p.SubmissionID, p.HarnessGeneration, p.SessionID, p.TaskID, p.IncarnationID, p.TurnID).Scan(&valid)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", conflict
	}
	return p.TurnID, r.validateAdmissionNativeSessionTx(ctx, tx, event, payload.SessionID, p.NativeSessionID)
}

// ResolveAgentDeliveryEventTurn resolves callback attribution without changing
// the received payload. Inbox admission repeats the check under writer locks.
func (r *Repository) ResolveAgentDeliveryEventTurn(ctx context.Context, event *models.AgentDeliveryEvent) (string, error) {
	tx, err := r.ro.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	return r.agentDeliveryEventAdmissionTx(ctx, tx, event)
}

func (p promptAdmission) matchesEvent(taskID string, event *models.AgentDeliveryEvent, payload streams.AgentEvent) bool {
	// The receipt verifies the current execution separately. A replacement
	// reader can replay a frame admitted by the original execution.
	return p.TaskID == taskID && p.SessionID == event.SessionID && p.IncarnationID == event.IncarnationID &&
		p.HarnessGeneration == uint64(event.HarnessGeneration) && p.StreamID == event.StreamID &&
		p.SubmissionID == event.SubmissionID && p.PromptGeneration != 0 &&
		(payload.PromptGeneration == 0 || payload.PromptGeneration == p.PromptGeneration) &&
		(payload.TurnID == "" || payload.TurnID == p.TurnID)
}

func (p promptAdmission) invalidRunningBinding(running *models.ExecutorRunning) bool {
	return p.TaskID != running.TaskID || p.SessionID != running.SessionID || p.ExecutionID != running.AgentExecutionID || p.IncarnationID == "" || p.HarnessGeneration == 0 || p.StreamID == "" || p.SubmissionID == "" || p.TurnID == ""
}

func conflictingAdmissionEnvelope(taskID string, event *models.AgentDeliveryEvent, payload streams.AgentEvent) bool {
	return (event.TaskID != "" && event.TaskID != taskID) || (payload.DeliverySubmissionID != "" && payload.DeliverySubmissionID != event.SubmissionID)
}

func (r *Repository) resolveCanonicalAgentDeliveryEventTx(ctx context.Context, tx *sqlx.Tx, stored *models.AgentDeliveryEvent, event streams.AgentEvent) (streams.AgentEvent, error) {
	turn, err := r.agentDeliveryEventAdmissionTx(ctx, tx, stored)
	if err == nil && turn != "" && event.TurnID == "" {
		event.TurnID = turn
	}
	return event, err
}

// ACP payload session IDs name native conversations, while the envelope names
// the Kandev session. Both must belong to the exact retained source.
func (r *Repository) validateAdmissionNativeSessionTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent, nativeID, admittedNativeID string) error {
	if nativeID == "" || nativeID == event.SessionID {
		return nil
	}
	if admittedNativeID != "" {
		if nativeID == admittedNativeID {
			return nil
		}
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	var valid bool
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM harness_session_generations WHERE session_id=? AND incarnation_id=? AND generation=? AND native_session_id=?)`), event.SessionID, event.IncarnationID, event.HarnessGeneration, nativeID).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return nil
}

// Opaque ordinary events retain their bytes without gaining turn authority.
// Any existing admission evidence keeps malformed payloads fail-closed.
func (r *Repository) validateOpaqueOrdinaryPayloadTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent, decodeErr error) error {
	var bound bool
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM agent_delivery_effects WHERE effect_key=? AND effect_type=?)`), promptAdmissionEffect+":"+event.SubmissionID, promptAdmissionEffect).Scan(&bound); err != nil {
		return err
	}
	if bound {
		return decodeErr
	}
	return nil
}
