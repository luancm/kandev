package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type recoveryAttentionTestRepo struct {
	*sqliterepo.Repository
	attentionReads     chan struct{}
	attentionCount     atomic.Int32
	sessionReads       atomic.Int32
	failSessionReadAt  atomic.Int32
	recoveryWriteError error
}

func (r *recoveryAttentionTestRepo) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	read := r.sessionReads.Add(1)
	if failure := r.failSessionReadAt.Load(); failure != 0 && read == failure {
		return nil, errors.New("injected recovery session reload failure")
	}
	return r.Repository.GetTaskSession(ctx, id)
}

func (r *recoveryAttentionTestRepo) ErrandRecoveryAttention(ctx context.Context, id string) (bool, error) {
	unresolved, err := r.Repository.ErrandRecoveryAttention(ctx, id)
	if err == nil {
		r.attentionCount.Add(1)
		select {
		case r.attentionReads <- struct{}{}:
		default:
		}
	}
	return unresolved, err
}

func (r *recoveryAttentionTestRepo) UpsertAgentDeliveryRecovery(
	ctx context.Context,
	recovery *models.AgentDeliveryRecovery,
	block *models.SessionRecoveryBlock,
) (bool, error) {
	if r.recoveryWriteError != nil {
		return false, r.recoveryWriteError
	}
	return r.Repository.UpsertAgentDeliveryRecovery(ctx, recovery, block)
}

type recoveryAttentionEventFixture struct {
	service       *Service
	errands       *errands.Service
	repo          *recoveryAttentionTestRepo
	updated       <-chan dto.ErrandUpdatedEvent
	payload       lifecycle.AgentctlEventPayload
	errandsID     string
	parentSession string
}

func newRecoveryAttentionEventFixture(t *testing.T) *recoveryAttentionEventFixture {
	t.Helper()
	launch := newErrandLaunchFixture(t)
	store := &recoveryAttentionTestRepo{
		Repository: launch.repo, attentionReads: make(chan struct{}, 16),
	}
	launch.svc.repo = store
	const parentSession = "parent-1"
	payload := seedRecoveryAttentionErrand(t, launch.repo, launch.taskID, parentSession)

	errandsBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(errandsBus.Close)
	updated := make(chan dto.ErrandUpdatedEvent, 8)
	_, err := errandsBus.Subscribe(events.ErrandUpdated, func(_ context.Context, event *bus.Event) error {
		if payload, ok := event.Data.(dto.ErrandUpdatedEvent); ok {
			updated <- payload
		}
		return nil
	})
	require.NoError(t, err)
	adapter := NewErrandAdapter(launch.svc)
	errandsService, err := errands.New(errands.Dependencies{
		Store: store, Launcher: adapter, Stopper: adapter, MCP: adapter,
		Profiles: errandProfileStub{},
		Events: errands.NewBusPublisher(errandsBus, func(context.Context, string) (string, error) {
			return "ws1", nil
		}, testLogger()),
		Logger: testLogger(),
	})
	require.NoError(t, err)
	launch.svc.SetErrandController(errandsService)
	launch.svc.eventBus = nil

	return &recoveryAttentionEventFixture{
		service: launch.svc, errands: errandsService, repo: store, updated: updated,
		errandsID: payload.SessionID, parentSession: parentSession, payload: payload,
	}
}

func seedRecoveryAttentionErrand(
	t *testing.T,
	repo *sqliterepo.Repository,
	taskID, parentSessionID string,
) lifecycle.AgentctlEventPayload {
	t.Helper()
	const (
		errandsID    = "errand-recovery-observer"
		turnID       = "turn-recovery-observer"
		submissionID = "submission-recovery-observer"
		executionID  = "execution-recovery-observer"
		streamID     = "stream-recovery-observer"
		harnessGen   = int64(3)
		promptGen    = uint64(9)
	)
	ctx := t.Context()
	session := &models.TaskSession{
		ID: errandsID, TaskID: taskID, ParentSessionID: parentSessionID,
		Role: models.SessionRoleErrand, State: models.TaskSessionStateWaitingForInput,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, session))
	session, err := repo.GetTaskSession(ctx, errandsID)
	require.NoError(t, err)
	seedExecutorRunning(t, repo, errandsID, taskID, executionID)
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: errandsID, IncarnationID: session.QueueIncarnationID, Generation: harnessGen,
		NativeSessionID: "native-recovery-observer", CreationReason: "test",
	}))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: turnID, TaskID: taskID, TaskSessionID: errandsID}))
	require.NoError(t, repo.CompleteTurn(ctx, turnID))
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{
		ID: errandsID, TaskID: taskID, ParentSessionID: parentSessionID,
		Title: "Recovery observer", Objective: "Observe recovery", AgentProfileID: "profile-1",
		FirstTurnID: turnID, SubmissionID: submissionID, Lifecycle: models.ErrandLifecycleLaunched,
		ResultState: models.ErrandResultNone,
	}))
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: errandsID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: harnessGen, OwnerGeneration: 1, PayloadHash: "hash-recovery-observer",
		Payload: []byte("original prompt"), State: models.DeliverySubmissionInterruptedUnknown,
	})
	require.NoError(t, err)
	recovery := models.AgentDeliveryRecovery{
		Phase: models.AgentDeliveryRecoveryReconnecting, Revision: 1, SessionID: errandsID,
		AgentExecutionID: executionID, SubmissionID: submissionID, StreamID: streamID,
		IncarnationID: session.QueueIncarnationID, HarnessGeneration: harnessGen, PromptGeneration: promptGen,
	}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, errandsID, models.SessionMetaKeyAgentDeliveryRecovery, recovery))
	return lifecycle.AgentctlEventPayload{
		TaskID: taskID, SessionID: errandsID, AgentExecutionID: executionID,
		DeliveryRecoveryPhase: models.AgentDeliveryRecoveryUncertain,
		DeliverySubmissionID:  submissionID, DeliveryStreamID: streamID,
		DeliveryIncarnationID: session.QueueIncarnationID, DeliveryHarnessGeneration: uint64(harnessGen),
		PromptGeneration: promptGen, ErrorMessage: "the original prompt outcome is uncertain",
	}
}

func TestDeliveryRecoveryPhaseChangePublishesAttentionAndWakesAwait(t *testing.T) {
	fixture := newRecoveryAttentionEventFixture(t)
	await := startPendingErrandAwait(t, fixture)

	fixture.service.handleAgentctlDeliveryRecovery(t.Context(), fixture.payload)
	select {
	case event := <-fixture.updated:
		require.Equal(t, fixture.errandsID, event.ErrandID)
		require.Equal(t, models.ErrandStatusNeedsAttention, event.Status)
		require.Equal(t, models.ErrandAttentionRecoveryUnresolved, event.AttentionReason)
	case <-time.After(5 * time.Second):
		t.Fatal("same-state recovery change did not publish an Errand attention event")
	}

	require.Eventually(t, func() bool {
		return fixture.repo.attentionCount.Load() >= 3
	}, 5*time.Second, time.Millisecond, "the waiting parent join re-read the Errand projection after the recovery event")
	session, err := fixture.repo.GetTaskSession(t.Context(), fixture.errandsID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State,
		"the recovery phase change publishes attention without changing coarse session state")

	await.stop(t)
}

func TestRecoveryPublicationFailureDoesNotNotifyOrWakeErrandAwait(t *testing.T) {
	for _, tc := range []struct {
		name          string
		failPersist   bool
		failSessionAt bool
		wantPhase     string
	}{
		{name: "persistence failure", failPersist: true, wantPhase: models.AgentDeliveryRecoveryReconnecting},
		{name: "session reload failure", failSessionAt: true, wantPhase: models.AgentDeliveryRecoveryUncertain},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newRecoveryAttentionEventFixture(t)
			await := startPendingErrandAwait(t, fixture)
			readsBefore := fixture.repo.attentionCount.Load()
			if tc.failPersist {
				fixture.repo.recoveryWriteError = errors.New("injected recovery persistence failure")
			}
			if tc.failSessionAt {
				fixture.repo.failSessionReadAt.Store(fixture.repo.sessionReads.Load() + 2)
			}

			fixture.service.handleAgentctlDeliveryRecovery(t.Context(), fixture.payload)

			require.Equal(t, readsBefore, fixture.repo.attentionCount.Load(),
				"a failed persistence or post-write session reload does not publish to or wake Errand observers")
			requireNoRecoveryAttentionEvent(t, fixture)
			session, err := fixture.repo.Repository.GetTaskSession(t.Context(), fixture.errandsID)
			require.NoError(t, err)
			recovery, ok := models.LoadAgentDeliveryRecovery(session.Metadata)
			require.True(t, ok)
			require.Equal(t, tc.wantPhase, recovery.Phase)
			await.stop(t)
		})
	}
}

func TestOrdinarySessionRecoveryDoesNotNotifyOrWakeErrandAwait(t *testing.T) {
	fixture := newRecoveryAttentionEventFixture(t)
	await := startPendingErrandAwait(t, fixture)
	readsBefore := fixture.repo.attentionCount.Load()
	ordinaryPayload := seedOrdinaryRecoveryOwner(t, fixture)

	fixture.service.handleAgentctlDeliveryRecovery(t.Context(), ordinaryPayload)

	require.Equal(t, readsBefore, fixture.repo.attentionCount.Load(),
		"a recovery change for an ordinary session does not reproject the unrelated Errand")
	requireNoRecoveryAttentionEvent(t, fixture)
	await.stop(t)
}

func TestRejectedStaleRecoveryDoesNotNotifyOrWakeErrandAwait(t *testing.T) {
	fixture := newRecoveryAttentionEventFixture(t)
	await := startPendingErrandAwait(t, fixture)
	readsBefore := fixture.repo.attentionCount.Load()
	stale := fixture.payload
	stale.AgentExecutionID = "replaced-execution"

	fixture.service.handleAgentctlDeliveryRecovery(t.Context(), stale)

	require.Equal(t, readsBefore, fixture.repo.attentionCount.Load(),
		"a rejected recovery event does not reproject or wake the Errand join")
	requireNoRecoveryAttentionEvent(t, fixture)
	session, err := fixture.repo.Repository.GetTaskSession(t.Context(), fixture.errandsID)
	require.NoError(t, err)
	recovery, ok := models.LoadAgentDeliveryRecovery(session.Metadata)
	require.True(t, ok)
	require.Equal(t, models.AgentDeliveryRecoveryReconnecting, recovery.Phase)
	await.stop(t)
}

func TestClearedRecoveryNoticePublishesAttentionAndWakesAwait(t *testing.T) {
	fixture := newRecoveryAttentionEventFixture(t)
	await := startPendingErrandAwait(t, fixture)
	session, err := fixture.repo.Repository.GetTaskSession(t.Context(), fixture.errandsID)
	require.NoError(t, err)

	require.NoError(t, fixture.service.clearAgentDeliveryRecoveryNotice(
		t.Context(), fixture.errandsID, session.QueueIncarnationID,
		fixture.payload.DeliverySubmissionID, int64(fixture.payload.DeliveryHarnessGeneration),
	))
	select {
	case event := <-fixture.updated:
		require.Equal(t, models.ErrandStatusNeedsAttention, event.Status)
		require.Equal(t, models.ErrandAttentionRecoveryUnresolved, event.AttentionReason)
	case <-time.After(5 * time.Second):
		t.Fatal("persisted recovery clear did not publish the current Errand attention snapshot")
	}
	require.Eventually(t, func() bool {
		return fixture.repo.attentionCount.Load() >= 3
	}, 5*time.Second, time.Millisecond, "the waiting parent join re-read after the recovery clear")
	await.stop(t)
}

func TestNoOpRecoveryClearDoesNotNotifyOrWakeErrandAwait(t *testing.T) {
	fixture := newRecoveryAttentionEventFixture(t)
	await := startPendingErrandAwait(t, fixture)
	readsBefore := fixture.repo.attentionCount.Load()
	session, err := fixture.repo.Repository.GetTaskSession(t.Context(), fixture.errandsID)
	require.NoError(t, err)

	require.NoError(t, fixture.service.clearAgentDeliveryRecoveryNotice(
		t.Context(), fixture.errandsID, session.QueueIncarnationID,
		"stale-recovery-submission", int64(fixture.payload.DeliveryHarnessGeneration),
	))

	require.Equal(t, readsBefore, fixture.repo.attentionCount.Load(),
		"a clear that did not persist must not publish or wake Errand observers")
	requireNoRecoveryAttentionEvent(t, fixture)
	await.stop(t)
}

type pendingErrandAwait struct {
	cancel   context.CancelFunc
	done     chan error
	finished chan struct{}
}

func startPendingErrandAwait(t *testing.T, fixture *recoveryAttentionEventFixture) *pendingErrandAwait {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	await := &pendingErrandAwait{cancel: cancel, done: make(chan error, 1), finished: make(chan struct{})}
	go func() {
		_, err := fixture.errands.Await(ctx, errands.AwaitRequest{
			ParentSessionID: fixture.parentSession, ErrandIDs: []string{fixture.errandsID},
			Mode: "all", MaxWaitSeconds: 600, Token: "recovery-observer-await",
		})
		await.done <- err
		close(await.finished)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-await.finished:
		case <-time.After(5 * time.Second):
			t.Error("parent await did not stop during cleanup")
		}
	})
	select {
	case <-fixture.repo.attentionReads:
	case <-time.After(5 * time.Second):
		t.Fatal("parent await did not read the initial Errand projection")
	}
	return await
}

func (w *pendingErrandAwait) stop(t *testing.T) {
	t.Helper()
	w.cancel()
	select {
	case <-w.finished:
		require.ErrorIs(t, <-w.done, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("parent await did not stop after its context was cancelled")
	}
}

func requireNoRecoveryAttentionEvent(t *testing.T, fixture *recoveryAttentionEventFixture) {
	t.Helper()
	select {
	case event := <-fixture.updated:
		t.Fatalf("unexpected Errand event: %+v", event)
	default:
	}
}

func seedOrdinaryRecoveryOwner(t *testing.T, fixture *recoveryAttentionEventFixture) lifecycle.AgentctlEventPayload {
	t.Helper()
	const (
		sessionID    = "ordinary-recovery-session"
		executionID  = "ordinary-recovery-execution"
		submissionID = "ordinary-recovery-submission"
		streamID     = "ordinary-recovery-stream"
	)
	ctx := t.Context()
	session := &models.TaskSession{ID: sessionID, TaskID: fixture.payload.TaskID, State: models.TaskSessionStateWaitingForInput}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, session))
	session, err := fixture.repo.Repository.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	seedExecutorRunning(t, fixture.repo.Repository, sessionID, fixture.payload.TaskID, executionID)
	require.NoError(t, fixture.repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: sessionID, IncarnationID: session.QueueIncarnationID, Generation: 3,
		NativeSessionID: "ordinary-recovery-native", CreationReason: "test",
	}))
	_, err = fixture.repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: sessionID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 3, OwnerGeneration: 1, PayloadHash: "ordinary-recovery-hash",
		Payload: []byte("ordinary prompt"), State: models.DeliverySubmissionInterruptedUnknown,
	})
	require.NoError(t, err)
	return lifecycle.AgentctlEventPayload{
		TaskID: fixture.payload.TaskID, SessionID: sessionID, AgentExecutionID: executionID,
		DeliveryRecoveryPhase: models.AgentDeliveryRecoveryUncertain,
		DeliverySubmissionID:  submissionID, DeliveryStreamID: streamID,
		DeliveryIncarnationID: session.QueueIncarnationID, DeliveryHarnessGeneration: 3,
		PromptGeneration: 10, ErrorMessage: "ordinary session recovery is uncertain",
	}
}
