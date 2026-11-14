package sqlite

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

type admittedToolFixture struct {
	t      *testing.T
	repo   *Repository
	row    *models.Errand
	intent *models.AgentDeliveryEffect
	seq    int64
}

func TestErrandAdmittedToolSourceEvidence(t *testing.T) {
	exerciseAdmittedToolSourceEvidence(t, newRepoForSessionTests)
}

func TestPostgresErrandAdmittedToolSourceEvidence(t *testing.T) {
	exerciseAdmittedToolSourceEvidence(t, newPostgresErrandRepo)
}

func exerciseAdmittedToolSourceEvidence(t *testing.T, repository func(*testing.T) *Repository) {
	for _, scenario := range []string{"retained", "historical", "historical_pruned", "historical_corrupt", "missing",
		"ambiguous", "explicit_generation", "unowned_competitor", "native", "parent", "generation", "control", "execution", "incarnation", "harness", "missing_admission"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAdmittedToolFixture(t, repository(t))
			source := admittedToolEvent()
			source.ControlTurnID = 1
			if scenario != "missing" {
				f.project(f.event(f.intent.SubmissionID, source))
			}
			switch scenario {
			case "historical", "historical_pruned", "historical_corrupt":
				_, err := f.repo.db.Exec("DELETE FROM agent_delivery_effects WHERE effect_type=$1 AND session_id=$2", "agent_delivery.admitted_tool_source.v1", f.row.ID)
				require.NoError(t, err)
			case "ambiguous", "explicit_generation":
				require.NoError(t, f.repo.CreateTurn(t.Context(), &models.Turn{ID: "successor", TaskID: f.row.TaskID,
					TaskSessionID: f.row.ID, StartedAt: errandScenarioBase.Add(1)}))
				f.admit("successor", "prompt:successor", 8)
				source.PromptGeneration = 8
				f.project(f.event("prompt:successor", source))
			case "unowned_competitor":
				f.project(f.event("", source))
			}
			switch scenario {
			case "retained", "historical_pruned":
				_, err := f.repo.db.Exec("UPDATE agent_delivery_inbox SET payload=$1,payload_pruned_at=$2 WHERE stream_id=$3", []byte("{}"), errandScenarioBase, f.intent.StreamID)
				require.NoError(t, err)
			case "historical_corrupt":
				_, err := f.repo.db.Exec("UPDATE agent_delivery_inbox SET payload=$1 WHERE stream_id=$2", []byte(`{"type":"tool_call","tool_call_id":"exec-A"}`), f.intent.StreamID)
				require.NoError(t, err)
			case "missing_admission":
				_, err := f.repo.db.Exec("DELETE FROM agent_delivery_effects WHERE effect_type=$1 AND session_id=$2", promptAdmissionEffect, f.row.ID)
				require.NoError(t, err)
			}
			f.repo = NewWithInitializedDB(f.repo.db, f.repo.ro, nil)
			late := streams.AgentEvent{Type: streams.EventTypeToolUpdate, SessionID: "native-A", ToolCallID: "exec-A", ToolStatus: "cancelled"}
			switch scenario {
			case "native":
				late.SessionID = "native-B"
			case "parent":
				late.ParentToolCallID = "other-parent"
			case "generation":
				late.PromptGeneration = 99
			case "control":
				late.ControlTurnID = 99
			case "explicit_generation":
				late.PromptGeneration = 7
			}
			event := f.event("", late)
			switch scenario {
			case "execution":
				event.ExecutionID = "replacement"
			case "incarnation":
				event.IncarnationID = "replacement"
			case "harness":
				event.HarnessGeneration = 2
			}
			turn, _, err := f.repo.ResolveAgentDeliveryEventTurn(t.Context(), event)
			if scenario == "retained" || scenario == "historical" || scenario == "explicit_generation" {
				require.NoError(t, err)
				require.Equal(t, f.intent.TurnID, turn)
				f.project(event)
				if scenario == "historical" {
					_, err := f.repo.db.Exec("UPDATE agent_delivery_inbox SET payload=$1,payload_pruned_at=$2 WHERE stream_id=$3 AND sequence=1", []byte("{}"), errandScenarioBase, f.intent.StreamID)
					require.NoError(t, err)
					f.repo = NewWithInitializedDB(f.repo.db, f.repo.ro, nil)
					turn, _, err = f.repo.ResolveAgentDeliveryEventTurn(t.Context(), event)
					require.NoError(t, err, "accepted historical attribution survives later compaction")
					require.Equal(t, f.intent.TurnID, turn)
				}
				return
			}
			require.Error(t, err)
			_, err = f.repo.ReceiveAgentDeliveryEvent(t.Context(), event, event.Sequence)
			require.Error(t, err)
			var count int
			require.NoError(t, f.repo.db.Get(&count, "SELECT COUNT(*) FROM agent_delivery_inbox WHERE stream_id=$1 AND sequence=$2", event.StreamID, event.Sequence))
			require.Zero(t, count, "rejected evidence must not advance the inbox")
			if scenario == "ambiguous" || scenario == "unowned_competitor" {
				require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryIdentityMismatch)
			}
		})
	}
}

func TestOrdinaryToolReplayKeepsExistingAttribution(t *testing.T) {
	f := newAdmittedToolFixture(t, newRepoForSessionTests(t))
	_, err := f.repo.db.Exec("UPDATE task_sessions SET role='' WHERE id=$1", f.row.ID)
	require.NoError(t, err)
	f.project(f.event(f.intent.SubmissionID, admittedToolEvent()))
	late := f.event("", streams.AgentEvent{Type: streams.EventTypeToolUpdate, ToolCallID: "exec-A", ToolStatus: "cancelled"})
	turn, errand, err := f.repo.ResolveAgentDeliveryEventTurn(t.Context(), late)
	require.NoError(t, err)
	require.False(t, errand)
	require.Empty(t, turn, "ordinary callback attribution is unchanged")
	f.project(late)
	var count int
	require.NoError(t, f.repo.db.Get(&count, "SELECT COUNT(*) FROM agent_delivery_effects WHERE effect_type=$1 AND session_id=$2", "agent_delivery.admitted_tool_source.v1", f.row.ID))
	require.Zero(t, count, "ordinary sessions do not accumulate Errand tool-source receipts")
}

func newAdmittedToolFixture(t *testing.T, repo *Repository) *admittedToolFixture {
	t.Helper()
	row, intent := seedExactSettlement(t, repo)
	f := &admittedToolFixture{t: t, repo: repo, row: row, intent: intent}
	committed, err := repo.CommitHarnessSessionGeneration(t.Context(), &models.HarnessSessionGeneration{
		SessionID: row.ID, IncarnationID: intent.IncarnationID, Generation: 1,
		NativeSessionID: "native-A", CreationReason: "initial", CreatedAt: errandScenarioBase, CommittedAt: errandScenarioBase,
	}, 0)
	require.NoError(t, err)
	require.True(t, committed)
	f.admit(intent.TurnID, intent.SubmissionID, 7)
	return f
}

func (f *admittedToolFixture) admit(turn, submission string, generation uint64) {
	f.t.Helper()
	_, err := f.repo.PrepareAgentDeliverySubmission(f.t.Context(), &models.AgentDeliverySubmission{
		ID: submission, SessionID: f.row.ID, IncarnationID: f.intent.IncarnationID,
		HarnessGeneration: 1, PayloadHash: submission, Payload: []byte("{}"),
	})
	require.NoError(f.t, err)
	require.NoError(f.t, f.repo.UpsertExecutorRunning(f.t.Context(), &models.ExecutorRunning{
		ID: f.row.ID, SessionID: f.row.ID, TaskID: f.row.TaskID, AgentExecutionID: "execution",
		Metadata: map[string]any{"delivery_prompt_turn_binding": map[string]any{
			"task_id": f.row.TaskID, "session_id": f.row.ID, "execution_id": "execution",
			"incarnation_id": f.intent.IncarnationID, "harness_generation": 1,
			"prompt_generation": generation, "stream_id": f.intent.StreamID,
			"submission_id": submission, "turn_id": turn, "native_session_id": "native-A",
		}},
	}))
}

func (f *admittedToolFixture) event(submission string, agent streams.AgentEvent) *models.AgentDeliveryEvent {
	f.t.Helper()
	f.seq++
	raw, err := json.Marshal(agent)
	require.NoError(f.t, err)
	return &models.AgentDeliveryEvent{SessionID: f.row.ID,
		IncarnationID: f.intent.IncarnationID, HarnessGeneration: 1, StreamID: f.intent.StreamID,
		SubmissionID: submission, Sequence: f.seq, EventType: agent.Type, Payload: raw,
		Terminal: agent.Type == streams.EventTypeComplete}
}

func (f *admittedToolFixture) project(event *models.AgentDeliveryEvent) {
	f.t.Helper()
	_, err := f.repo.ReceiveAgentDeliveryEvent(f.t.Context(), event, event.Sequence)
	require.NoError(f.t, err)
	_, err = f.repo.ProjectCanonicalAgentDeliveryEvent(f.t.Context(), event, nil)
	require.NoError(f.t, err)
}

func admittedToolEvent() streams.AgentEvent {
	return streams.AgentEvent{Type: streams.EventTypeToolCall, SessionID: "native-A",
		ToolCallID: "exec-A", ToolStatus: "completed", PromptGeneration: 7,
		NormalizedPayload: streams.NewShellExec("true", "/tmp", "original output", 0, false)}
}

// @covers AC-TASKS-ERRANDS-002.7, AC-TASKS-ERRANDS-003.7
func TestErrandLateOwnedToolTerminalReplay(t *testing.T) {
	exerciseLateOwnedToolTerminalReplay(t, newRepoForSessionTests(t))
}

func TestPostgresErrandLateOwnedToolTerminalReplay(t *testing.T) {
	exerciseLateOwnedToolTerminalReplay(t, newPostgresErrandRepo(t))
}

func exerciseLateOwnedToolTerminalReplay(t *testing.T, repo *Repository) {
	f := newAdmittedToolFixture(t, repo)
	f.project(f.event(f.intent.SubmissionID, admittedToolEvent()))
	require.NoError(t, repo.CreateTurn(t.Context(), &models.Turn{ID: "successor", TaskID: f.row.TaskID,
		TaskSessionID: f.row.ID, StartedAt: errandScenarioBase.Add(1)}))
	f.admit("successor", "prompt:successor", 8)
	successor := admittedToolEvent()
	successor.ToolCallID, successor.PromptGeneration = "exec-B", 8
	successor.NormalizedPayload = streams.NewShellExec("true", "/tmp", "successor output", 0, false)
	f.project(f.event("prompt:successor", successor))
	late := f.event("", streams.AgentEvent{Type: streams.EventTypeToolUpdate, SessionID: "native-A",
		ToolCallID: "exec-A", ToolStatus: "cancelled"})
	originalBytes := append([]byte(nil), late.Payload...)
	for range 2 {
		turn, errand, err := repo.ResolveAgentDeliveryEventTurn(t.Context(), late)
		require.NoError(t, err)
		require.True(t, errand)
		require.Equal(t, f.intent.TurnID, turn)
		f.project(late)
		require.Equal(t, originalBytes, late.Payload)
	}
	terminal := f.event(f.intent.SubmissionID, streams.AgentEvent{Type: streams.EventTypeComplete,
		SessionID: "native-A", PromptGeneration: 7, Data: map[string]any{"stop_reason": "end_turn"}})
	f.project(terminal)
	f.project(terminal)
	cursor, err := repo.GetAgentDeliveryCursor(t.Context(), f.intent.StreamID)
	require.NoError(t, err)
	require.Equal(t, terminal.Sequence, cursor.ReceivedSequence)
	require.Equal(t, terminal.Sequence, cursor.ProjectedSequence)
	var ids []string
	require.NoError(t, repo.db.Select(&ids, "SELECT id FROM task_session_messages WHERE task_session_id=$1 AND type='tool_execute'", f.row.ID))
	require.Len(t, ids, 2)
	for _, id := range ids {
		message, err := repo.GetMessage(t.Context(), id)
		require.NoError(t, err)
		if message.TurnID == "successor" {
			require.Equal(t, "completed", message.Metadata["status"])
		}
	}
	var effects int
	require.NoError(t, repo.db.Get(&effects, "SELECT COUNT(*) FROM agent_delivery_effects WHERE stream_id=$1 AND effect_type=$2 AND turn_id=$3", f.intent.StreamID, models.DeliveryTerminalEffectType, f.intent.TurnID))
	require.Equal(t, 1, effects, "the following terminal keeps its own settlement authority")
}
