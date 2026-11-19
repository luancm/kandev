package sqlite

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestOwnedErrandRecoveryAttentionProjectsCurrentTurn(t *testing.T) {
	exerciseOwnedErrandRecoveryAttention(t, newRepoForSessionTests(t))
}

func TestPostgresOwnedErrandRecoveryAttentionProjectsCurrentTurn(t *testing.T) {
	exerciseOwnedErrandRecoveryAttention(t, newPostgresErrandRepo(t))
}

func prepareOwnedErrandRecoveryAttention(t *testing.T, repo *Repository) (*models.Errand, models.ErrandRecoveryObservation) {
	t.Helper()
	ctx := t.Context()
	row := seedOwnedErrandCompletion(t, repo)
	row.ResultState = models.ErrandResultSuperseded
	require.NoError(t, repo.UpdateErrand(ctx, row))
	_, err := repo.TransitionErrandTeardownIntent(ctx, row.ID, row.TeardownIntent.ID, row.Version, models.ErrandTeardownIntentCompleted)
	require.NoError(t, err)
	request := models.ErrandContinuation{
		TaskID: row.TaskID, ErrandID: row.ID, TurnID: "follow-up", SubmissionID: "prompt:errand:next",
		AuthorID: "reader", Content: "Review the output",
	}
	prepared, err := repo.PrepareErrandContinuation(ctx, request)
	require.NoError(t, err)
	bindAcceptedErrandContinuation(t, repo, request, prepared)

	return row, models.ErrandRecoveryObservation{
		TaskID: row.TaskID, TurnID: request.TurnID, SubmissionID: request.SubmissionID,
		PromptGeneration: 1, Unresolved: true,
		Binding: models.ErrandHarnessBinding{
			ErrandID: row.ID, Generation: prepared.Generation, ExecutionID: "next-execution", IncarnationID: "next-incarnation",
			HarnessGeneration: 1, StreamID: "next-stream", NativeSessionID: prepared.NativeID,
		},
	}
}

func exerciseOwnedErrandRecoveryAttention(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	row, observation := prepareOwnedErrandRecoveryAttention(t, repo)
	before, err := repo.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	stale := observation
	stale.Binding.Generation--
	changed, err := repo.RecordErrandRecoveryObservation(ctx, stale)
	require.NoError(t, err)
	require.False(t, changed)
	changed, err = repo.RecordErrandRecoveryObservation(ctx, observation)
	require.NoError(t, err)
	require.True(t, changed)
	changed, err = repo.RecordErrandRecoveryObservation(ctx, observation)
	require.NoError(t, err)
	require.False(t, changed)

	statuses, err := repo.ReadErrandStatuses(ctx, row.TaskID, row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandStatusNeedsAttention, statuses[row.ID].Status)
	require.Equal(t, models.ErrandAttentionRecoveryUnresolved, statuses[row.ID].AttentionReason)

	summary, err := repo.GetErrandBrowserSummary(ctx, row.TaskID)
	require.NoError(t, err)
	require.Len(t, summary.Errands, 1)
	require.Equal(t, row.ID, summary.Errands[0].ID)
	require.Equal(t, 1, summary.Counts.Live)
	require.Equal(t, 1, summary.Counts.Outstanding)
	batch, err := repo.BatchErrandsSummariesByTaskIDs(ctx, []string{row.TaskID})
	require.NoError(t, err)
	require.Equal(t, summary.Counts.Live, batch[row.TaskID].Live)
	after, err := repo.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, before.ResultRevision, after.ResultRevision)
	require.Equal(t, before.ResultState, after.ResultState)
	require.Equal(t, before.ResultText, after.ResultText)
	hasErrands, outstanding, err := repo.GetParentErrandState(ctx, row.ParentSessionID)
	require.NoError(t, err)
	require.True(t, hasErrands)
	require.True(t, outstanding)
	attention, err := repo.ErrandRecoveryAttention(ctx, row.ID)
	require.NoError(t, err)
	require.True(t, attention)

	stale = observation
	stale.Binding.StreamID = "obsolete-stream"
	stale.Unresolved = false
	changed, err = repo.RecordErrandRecoveryObservation(ctx, stale)
	require.NoError(t, err)
	require.False(t, changed)
	observation.Unresolved = false
	changed, err = repo.RecordErrandRecoveryObservation(ctx, observation)
	require.NoError(t, err)
	require.True(t, changed)
	statuses, err = repo.ReadErrandStatuses(ctx, row.TaskID, row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandStatusRunning, statuses[row.ID].Status)
	require.Empty(t, statuses[row.ID].AttentionReason)
	_, outstanding, err = repo.GetParentErrandState(ctx, row.ParentSessionID)
	require.NoError(t, err)
	require.False(t, outstanding)

	observation.Unresolved = true
	changed, err = repo.RecordErrandRecoveryObservation(ctx, observation)
	require.NoError(t, err)
	require.True(t, changed)
	successor := observation.Binding
	successor.StreamID = "successor-stream"
	successor.HarnessGeneration++
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &successor, 1))
	attention, err = repo.ErrandRecoveryAttention(ctx, row.ID)
	require.NoError(t, err)
	require.False(t, attention, "evidence from the former binding must not classify the successor")

	stale = observation
	stale.Binding = successor
	changed, err = repo.RecordErrandRecoveryObservation(ctx, stale)
	require.NoError(t, err)
	require.False(t, changed, "a current binding cannot report recovery for a prompt admitted on another stream")
}

func TestOwnedErrandTerminalClearsRecoveryAttention(t *testing.T) {
	exerciseOwnedErrandTerminalClearsRecoveryAttention(t, newRepoForSessionTests(t))
}

func TestPostgresOwnedErrandTerminalClearsRecoveryAttention(t *testing.T) {
	exerciseOwnedErrandTerminalClearsRecoveryAttention(t, newPostgresErrandRepo(t))
}

func exerciseOwnedErrandTerminalClearsRecoveryAttention(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	row, observation := prepareOwnedErrandRecoveryAttention(t, repo)
	changed, err := repo.RecordErrandRecoveryObservation(ctx, observation)
	require.NoError(t, err)
	require.True(t, changed)
	binding := observation.Binding
	payload, err := json.Marshal(streams.AgentEvent{
		Type: streams.EventTypeComplete, SessionID: binding.NativeSessionID,
		TurnID: observation.TurnID, PromptGeneration: observation.PromptGeneration,
	})
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{
		SessionID: models.ErrandExecutionIdentity(row.ID), TaskID: row.TaskID,
		ExecutionID: binding.ExecutionID, IncarnationID: binding.IncarnationID,
		HarnessGeneration: binding.HarnessGeneration, OwnerGeneration: binding.HarnessGeneration,
		StreamID: binding.StreamID, ExpectedStreamID: binding.StreamID, Sequence: 1,
		SubmissionID: observation.SubmissionID, EventType: streams.EventTypeComplete,
		Payload: payload, Terminal: true,
	}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	_, err = repo.SettleAgentDeliveryTerminal(ctx, binding.StreamID, 1, models.DeliverySubmissionCompleted, errandScenarioBase)
	require.NoError(t, err)
	attention, err := repo.ErrandRecoveryAttention(ctx, row.ID)
	require.NoError(t, err)
	require.False(t, attention)
}
