package sqlite

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestOrdinaryPromptAdmissionSurvivesInventoryReplacement(t *testing.T) {
	exerciseOrdinaryPromptAdmissionSurvivesInventoryReplacement(t, newRepoForSessionTests(t))
}
func TestPostgresOrdinaryPromptAdmissionSurvivesInventoryReplacement(t *testing.T) {
	exerciseOrdinaryPromptAdmissionSurvivesInventoryReplacement(t, newPostgresErrandRepo(t))
}

func exerciseOrdinaryPromptAdmissionSurvivesInventoryReplacement(t *testing.T, repo *Repository) {
	row, intent := seedExactOrdinaryAdmission(t, repo)
	binding := map[string]any{"task_id": row.TaskID, "session_id": row.ID, "execution_id": "execution", "incarnation_id": intent.IncarnationID, "harness_generation": 1, "prompt_generation": 7, "stream_id": intent.StreamID, "submission_id": intent.SubmissionID, "turn_id": intent.TurnID}
	running := &models.ExecutorRunning{ID: row.ID, SessionID: row.ID, TaskID: row.TaskID, AgentExecutionID: "execution", Metadata: map[string]any{"delivery_prompt_turn_binding": binding}}
	_, err := repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{ID: intent.SubmissionID, SessionID: row.ID, IncarnationID: intent.IncarnationID, HarnessGeneration: 1, PayloadHash: "hash", Payload: []byte(`{}`)})
	require.NoError(t, err)
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), running))
	require.NoError(t, repo.CompleteTurn(t.Context(), intent.TurnID))
	running.Metadata = nil
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), running))
	var retained int
	require.NoError(t, repo.db.Get(&retained, `SELECT COUNT(*) FROM agent_delivery_effects WHERE effect_type='agent_delivery.prompt_admission.v1'`))
	require.Equal(t, 1, retained, "replacement must retain immutable admitted ownership")
	// A conflicting admission must roll back the running-row write too.
	running.Metadata = map[string]any{"delivery_prompt_turn_binding": binding}
	binding["prompt_generation"] = 8
	require.Error(t, repo.UpsertExecutorRunning(t.Context(), running))
	saved, err := repo.GetExecutorRunningBySessionID(t.Context(), row.ID)
	require.NoError(t, err)
	require.Empty(t, saved.Metadata)
	binding["prompt_generation"] = 7
	require.NoError(t, repo.DeleteExecutorRunningBySessionID(t.Context(), row.ID))
	// A successor turn cannot acquire an existing prompt admission.
	_, err = repo.db.Exec(`INSERT INTO task_session_turns(id,task_session_id,task_id,started_at,created_at,updated_at) VALUES('successor',$1,$2,$3,$3,$3)`, row.ID, row.TaskID, errandScenarioBase.Add(1))
	require.NoError(t, err)
	for _, tc := range []struct {
		name, turn string
		generation uint64
		reject     bool
	}{
		{"late original", "", 7, false}, {"explicit original", intent.TurnID, 7, false}, {"conflicting turn", "successor", 7, true}, {"conflicting generation", "", 8, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(streams.AgentEvent{Type: "message_chunk", Text: "late A", CanonicalMessageID: tc.name, PromptGeneration: tc.generation, TurnID: tc.turn})
			require.NoError(t, err)
			event := &models.AgentDeliveryEvent{SessionID: row.ID, IncarnationID: intent.IncarnationID, HarnessGeneration: 1, StreamID: intent.StreamID, Sequence: 1, SubmissionID: intent.SubmissionID, EventType: "message_chunk", Payload: payload}
			if tc.reject {
				event.Sequence = 3
				_, err = repo.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
				require.Error(t, err)
				return
			}
			// Each accepted variant uses a separate sequence; replay remains idempotent.
			if tc.turn != "" {
				event.Sequence = 2
			}
			_, err = repo.ReceiveAgentDeliveryEvent(t.Context(), event, event.Sequence)
			require.NoError(t, err)
			result, err := repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), event, nil)
			require.NoError(t, err)
			require.NotEmpty(t, result)
			var turn string
			require.NoError(t, repo.db.Get(&turn, `SELECT turn_id FROM task_session_messages WHERE task_session_id=$1 ORDER BY created_at DESC LIMIT 1`, row.ID))
			require.Equal(t, intent.TurnID, turn)
			_, err = repo.ReceiveAgentDeliveryEvent(t.Context(), event, event.Sequence)
			require.NoError(t, err)
		})
	}
	terminal, err := json.Marshal(streams.AgentEvent{Type: "complete", PromptGeneration: 7})
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{
		SessionID: row.ID, IncarnationID: intent.IncarnationID, HarnessGeneration: 1, StreamID: intent.StreamID,
		Sequence: 3, SubmissionID: intent.SubmissionID, EventType: "complete", Terminal: true, Payload: terminal,
	}
	_, err = repo.ReceiveAgentDeliveryEvent(t.Context(), event, 3)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(t.Context(), event, nil)
	require.NoError(t, err)
	var terminalTurn string
	require.NoError(t, repo.db.Get(&terminalTurn, `SELECT turn_id FROM agent_delivery_effects WHERE effect_type='agent_delivery.terminal_settlement' AND sequence=3`))
	require.Equal(t, intent.TurnID, terminalTurn, "terminal projection must resolve the same immutable admission")
}

func seedExactOrdinaryAdmission(t *testing.T, repo *Repository) (*models.TaskSession, *models.AgentDeliveryEffect) {
	t.Helper()
	ctx := t.Context()
	task := &models.Task{ID: "exact-task", Title: "Exact prompt admission"}
	session := &models.TaskSession{ID: "exact-session", TaskID: task.ID}
	require.NoError(t, repo.CreateTask(ctx, task))
	require.NoError(t, repo.CreateTaskSession(ctx, session))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "exact-A", TaskID: task.ID, TaskSessionID: session.ID, StartedAt: errandScenarioBase,
	}))
	stored, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	return stored, &models.AgentDeliveryEffect{
		SessionID: stored.ID, IncarnationID: stored.QueueIncarnationID, StreamID: "exact-execution",
		HarnessGeneration: 1, SubmissionID: "prompt:exact-A", TurnID: "exact-A",
	}
}
