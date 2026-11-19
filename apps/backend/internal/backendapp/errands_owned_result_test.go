package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	sqlitetaskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestOwnedErrandTerminalProducesParentResultWithoutChildSession(t *testing.T) {
	ctx := t.Context()
	_, _, repo := openErrandTranscriptRepo(t, filepath.Join(t.TempDir(), "owned-errand.db"))
	parent := seedErrandTranscriptSession(t, repo)
	env := bus.NewMemoryEventBus(logger.Default())
	t.Cleanup(env.Close)
	adapter := orchestrator.NewErrandAdapter(nil)
	stopper := &ownedErrandResultStopper{err: errors.New("stop remains unconfirmed")}
	svc, err := errands.New(errands.Dependencies{
		Store: repo, Launcher: adapter, Stopper: stopper, MCP: adapter,
		Profiles: errands.NewProfileResolver(errandProfileCatalog{}),
		Events:   errands.NewBusPublisher(env, func(context.Context, string) (string, error) { return "errand-transcript-workspace", nil }, logger.Default()),
	})
	require.NoError(t, err)
	svc.SetTurnReader(errandTranscript{repo: repo})
	row := seedOwnedErrandTerminal(t, repo, parent)
	status, err := svc.Project(ctx, row)
	require.NoError(t, err)
	require.Equal(t, models.ErrandStatusRunning, status, "result is still awaiting settlement")

	require.ErrorIs(t, svc.SweepDelivery(ctx), stopper.err)
	settled, err := repo.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultPending, settled.ResultState)
	require.Equal(t, "owned answer", settled.ResultText)
	require.Equal(t, row.FirstTurnID, settled.ResultTurnID)
	require.Equal(t, models.ErrandOutcomeDone, settled.LastOutcome)
	require.NotNil(t, settled.TeardownIntent)
	require.Equal(t, row.ID, settled.TeardownIntent.Target.ErrandID)
	require.Equal(t, "execution", settled.TeardownIntent.Target.ExecutionID)
	require.Equal(t, models.ErrandTeardownIntentPending, settled.TeardownIntent.State)
	require.Equal(t, settled.TeardownIntent.ID, stopper.observed.ID)
	composition, err := svc.ComposeParentTurn(ctx, parent.ID, models.ErrandDeliveryTurnContext)
	require.NoError(t, err)
	require.NotEmpty(t, composition.Token)
	require.Contains(t, composition.Block, "owned answer")
	require.Len(t, composition.Items, 1)
	require.Equal(t, row.ID, composition.Items[0].ErrandID)
	delivered, err := repo.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultDelivered, delivered.ResultState)
	require.Equal(t, models.ErrandTeardownIntentPending, delivered.TeardownIntent.State)
	sessions, err := repo.ListTaskSessions(ctx, parent.TaskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, parent.ID, sessions[0].ID)
}

type ownedErrandResultStopper struct {
	errands.ErrandTeardown
	observed errands.ErrandTeardownIntent
	err      error
}

func (s *ownedErrandResultStopper) RetryErrandTeardown(_ context.Context, intent errands.ErrandTeardownIntent) (errands.TeardownOutcome, error) {
	s.observed = intent
	return "", s.err
}

func seedOwnedErrandTerminal(t *testing.T, repo *sqlitetaskrepo.Repository, parent *models.TaskSession) *models.Errand {
	t.Helper()
	ctx := t.Context()
	row := &models.Errand{
		ID: "owned", TaskID: parent.TaskID, ParentSessionID: parent.ID, Title: "Audit", Objective: "Inspect",
		AgentProfileID: "profile", FirstTurnID: "owned-turn", SubmissionID: "prompt:owned",
		Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone, AwaitingResult: true,
	}
	require.NoError(t, repo.CreateErrand(ctx, row))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, row.ID, 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, row.ID, 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	binding := &models.ErrandHarnessBinding{
		ErrandID: row.ID, ExecutionID: "execution", Generation: 1, StreamID: "stream",
		IncarnationID: "incarnation", HarnessGeneration: 1,
	}
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	require.NoError(t, repo.CreateErrandTurn(ctx, row.ID, &models.Turn{ID: row.FirstTurnID, TaskID: row.TaskID}))
	payload := []byte(`{"text":"Inspect"}`)
	submission := &models.AgentDeliverySubmission{
		ID: row.SubmissionID, Payload: payload, PayloadHash: journal.SubmissionHash(payload),
		DispatchAttemptID: binding.ExecutionID, State: models.DeliverySubmissionDispatching,
	}
	admitted, err := repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{
		ErrandID: row.ID, Generation: 1, ExecutionID: binding.ExecutionID, IncarnationID: binding.IncarnationID,
		HarnessGeneration: 1, StreamID: binding.StreamID, PromptGeneration: 1, TurnID: row.FirstTurnID,
		Submission: submission,
	})
	require.NoError(t, err)
	require.True(t, admitted)
	row.Lifecycle = models.ErrandLifecycleLaunched
	require.NoError(t, repo.UpdateErrand(ctx, row))
	projectOwnedErrandEvent(t, repo, row, binding, 1, streams.AgentEvent{
		Type: streams.EventTypeMessageChunk, Text: "owned answer", CanonicalMessageID: "message",
		CanonicalMessageBaseID: "message",
	})
	message, err := repo.GetErrandLastAssistantMessage(ctx, row.ID, row.FirstTurnID)
	require.NoError(t, err)
	require.Equal(t, "owned answer", message.Content)
	projectOwnedErrandEvent(t, repo, row, binding, 2, streams.AgentEvent{Type: streams.EventTypeComplete})
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, binding.StreamID, 2, models.DeliverySubmissionCompleted, time.Time{})
	require.NoError(t, err)
	require.True(t, settled)
	return row
}

func projectOwnedErrandEvent(t *testing.T, repo *sqlitetaskrepo.Repository, row *models.Errand, binding *models.ErrandHarnessBinding, sequence int64, event streams.AgentEvent) {
	t.Helper()
	ctx := t.Context()
	event.TurnID, event.PromptGeneration = row.FirstTurnID, 1
	payload, err := json.Marshal(event)
	require.NoError(t, err)
	incoming := &models.AgentDeliveryEvent{
		SessionID: models.ErrandExecutionIdentity(row.ID), TaskID: row.TaskID, ExecutionID: binding.ExecutionID,
		IncarnationID: binding.IncarnationID, HarnessGeneration: 1, OwnerGeneration: 1,
		ExpectedStreamID: binding.StreamID, StreamID: binding.StreamID, Sequence: sequence, SubmissionID: row.SubmissionID,
		EventType: event.Type, Payload: payload, Terminal: event.Type == streams.EventTypeComplete,
	}
	if event.Type == streams.EventTypeMessageChunk {
		projected, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, []*models.AgentDeliveryEvent{incoming}, []*models.AgentDeliveryEffect{nil}, sequence)
		require.NoError(t, err)
		require.Len(t, projected, 1)
		require.False(t, projected[0].AlreadyProjected)
		return
	}
	received, err := repo.ReceiveAgentDeliveryEvent(ctx, incoming, sequence)
	require.NoError(t, err)
	require.True(t, received)
	projected, err := repo.ProjectAgentDeliveryEvent(ctx, incoming, nil)
	require.NoError(t, err)
	require.True(t, projected)
}
