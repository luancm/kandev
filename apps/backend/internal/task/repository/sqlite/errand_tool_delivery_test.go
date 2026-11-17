package sqlite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestErrandToolDelivery(t *testing.T) {
	exerciseErrandToolDelivery(t, newRepoForSessionTests(t), true)
}
func TestPostgresErrandToolDelivery(t *testing.T) {
	exerciseErrandToolDelivery(t, newPostgresErrandRepo(t), false)
}
func exerciseErrandToolDelivery(t *testing.T, repo *Repository, injectFailure bool) {
	t.Helper()
	ctx := context.Background()
	task := newErrandTask(t, repo, "tool-delivery")
	mustCreateErrand(t, repo, sampleErrand("tool-errand", task, "parent", errandScenarioBase))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "tool-errand", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "tool-errand", 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &models.ErrandHarnessBinding{ErrandID: "tool-errand", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "tool-stream", NativeSessionID: "native-a"}, 0))
	require.NoError(t, repo.CreateErrandTurn(ctx, "tool-errand", &models.Turn{ID: "turn", TaskID: task}))
	sub := &models.AgentDeliverySubmission{ID: "submission", Payload: []byte("prompt"), PayloadHash: journal.SubmissionHash([]byte("prompt"))}
	_, err = repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{ErrandID: "tool-errand", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "tool-stream", NativeSessionID: "native-a", PromptGeneration: 1, TurnID: "turn", Submission: sub})
	require.NoError(t, err)
	makeEvent := func(seq int64, agent streams.AgentEvent) *models.AgentDeliveryEvent {
		agent.TurnID = "turn"
		agent.PromptGeneration = 1
		payload, e := json.Marshal(agent)
		require.NoError(t, e)
		return &models.AgentDeliveryEvent{SessionID: "errand:tool-errand", TaskID: task, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, OwnerGeneration: 1, ExpectedStreamID: "tool-stream", StreamID: "tool-stream", Sequence: seq, SubmissionID: "submission", EventType: agent.Type, Payload: payload}
	}
	start := makeEvent(1, streams.AgentEvent{Type: streams.EventTypeToolCall, ToolCallID: "tool-1", ParentToolCallID: "parent-tool", ProtocolMessageID: "source-a", ToolTitle: "Read file", ToolStatus: "pending", NormalizedPayload: streams.NewReadFile("/repo/a.go", 0, 0)})
	wrongStart := *start
	wrongPayload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeToolCall, SessionID: "native-b", ToolCallID: "tool-1", ParentToolCallID: "parent-tool", ToolTitle: "Read file", ToolStatus: "pending", TurnID: "turn", PromptGeneration: 1, NormalizedPayload: streams.NewReadFile("/repo/a.go", 0, 0)})
	require.NoError(t, err)
	wrongStart.Payload = wrongPayload
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, &wrongStart, 1)
	require.Error(t, err)
	inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, start, 1)
	require.NoError(t, err)
	require.True(t, inserted)
	collisionID := errandToolMessageID(start, "turn", streams.AgentEvent{ToolCallID: "tool-1", ParentToolCallID: "parent-tool"})
	require.NoError(t, repo.CreateErrandMessage(ctx, "tool-errand", &models.Message{ID: collisionID, TaskID: task, TurnID: "turn", AuthorType: models.MessageAuthorUser, Content: "untouched", Type: models.MessageTypeMessage}))
	_, err = repo.ProjectAgentDeliveryEvent(ctx, start, nil)
	require.Error(t, err)
	collision, err := repo.GetErrandMessage(ctx, "tool-errand", collisionID)
	require.NoError(t, err)
	require.Equal(t, "untouched", collision.Content)
	before, err := repo.GetAgentDeliveryCursor(ctx, "tool-stream")
	require.NoError(t, err)
	require.EqualValues(t, 0, before.ProjectedSequence)
	var owners int
	require.NoError(t, repo.db.GetContext(ctx, &owners, repo.db.Rebind(`SELECT COUNT(*) FROM agent_delivery_effects WHERE effect_key=?`), "agent_delivery.errand_tool_owner.v1:"+collisionID))
	require.Zero(t, owners)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`DELETE FROM errand_messages WHERE errand_id=? AND id=?`), "tool-errand", collisionID)
	require.NoError(t, err)
	effect := &models.AgentDeliveryEffect{EffectKey: "tool-event-1", StreamID: "tool-stream", Sequence: 1, EffectType: "agent_delivery.event", State: models.DeliveryEffectCompleted}
	projected, err := repo.ProjectAgentDeliveryEvent(ctx, start, effect)
	require.NoError(t, err)
	require.True(t, projected)
	messages, _, err := repo.ListErrandMessages(ctx, "tool-errand", "", 10)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, models.MessageTypeToolRead, messages[0].Type)
	require.Equal(t, "tool-1", messages[0].Metadata["tool_call_id"])
	require.Equal(t, "parent-tool", messages[0].Metadata["parent_tool_call_id"])
	require.Empty(t, messages[0].TaskSessionID)
	duplicate, err := repo.ProjectAgentDeliveryEvent(ctx, start, effect)
	require.NoError(t, err)
	require.False(t, duplicate)
	_, err = repo.ReserveErrandExecutionGeneration(ctx, "tool-errand", 1)
	require.NoError(t, err)
	update := makeEvent(2, streams.AgentEvent{Type: streams.EventTypeToolUpdate, ToolCallID: "tool-1", ParentToolCallID: "parent-tool", ToolTitle: "Read file complete", ToolStatus: "completed", NormalizedPayload: streams.NewReadFile("/repo/a.go", 0, 0)})
	update.SubmissionID = ""
	bad := *update
	badPayload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeToolUpdate, SessionID: "native-b", ToolCallID: "tool-1", ParentToolCallID: "parent-tool", ToolStatus: "completed", TurnID: "turn", PromptGeneration: 1})
	require.NoError(t, err)
	bad.Payload = badPayload
	_, _, err = repo.ResolveAgentDeliveryEventTurn(ctx, &bad)
	require.Error(t, err)
	tampered, err := repo.GetErrandMessage(ctx, "tool-errand", collisionID)
	require.NoError(t, err)
	tampered.Metadata["parent_tool_call_id"] = "foreign-parent"
	require.NoError(t, repo.UpdateErrandMessage(ctx, "tool-errand", tampered))
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, update, 2)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, update, nil)
	require.Error(t, err)
	tampered.Metadata["parent_tool_call_id"] = "parent-tool"
	require.NoError(t, repo.UpdateErrandMessage(ctx, "tool-errand", tampered))
	if injectFailure {
		_, err = repo.db.Exec(`CREATE TRIGGER reject_tool_update BEFORE UPDATE ON errand_messages BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
		require.NoError(t, err)
		_, err = repo.ProjectAgentDeliveryEvent(ctx, update, &models.AgentDeliveryEffect{EffectKey: "tool-event-2", StreamID: "tool-stream", Sequence: 2, EffectType: "agent_delivery.event", State: models.DeliveryEffectCompleted})
		require.Error(t, err)
		unchanged, e := repo.GetErrandMessage(ctx, "tool-errand", collisionID)
		require.NoError(t, e)
		require.Equal(t, "pending", unchanged.Metadata["status"])
		c, e := repo.GetAgentDeliveryCursor(ctx, "tool-stream")
		require.NoError(t, e)
		require.EqualValues(t, 1, c.ProjectedSequence)
		var effects int
		require.NoError(t, repo.db.GetContext(ctx, &effects, repo.db.Rebind(`SELECT COUNT(*) FROM agent_delivery_effects WHERE effect_key=?`), "tool-event-2"))
		require.Zero(t, effects)
		_, err = repo.db.Exec(`DROP TRIGGER reject_tool_update`)
		require.NoError(t, err)
	}
	_, err = repo.ProjectAgentDeliveryEvent(ctx, update, nil)
	require.NoError(t, err)
	messages, _, err = repo.ListErrandMessages(ctx, "tool-errand", "", 10)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "completed", messages[0].Metadata["status"])
	require.Equal(t, "Read file complete", messages[0].Content)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, "tool-stream")
	require.NoError(t, err)
	require.EqualValues(t, 2, cursor.ProjectedSequence)
	successor, err := repo.GetErrandExecutionDescriptor(ctx, "tool-errand")
	require.NoError(t, err)
	require.EqualValues(t, 2, successor.Generation)
	require.Empty(t, successor.ExecutionID)
	removed, err := repo.GetErrandMessage(ctx, "tool-errand", collisionID)
	require.NoError(t, err)
	removed.Metadata["payload_retention"] = map[string]interface{}{"version": 1}
	require.NoError(t, repo.UpdateErrandMessage(ctx, "tool-errand", removed))
	removed, err = repo.GetErrandMessage(ctx, "tool-errand", collisionID)
	require.NoError(t, err)
	removedAt := removed.UpdatedAt
	afterRemoval := makeEvent(3, streams.AgentEvent{Type: streams.EventTypeToolUpdate, ToolCallID: "tool-1", ParentToolCallID: "parent-tool", ProtocolMessageID: "source-a", ToolTitle: "After removal", ToolStatus: "completed", NormalizedPayload: streams.NewReadFile("/secret", 0, 0)})
	afterRemoval.SubmissionID = ""
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, afterRemoval, 3)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, afterRemoval, nil)
	require.NoError(t, err)
	after, err := repo.GetErrandMessage(ctx, "tool-errand", collisionID)
	require.NoError(t, err)
	require.True(t, after.UpdatedAt.Equal(removedAt))
	require.Equal(t, "After removal", after.Content)
	encoded, err := json.Marshal(after.Metadata)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "/secret")
	repeated := makeEvent(4, streams.AgentEvent{Type: streams.EventTypeToolCall, ToolCallID: "tool-1", ParentToolCallID: "parent-tool", ProtocolMessageID: "source-b", ToolTitle: "Different start", ToolStatus: "pending", NormalizedPayload: streams.NewReadFile("/repo/b.go", 0, 0)})
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, repeated, 4)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, repeated, nil)
	require.Error(t, err)
	unchanged, err := repo.GetErrandMessage(ctx, "tool-errand", collisionID)
	require.NoError(t, err)
	require.Equal(t, "After removal", unchanged.Content)
	ambiguous := makeEvent(5, streams.AgentEvent{Type: streams.EventTypeToolUpdate, ToolCallID: "tool-1", ParentToolCallID: "parent-tool", ToolStatus: "completed"})
	ambiguous.SubmissionID = ""
	_, _, err = repo.ResolveAgentDeliveryEventTurn(ctx, ambiguous)
	require.Error(t, err)
	exactPayload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeToolUpdate, ToolCallID: "tool-1", ParentToolCallID: "parent-tool", ProtocolMessageID: "source-a", ToolStatus: "completed", TurnID: "turn", PromptGeneration: 1})
	require.NoError(t, err)
	ambiguous.Payload = exactPayload
	turn, _, err := repo.ResolveAgentDeliveryEventTurn(ctx, ambiguous)
	require.NoError(t, err)
	require.Equal(t, "turn", turn)
}

func TestErrandToolTombstoneSurvivesStaleUpdate(t *testing.T) {
	exerciseErrandToolTombstone(t, newRepoForSessionTests(t))
}
func TestPostgresErrandToolTombstoneSurvivesStaleUpdate(t *testing.T) {
	exerciseErrandToolTombstone(t, newPostgresErrandRepo(t))
}
func exerciseErrandToolTombstone(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	task := newErrandTask(t, repo, "tool-retention")
	mustCreateErrand(t, repo, sampleErrand("retention", task, "parent", errandScenarioBase))
	require.NoError(t, repo.CreateErrandTurn(ctx, "retention", &models.Turn{ID: "retention-turn", TaskID: task}))
	payload := streams.NewGeneric("Search", map[string]any{"query": "secret"})
	payload.Generic().Output = strings.Repeat("sensitive output", 100)
	msg := &models.Message{ID: "retention-message", TaskID: task, TurnID: "retention-turn", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolCall, Metadata: map[string]interface{}{"tool_call_id": "tool", "status": "pending", "normalized": payload}}
	require.NoError(t, repo.CreateErrandMessage(ctx, "retention", msg))
	stale, err := repo.GetErrandMessage(ctx, "retention", msg.ID)
	require.NoError(t, err)
	metadata, err := json.Marshal(msg.Metadata)
	require.NoError(t, err)
	reduced, err := models.ReduceToolPayload(string(msg.Type), metadata, errandScenarioBase)
	require.NoError(t, err)
	require.Empty(t, reduced.Reason)
	require.NoError(t, json.Unmarshal(reduced.Metadata, &msg.Metadata))
	require.NoError(t, repo.UpdateErrandMessage(ctx, "retention", msg))
	stale.Metadata["status"] = "completed"
	stale.Metadata["result"] = "sensitive output"
	stale.Type = models.MessageTypeToolRead
	require.NoError(t, repo.UpdateErrandMessage(ctx, "retention", stale))
	got, err := repo.GetErrandMessage(ctx, "retention", msg.ID)
	require.NoError(t, err)
	require.True(t, models.ToolPayloadRemoved(got.Metadata))
	require.Equal(t, models.MessageTypeToolCall, got.Type)
	require.Equal(t, "completed", got.Metadata["status"])
	require.NotContains(t, got.Metadata, "result")
	encoded, err := json.Marshal(got.Metadata)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "sensitive output")
}
