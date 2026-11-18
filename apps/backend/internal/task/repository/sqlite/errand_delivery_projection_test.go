package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestErrandDeliveryProjectsWithoutTaskSession(t *testing.T) {
	exerciseErrandDeliveryProjection(t, newRepoForSessionTests(t), true)
}
func TestPostgresErrandDeliveryProjectsWithoutTaskSession(t *testing.T) {
	exerciseErrandDeliveryProjection(t, newPostgresErrandRepo(t), false)
}
func exerciseErrandDeliveryProjection(t *testing.T, repo *Repository, inject bool) {
	t.Helper()
	ctx := context.Background()
	task := newErrandTask(t, repo, "projection")
	mustCreateErrand(t, repo, sampleErrand("projection", task, "parent", errandScenarioBase))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "projection", 0)
	require.NoError(t, err)
	ok, err := repo.BindErrandExecution(ctx, "projection", 1, "execution")
	require.NoError(t, err)
	require.True(t, ok)
	binding := &models.ErrandHarnessBinding{ErrandID: "projection", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 3, StreamID: "stream"}
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	require.NoError(t, repo.CreateErrandTurn(ctx, "projection", &models.Turn{ID: "turn", TaskID: task}))
	sub := &models.AgentDeliverySubmission{ID: "submission", Payload: []byte("prompt"), PayloadHash: journal.SubmissionHash([]byte("prompt"))}
	_, err = repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{ErrandID: "projection", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 3, StreamID: "stream", PromptGeneration: 1, TurnID: "turn", Submission: sub})
	require.NoError(t, err)
	payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "Hello", CanonicalMessageID: "message", CanonicalMessageBaseID: "message", TurnID: "turn", PromptGeneration: 1})
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{SessionID: models.ErrandExecutionIdentity("projection"), TaskID: task, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 3, OwnerGeneration: 3, ExpectedStreamID: "stream", StreamID: "stream", Sequence: 1, SubmissionID: "submission", EventType: streams.EventTypeMessageChunk, Payload: payload}
	if inject {
		_, err = repo.db.Exec(`CREATE TRIGGER reject_errand_message BEFORE INSERT ON errand_messages BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
		require.NoError(t, err)
		_, err = repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, []*models.AgentDeliveryEvent{event}, []*models.AgentDeliveryEffect{nil}, 1)
		require.Error(t, err)
		var inbox int
		require.NoError(t, repo.db.GetContext(ctx, &inbox, `SELECT COUNT(*) FROM agent_delivery_inbox`))
		require.Zero(t, inbox)
		_, err = repo.db.Exec(`DROP TRIGGER reject_errand_message`)
		require.NoError(t, err)
	}
	for _, foreign := range []*models.Message{
		{ID: "message", TaskID: task, TurnID: "turn", AuthorType: models.MessageAuthorUser, Content: "user content", Type: models.MessageTypeMessage},
		{ID: "message", TaskID: task, TurnID: "turn", AuthorType: models.MessageAuthorAgent, AuthorID: "other-submission", Content: "other content", Type: models.MessageTypeMessage, Metadata: map[string]interface{}{"durable_delivery": true, "delivery_stream_id": "stream"}},
	} {
		require.NoError(t, repo.CreateErrandMessage(ctx, "projection", foreign))
		_, err = repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, []*models.AgentDeliveryEvent{event}, []*models.AgentDeliveryEffect{nil}, 1)
		require.Error(t, err)
		current, readErr := repo.GetErrandMessage(ctx, "projection", "message")
		require.NoError(t, readErr)
		require.Equal(t, foreign.Content, current.Content)
		var count int
		require.NoError(t, repo.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM agent_delivery_inbox`))
		require.Zero(t, count)
		_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`DELETE FROM errand_messages WHERE errand_id=? AND id=?`), "projection", "message")
		require.NoError(t, err)
	}
	_, err = repo.ReserveErrandExecutionGeneration(ctx, "projection", 1)
	require.NoError(t, err)
	results, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, []*models.AgentDeliveryEvent{event}, []*models.AgentDeliveryEffect{nil}, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)
	retry, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, []*models.AgentDeliveryEvent{event}, []*models.AgentDeliveryEffect{nil}, 1)
	require.NoError(t, err)
	require.True(t, retry[0].AlreadyProjected)
	successor, err := repo.GetErrandExecutionDescriptor(ctx, "projection")
	require.NoError(t, err)
	require.EqualValues(t, 2, successor.Generation)
	require.Empty(t, successor.ExecutionID)
	messages, _, err := repo.ListErrandMessages(ctx, "projection", "", 10)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "Hello", messages[0].Content)
	require.Empty(t, messages[0].TaskSessionID)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, "stream")
	require.NoError(t, err)
	require.EqualValues(t, 1, cursor.ProjectedSequence)
	continuationPayload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: " world", CanonicalMessageID: "message", CanonicalMessageBaseID: "message", TurnID: "turn", PromptGeneration: 1})
	require.NoError(t, err)
	continuation := *event
	continuation.Sequence = 2
	continuation.Payload = continuationPayload
	continuationResult, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, []*models.AgentDeliveryEvent{&continuation}, []*models.AgentDeliveryEffect{nil}, 2)
	require.NoError(t, err)
	require.True(t, continuationResult[0].Appended)
	appended, err := repo.GetErrandMessage(ctx, "projection", "message")
	require.NoError(t, err)
	require.Equal(t, "Hello world", appended.Content)
	projectErrandObservedCommands(t, repo, event)
	unknownPayload, err := json.Marshal(streams.AgentEvent{Type: "future_event", TurnID: "turn", PromptGeneration: 1})
	require.NoError(t, err)
	unknown := *event
	unknown.Sequence = 4
	unknown.EventType = "future_event"
	unknown.Payload = unknownPayload
	_, err = repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, []*models.AgentDeliveryEvent{&unknown}, []*models.AgentDeliveryEffect{nil}, 4)
	require.Error(t, err)
	cursor, err = repo.GetAgentDeliveryCursor(ctx, "stream")
	require.NoError(t, err)
	require.EqualValues(t, 3, cursor.ReceivedSequence)
	terminalPayload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeComplete, TurnID: "turn", PromptGeneration: 1})
	require.NoError(t, err)
	terminal := *event
	terminal.Sequence = 4
	terminal.EventType = streams.EventTypeComplete
	terminal.Payload = terminalPayload
	terminal.Terminal = true
	received, err := repo.ReceiveAgentDeliveryEvent(ctx, &terminal, 4)
	require.NoError(t, err)
	require.True(t, received)
	projected, err := repo.ProjectAgentDeliveryEvent(ctx, &terminal, nil)
	require.NoError(t, err)
	require.True(t, projected)
	projected, err = repo.ProjectAgentDeliveryEvent(ctx, &terminal, nil)
	require.NoError(t, err)
	require.False(t, projected)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, "stream", 4, models.DeliverySubmissionCompleted, time.Time{})
	require.NoError(t, err)
	require.True(t, settled)
	stored, err := repo.GetAgentDeliverySubmission(ctx, "submission")
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionCompleted, stored.State)
	terminalMatches, err := repo.HasSettledErrandDeliverySubmission(ctx, stored, "stream", "turn")
	require.NoError(t, err)
	require.True(t, terminalMatches)
	terminalMatches, err = repo.HasSettledErrandDeliverySubmission(ctx, stored, "stream", "another-turn")
	require.NoError(t, err)
	require.False(t, terminalMatches)
	turn, err := repo.GetErrandTurn(ctx, "projection", "turn")
	require.NoError(t, err)
	require.NotNil(t, turn.CompletedAt, "a historical terminal receipt still closes its owned turn")
	var active string
	require.NoError(t, repo.db.GetContext(ctx, &active, repo.db.Rebind(`SELECT active_submission_id FROM errand_execution_generations WHERE errand_id=? AND generation=?`), "projection", 1))
	require.Empty(t, active)
	successor, err = repo.GetErrandExecutionDescriptor(ctx, "projection")
	require.NoError(t, err)
	require.EqualValues(t, 2, successor.Generation)
	require.Empty(t, successor.ExecutionID)
	var sessions int
	require.NoError(t, repo.db.GetContext(ctx, &sessions, repo.db.Rebind(`SELECT COUNT(*) FROM task_sessions WHERE task_id=?`), task))
	require.Zero(t, sessions)
}

func projectErrandObservedCommands(t *testing.T, repo *Repository, message *models.AgentDeliveryEvent) {
	t.Helper()
	ctx := t.Context()
	event := *message
	event.Sequence = 3
	event.EventType = streams.EventTypeAvailableCommands
	var err error
	event.Payload, err = json.Marshal(streams.AgentEvent{Type: event.EventType, TurnID: "turn", PromptGeneration: 1})
	require.NoError(t, err)
	received, err := repo.ReceiveAgentDeliveryEvent(ctx, &event, 3)
	require.NoError(t, err)
	require.True(t, received)
	projected, err := repo.ProjectAgentDeliveryEvent(ctx, &event, nil)
	require.NoError(t, err)
	require.True(t, projected)
	messages, _, err := repo.ListErrandMessages(ctx, "projection", "", 10)
	require.NoError(t, err)
	require.Len(t, messages, 1, "command availability is a control frame")
}

func TestErrandUnsubmittedControlReplay(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	task := newErrandTask(t, repo, "control")
	mustCreateErrand(t, repo, sampleErrand("control", task, "parent", errandScenarioBase))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "control", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "control", 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &models.ErrandHarnessBinding{ErrandID: "control", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 3, StreamID: "stream-control"}, 0))
	_, err = repo.ReserveErrandExecutionGeneration(ctx, "control", 1)
	require.NoError(t, err)
	payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeSessionMode, CurrentModeID: "plan"})
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{SessionID: "errand:control", TaskID: task, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 3, OwnerGeneration: 3, ExpectedStreamID: "stream-control", StreamID: "stream-control", Sequence: 1, EventType: streams.EventTypeSessionMode, Payload: payload}
	_, err = repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, []*models.AgentDeliveryEvent{event}, []*models.AgentDeliveryEffect{nil}, 1)
	require.NoError(t, err)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, event.StreamID)
	require.NoError(t, err)
	require.EqualValues(t, 1, cursor.ProjectedSequence)
	var raw []byte
	require.NoError(t, repo.db.GetContext(ctx, &raw, repo.db.Rebind(`SELECT payload FROM agent_delivery_inbox WHERE stream_id=? AND sequence=1`), event.StreamID))
	var stored streams.AgentEvent
	require.NoError(t, json.Unmarshal(raw, &stored))
	require.Equal(t, "plan", stored.CurrentModeID)
	payload, err = json.Marshal(streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "unowned"})
	require.NoError(t, err)
	event.Sequence = 2
	event.EventType = streams.EventTypeMessageChunk
	event.Payload = payload
	_, err = repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, []*models.AgentDeliveryEvent{event}, []*models.AgentDeliveryEffect{nil}, 2)
	require.Error(t, err)
	cursor, err = repo.GetAgentDeliveryCursor(ctx, event.StreamID)
	require.NoError(t, err)
	require.EqualValues(t, 1, cursor.ReceivedSequence)
}

func TestErrandUnsentPromptReleasesOnlyExactClaim(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	task := newErrandTask(t, repo, "unsent")
	mustCreateErrand(t, repo, sampleErrand("unsent", task, "parent", errandScenarioBase))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "unsent", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "unsent", 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &models.ErrandHarnessBinding{ErrandID: "unsent", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream-unsent"}, 0))
	require.NoError(t, repo.CreateErrandTurn(ctx, "unsent", &models.Turn{ID: "turn-unsent", TaskID: task}))
	sub := &models.AgentDeliverySubmission{ID: "prompt:unsent", SessionID: "errand:unsent", IncarnationID: "incarnation", HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "execution", State: models.DeliverySubmissionDispatching, Payload: []byte("prompt"), PayloadHash: journal.SubmissionHash([]byte("prompt"))}
	_, err = repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{ErrandID: "unsent", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream-unsent", PromptGeneration: 1, TurnID: "turn-unsent", Submission: sub})
	require.NoError(t, err)
	settled, err := repo.FailUnsentAgentDeliverySubmission(ctx, sub, time.Now())
	require.NoError(t, err)
	require.True(t, settled)
	stored, err := repo.GetAgentDeliverySubmission(ctx, sub.ID)
	require.NoError(t, err)
	terminalMatches, err := repo.HasSettledErrandDeliverySubmission(ctx, stored, "stream-unsent", "turn-unsent")
	require.NoError(t, err)
	require.False(t, terminalMatches, "a failed unsent attempt has no terminal receipt")
	var active string
	require.NoError(t, repo.db.GetContext(ctx, &active, repo.db.Rebind(`SELECT active_submission_id FROM errand_execution_generations WHERE errand_id=?`), "unsent"))
	require.Empty(t, active)
}

func TestErrandAdmittedSubmissionsCannotShareCanonicalMessage(t *testing.T) {
	exerciseErrandAdmittedSubmissionCollision(t, newRepoForSessionTests(t))
}
func TestPostgresErrandAdmittedSubmissionsCannotShareCanonicalMessage(t *testing.T) {
	exerciseErrandAdmittedSubmissionCollision(t, newPostgresErrandRepo(t))
}
func exerciseErrandAdmittedSubmissionCollision(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	task := newErrandTask(t, repo, "admitted-collision")
	mustCreateErrand(t, repo, sampleErrand("collision", task, "parent", errandScenarioBase))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "collision", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "collision", 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &models.ErrandHarnessBinding{ErrandID: "collision", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "collision-stream"}, 0))
	require.NoError(t, repo.CreateErrandTurn(ctx, "collision", &models.Turn{ID: "collision-turn", TaskID: task}))
	admit := func(id string, generation uint64) {
		sub := &models.AgentDeliverySubmission{ID: id, Payload: []byte(id), PayloadHash: journal.SubmissionHash([]byte(id)), State: models.DeliverySubmissionDispatching}
		created, e := repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{ErrandID: "collision", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "collision-stream", PromptGeneration: generation, TurnID: "collision-turn", Submission: sub})
		require.NoError(t, e)
		require.True(t, created)
	}
	makeEvent := func(sequence int64, submission string, generation uint64, kind, text string) *models.AgentDeliveryEvent {
		payload, e := json.Marshal(streams.AgentEvent{Type: kind, Text: text, CanonicalMessageID: "shared-id", CanonicalMessageBaseID: "shared-id", TurnID: "collision-turn", PromptGeneration: generation})
		require.NoError(t, e)
		return &models.AgentDeliveryEvent{SessionID: "errand:collision", TaskID: task, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, OwnerGeneration: 1, ExpectedStreamID: "collision-stream", StreamID: "collision-stream", Sequence: sequence, SubmissionID: submission, EventType: kind, Payload: payload, Terminal: kind == streams.EventTypeComplete}
	}
	project := func(event *models.AgentDeliveryEvent, effect *models.AgentDeliveryEffect) ([]models.CanonicalAgentDeliveryProjectionResult, error) {
		return repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, []*models.AgentDeliveryEvent{event}, []*models.AgentDeliveryEffect{effect}, event.Sequence)
	}
	admit("first", 1)
	_, err = project(makeEvent(1, "first", 1, streams.EventTypeMessageChunk, "first"), nil)
	require.NoError(t, err)
	continuation, err := project(makeEvent(2, "first", 1, streams.EventTypeMessageChunk, " continued"), nil)
	require.NoError(t, err)
	require.True(t, continuation[0].Appended)
	_, err = project(makeEvent(3, "first", 1, streams.EventTypeComplete, ""), nil)
	require.NoError(t, err)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, "collision-stream", 3, models.DeliverySubmissionCompleted, time.Time{})
	require.NoError(t, err)
	require.True(t, settled)
	admit("second", 2)
	before, err := repo.GetErrandMessage(ctx, "collision", "shared-id")
	require.NoError(t, err)
	require.Equal(t, "first continued", before.Content)
	var effectsBefore int
	require.NoError(t, repo.db.GetContext(ctx, &effectsBefore, `SELECT COUNT(*) FROM agent_delivery_effects`))
	effect := &models.AgentDeliveryEffect{EffectKey: "collision-second-effect", StreamID: "collision-stream", Sequence: 4, EffectType: "agent_delivery.event", State: models.DeliveryEffectCompleted}
	_, err = project(makeEvent(4, "second", 2, streams.EventTypeMessageChunk, " corrupt"), effect)
	require.Error(t, err)
	after, err := repo.GetErrandMessage(ctx, "collision", "shared-id")
	require.NoError(t, err)
	require.Equal(t, before.Content, after.Content)
	require.Equal(t, "first", after.AuthorID)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, "collision-stream")
	require.NoError(t, err)
	require.EqualValues(t, 3, cursor.ReceivedSequence)
	require.EqualValues(t, 3, cursor.ProjectedSequence)
	var inbox, effectsAfter int
	require.NoError(t, repo.db.GetContext(ctx, &inbox, repo.db.Rebind(`SELECT COUNT(*) FROM agent_delivery_inbox WHERE stream_id=? AND sequence=4`), "collision-stream"))
	require.Zero(t, inbox)
	require.NoError(t, repo.db.GetContext(ctx, &effectsAfter, `SELECT COUNT(*) FROM agent_delivery_effects`))
	require.Equal(t, effectsBefore, effectsAfter)
}
