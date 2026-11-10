package sqlite

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestErrandShellOwnership(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "admitted", true: "unowned"}[native], func(t *testing.T) { exerciseErrandShellOwnership(t, newRepoForSessionTests(t), native) })
	}
}
func TestPostgresErrandShellOwnership(t *testing.T) {
	exerciseErrandShellOwnership(t, newPostgresErrandRepo(t), true)
}

func exerciseErrandShellOwnership(t *testing.T, repo *Repository, native bool) {
	row, intent := seedExactSettlement(t, repo)
	admit := func(turn, submission string) {
		_, err := repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{ID: submission, SessionID: row.ID, IncarnationID: intent.IncarnationID, HarnessGeneration: 1, PayloadHash: submission, Payload: []byte("{}")})
		require.NoError(t, err)
		require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: row.ID, SessionID: row.ID, TaskID: row.TaskID, AgentExecutionID: "execution", Metadata: map[string]any{"delivery_prompt_turn_binding": map[string]any{"task_id": row.TaskID, "session_id": row.ID, "execution_id": "execution", "incarnation_id": intent.IncarnationID, "harness_generation": 1, "prompt_generation": 7, "stream_id": intent.StreamID, "submission_id": submission, "turn_id": turn}}}))
	}
	admit(intent.TurnID, intent.SubmissionID)
	seq := int64(0)
	send := func(submission, protocol string, patch *streams.ShellOutputPatch) {
		seq++
		kind := streams.EventTypeToolCall
		if patch != nil {
			kind = streams.EventTypeShellOutputUpdate
		}
		payload, err := json.Marshal(streams.AgentEvent{Type: kind, ToolCallID: "reused", ProtocolMessageID: protocol, PromptGeneration: 7, ToolStatus: "running", ShellOutput: patch, NormalizedPayload: streams.NewShellExec("command", "/tmp", "", 0, false)})
		require.NoError(t, err)
		event := &models.AgentDeliveryEvent{SessionID: row.ID, IncarnationID: intent.IncarnationID, HarnessGeneration: 1, StreamID: intent.StreamID, SubmissionID: submission, Sequence: seq, EventType: kind, Payload: payload}
		for replay := range 2 {
			if replay == 1 {
				var reconstructed streams.AgentEvent
				require.NoError(t, json.Unmarshal(event.Payload, &reconstructed))
				reconstructed.CanonicalMessageID = "reconstructed"
				reconstructed.CanonicalMessageBaseID = "different-base"
				event.Payload, err = json.Marshal(reconstructed)
				require.NoError(t, err)
			}
			_, err = repo.ReceiveAgentDeliveryEvent(t.Context(), event, seq)
			require.NoError(t, err)
			_, err = repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), event, nil)
			require.NoError(t, err)
		}
	}
	checkpoint := func(output string) *streams.ShellOutputPatch {
		p, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: output}, "epoch", true)
		require.NoError(t, err)
		return p
	}
	send(intent.SubmissionID, "a-start", nil)
	a := checkpoint("A original")
	send(intent.SubmissionID, "a-output", a)
	require.NoError(t, repo.CompleteTurn(t.Context(), intent.TurnID))
	bSub := ""
	if !native {
		_, err := repo.db.Exec("INSERT INTO task_session_turns(id,task_session_id,task_id,started_at,created_at,updated_at) VALUES('shell-successor',$1,$2,$3,$3,$3)", row.ID, row.TaskID, errandScenarioBase.Add(1))
		require.NoError(t, err)
		bSub = "shell-successor-submission"
		admit("shell-successor", bSub)
	}
	send(bSub, "b-start", nil)
	var ids []string
	require.NoError(t, repo.db.Select(&ids, "SELECT id FROM task_session_messages WHERE task_session_id=$1 AND type='tool_execute' ORDER BY created_at,id", row.ID))
	require.Len(t, ids, 2, "successor tool ID reuse must create a separate message")
	b := checkpoint("B output")
	send(bSub, "b-different-update-id", b)
	read := func(id string) string {
		m, err := repo.GetMessage(t.Context(), id)
		require.NoError(t, err)
		require.NoError(t, repo.RehydrateMessagePayload(t.Context(), m))
		o, ok := models.ExtractShellExecOutput(m.Metadata)
		require.True(t, ok)
		return o.Stdout
	}
	require.Equal(t, "A original", read(ids[0]))
	require.Equal(t, "B output", read(ids[1]))
	state, err := streams.ApplyShellOutputPatch(nil, a)
	require.NoError(t, err)
	late, err := streams.EncodeShellOutputPatch(state, streams.ShellExecOutput{Stdout: "A original late"}, "epoch", false)
	require.NoError(t, err)
	send(intent.SubmissionID, "late-a", late)
	require.Equal(t, "A original late", read(ids[0]))
	require.Equal(t, "B output", read(ids[1]))
	bState, err := streams.ApplyShellOutputPatch(nil, b)
	require.NoError(t, err)
	next, err := streams.EncodeShellOutputPatch(bState, streams.ShellExecOutput{Stdout: "B output continued"}, "epoch", false)
	require.NoError(t, err)
	send(bSub, "b-yet-another-update", next)
	require.Equal(t, "A original late", read(ids[0]))
	require.Equal(t, "B output continued", read(ids[1]))
}

func TestErrandShellSourceEvidence(t *testing.T) {
	for _, scenario := range []string{"missing", "conflicting_control", "ambiguous", "parent", "repeated_start", "retained", "exact_protocol", "duplicate_message"} {
		t.Run(scenario, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			row, intent := seedExactSettlement(t, repo)
			seq := int64(0)
			event := func(kind, protocol, parent string, control int64) *models.AgentDeliveryEvent {
				seq++
				agent := streams.AgentEvent{Type: kind, ToolCallID: "tool", ParentToolCallID: parent, ProtocolMessageID: protocol, ControlTurnID: control, PromptGeneration: 9, CanonicalMessageID: "mutable-" + protocol, NormalizedPayload: streams.NewShellExec("command", "/tmp", "", 0, false)}
				if kind == streams.EventTypeShellOutputUpdate {
					var err error
					agent.ShellOutput, err = streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: protocol}, "epoch", true)
					require.NoError(t, err)
				}
				raw, err := json.Marshal(agent)
				require.NoError(t, err)
				return &models.AgentDeliveryEvent{SessionID: row.ID, IncarnationID: intent.IncarnationID, HarnessGeneration: 1, StreamID: intent.StreamID, Sequence: seq, EventType: kind, Payload: raw}
			}
			project := func(e *models.AgentDeliveryEvent) {
				_, err := repo.ReceiveAgentDeliveryEvent(t.Context(), e, e.Sequence)
				require.NoError(t, err)
				_, err = repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), e, nil)
				require.NoError(t, err)
			}
			if scenario != "missing" {
				project(event(streams.EventTypeToolCall, "start", "parent-a", 1))
			}
			switch scenario {
			case "ambiguous", "exact_protocol":
				project(event(streams.EventTypeToolCall, "second", "parent-a", 1))
			case "parent":
				project(event(streams.EventTypeToolCall, "start", "parent-b", 1))
			case "repeated_start":
				project(event(streams.EventTypeToolCall, "start", "parent-a", 1))
			case "retained":
				// Inbox bodies are dispensable; source provenance and message ownership are not.
				_, err := repo.db.Exec("UPDATE agent_delivery_inbox SET payload=$1 WHERE stream_id=$2", []byte("{}"), intent.StreamID)
				require.NoError(t, err)
				repo = NewWithInitializedDB(repo.db, repo.db, nil)
			}
			update := event(streams.EventTypeShellOutputUpdate, "different-update", "parent-a", 1)
			if scenario == "conflicting_control" {
				var a streams.AgentEvent
				require.NoError(t, json.Unmarshal(update.Payload, &a))
				a.ControlTurnID = 2
				var err error
				update.Payload, err = json.Marshal(a)
				require.NoError(t, err)
			}
			if scenario == "exact_protocol" {
				var a streams.AgentEvent
				require.NoError(t, json.Unmarshal(update.Payload, &a))
				a.ProtocolMessageID = "start"
				var err error
				update.Payload, err = json.Marshal(a)
				require.NoError(t, err)
			}
			if scenario == "duplicate_message" {
				var id string
				require.NoError(t, repo.db.Get(&id, "SELECT id FROM task_session_messages WHERE task_session_id=$1", row.ID))
				message, err := repo.GetMessage(t.Context(), id)
				require.NoError(t, err)
				message.ID = "duplicate-shell"
				require.NoError(t, repo.CreateMessage(t.Context(), message))
				_, err = repo.ReceiveAgentDeliveryEvent(t.Context(), update, update.Sequence)
				require.NoError(t, err)
				_, err = repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), update, nil)
				require.Error(t, err)
				var count int
				require.NoError(t, repo.db.Get(&count, "SELECT COUNT(*) FROM task_message_shell_output"))
				require.Zero(t, count)
				return
			}
			reject := scenario == "missing" || scenario == "conflicting_control" || scenario == "ambiguous"
			if reject {
				_, err := repo.ReceiveAgentDeliveryEvent(t.Context(), update, update.Sequence)
				require.Error(t, err)
				var count int
				require.NoError(t, repo.db.Get(&count, "SELECT COUNT(*) FROM task_message_shell_output"))
				require.Zero(t, count, "rejected evidence cannot mutate any shell output")
				return
			}
			project(update)
			var count int
			require.NoError(t, repo.db.Get(&count, "SELECT COUNT(*) FROM task_message_shell_output"))
			require.Equal(t, 1, count)
			// Tool status updates resolve through the same source, even with reconstructed canonical fields.
			protocol := "another-protocol-id"
			if scenario == "exact_protocol" {
				protocol = "start"
			}
			status := event(streams.EventTypeToolUpdate, protocol, "parent-a", 1)
			turn, owned, err := repo.ResolveAgentDeliveryEventTurn(t.Context(), status)
			require.NoError(t, err)
			require.True(t, owned)
			var actual string
			require.NoError(t, repo.db.Get(&actual, "SELECT turn_id FROM task_session_messages WHERE id=(SELECT message_id FROM task_message_shell_output)"))
			require.Equal(t, actual, turn)
			project(status)
			if scenario == "parent" {
				project(event(streams.EventTypeShellOutputUpdate, "parent-b-output", "parent-b", 1))
				require.NoError(t, repo.db.Get(&count, "SELECT COUNT(*) FROM task_message_shell_output"))
				require.Equal(t, 2, count, "same tool ID under different parents must have independent output")
			}
		})
	}
}
