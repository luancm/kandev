package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type errandObservationCounter struct {
	*sqliterepo.Repository
	statusReads atomic.Int32
}

func (r *errandObservationCounter) ReadErrandStatuses(ctx context.Context, taskID, errandID string) (map[string]models.ErrandBrowserObservation, error) {
	r.statusReads.Add(1)
	return r.Repository.ReadErrandStatuses(ctx, taskID, errandID)
}

func TestOrdinarySessionRecoveryDoesNotNotifyOrWakeErrandAwait(t *testing.T) {
	ctx := t.Context()
	fixture := newErrandLaunchFixture(t)
	repo := &errandObservationCounter{Repository: fixture.repo}
	fixture.svc.repo = repo
	parent, err := repo.GetTaskSession(ctx, "parent-1")
	require.NoError(t, err)
	row := &models.Errand{
		ID: "owned-errand", TaskID: parent.TaskID, ParentSessionID: parent.ID,
		Title: "Observe recovery", AgentProfileID: "profile", FirstTurnID: "owned-turn",
		SubmissionID: "prompt:owned", Lifecycle: models.ErrandLifecycleLaunched,
		AwaitingResult: true, ResultState: models.ErrandResultNone,
	}
	require.NoError(t, repo.CreateErrand(ctx, row))
	_, err = repo.ReserveErrandExecutionGeneration(ctx, row.ID, 0)
	require.NoError(t, err)

	errandEvents := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(errandEvents.Close)
	var updates atomic.Int32
	_, err = errandEvents.Subscribe(events.ErrandUpdated, func(context.Context, *bus.Event) error {
		updates.Add(1)
		return nil
	})
	require.NoError(t, err)
	adapter := NewErrandAdapter(fixture.svc)
	errandService, err := errands.New(errands.Dependencies{
		Store: repo, Launcher: adapter, Stopper: adapter, MCP: adapter,
		Profiles: errandProfileStub{}, Logger: testLogger(),
		Events: errands.NewBusPublisher(errandEvents, func(context.Context, string) (string, error) {
			return "ws1", nil
		}, testLogger()),
	})
	require.NoError(t, err)
	fixture.svc.SetErrandController(errandService)

	incoming := seedOrdinaryRecoveryOwner(t, repo.Repository, parent.TaskID)
	awaitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, awaitErr := errandService.Await(awaitCtx, errands.AwaitRequest{
			Owner: models.ErrandAwaitIdentity{
				TaskID: parent.TaskID, ParentSessionID: parent.ID, ParentIncarnationID: parent.QueueIncarnationID,
				HarnessGeneration: 1, Token: "ordinary-recovery-control",
			},
			ErrandIDs: []string{row.ID}, Mode: "all", MaxWaitSeconds: 600,
		})
		done <- awaitErr
	}()
	require.Eventually(t, func() bool { return repo.statusReads.Load() > 0 }, 5*time.Second, time.Millisecond)
	readsBefore := repo.statusReads.Load()

	fixture.svc.handleAgentctlDeliveryRecovery(ctx, incoming)
	ordinary, err := repo.GetTaskSession(ctx, incoming.SessionID)
	require.NoError(t, err)
	recovery, found := models.LoadAgentDeliveryRecovery(ordinary.Metadata)
	require.True(t, found)
	require.Equal(t, models.AgentDeliveryRecoveryUncertain, recovery.Phase)
	require.Never(t, func() bool {
		return repo.statusReads.Load() != readsBefore || updates.Load() != 0
	}, 50*time.Millisecond, time.Millisecond)

	cancel()
	select {
	case awaitErr := <-done:
		require.ErrorIs(t, awaitErr, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("parent await did not stop")
	}
}

func seedOrdinaryRecoveryOwner(t *testing.T, repo *sqliterepo.Repository, taskID string) lifecycle.AgentctlEventPayload {
	t.Helper()
	const (
		sessionID    = "ordinary-recovery-session"
		executionID  = "ordinary-recovery-execution"
		submissionID = "ordinary-recovery-submission"
		streamID     = "ordinary-recovery-stream"
	)
	ctx := t.Context()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	}))
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: sessionID, IncarnationID: session.QueueIncarnationID, Generation: 3,
		NativeSessionID: "ordinary-recovery-native", CreationReason: "test",
	}))
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: sessionID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 3, OwnerGeneration: 1, PayloadHash: "ordinary-recovery-hash",
		Payload: []byte("ordinary prompt"), State: models.DeliverySubmissionInterruptedUnknown,
	})
	require.NoError(t, err)
	return lifecycle.AgentctlEventPayload{
		TaskID: taskID, SessionID: sessionID, AgentExecutionID: executionID,
		DeliveryRecoveryPhase: models.AgentDeliveryRecoveryUncertain,
		DeliverySubmissionID:  submissionID, DeliveryStreamID: streamID,
		DeliveryIncarnationID: session.QueueIncarnationID, DeliveryHarnessGeneration: 3,
		PromptGeneration: 10, ErrorMessage: "ordinary session recovery is uncertain",
	}
}
