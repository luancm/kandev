package sqlite

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type admittedToolFixture struct {
	t      *testing.T
	repo   *Repository
	row    *models.TaskSession
	intent *models.AgentDeliveryEffect
	seq    int64
}

func TestOrdinaryToolReplayKeepsExistingAttribution(t *testing.T) {
	f := newAdmittedToolFixture(t, newRepoForSessionTests(t))
	f.project(f.event(f.intent.SubmissionID, admittedToolEvent()))
	late := f.event("", streams.AgentEvent{Type: streams.EventTypeToolUpdate, ToolCallID: "exec-A", ToolStatus: "cancelled"})
	turn, err := f.repo.ResolveAgentDeliveryEventTurn(t.Context(), late)
	require.NoError(t, err)

	require.Empty(t, turn, "ordinary callback attribution is unchanged")
	f.project(late)
	var count int
	require.NoError(t, f.repo.db.Get(&count, "SELECT COUNT(*) FROM agent_delivery_effects WHERE effect_type=$1 AND session_id=$2", "agent_delivery.admitted_tool_source.v1", f.row.ID))
	require.Zero(t, count, "ordinary sessions do not accumulate Errand tool-source receipts")
}

func newAdmittedToolFixture(t *testing.T, repo *Repository) *admittedToolFixture {
	t.Helper()
	row, intent := seedExactOrdinaryAdmission(t, repo)
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
