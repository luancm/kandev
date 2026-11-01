package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func seedErrandLaunchReservation(t *testing.T, f *errandLaunchFixture, req errands.LaunchRequest) *models.Errand {
	t.Helper()
	row := &models.Errand{ID: req.ErrandID, TaskID: req.TaskID, ParentSessionID: req.ParentSessionID,
		Title: req.Title, Objective: req.Objective, AgentProfileID: req.AgentProfileID,
		FirstTurnID: req.TurnID, SubmissionID: req.SubmissionID,
		Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone, AwaitingResult: true}
	require.NoError(t, f.repo.CreateErrand(t.Context(), row))
	return row
}

type errandCreationGateRepo struct {
	*sqliterepo.Repository
	reached chan struct{}
	allow   chan struct{}
	once    sync.Once
}

type errandRegistrationGateRepo struct {
	*sqliterepo.Repository
	reached chan struct{}
	allow   chan struct{}
	gated   atomic.Bool
}

func (r *errandRegistrationGateRepo) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	session, err := r.Repository.GetTaskSession(ctx, id)
	if err == nil && models.IsErrandSession(session) && session.State == models.TaskSessionStateCreated && r.gated.CompareAndSwap(false, true) {
		close(r.reached)
		<-r.allow
	}
	return session, err
}

func TestQueuedErrandCancellationFencesRuntimeRegistration(t *testing.T) {
	f := newErrandLaunchFixture(t)
	req := f.request()
	service, _ := newErrandServiceOnRealStack(t, f)
	repo := &errandRegistrationGateRepo{Repository: f.repo, reached: make(chan struct{}), allow: make(chan struct{})}
	f.svc.repo = repo
	var once sync.Once
	release := func() { once.Do(func() { close(repo.allow) }) }
	done := make(chan struct{})
	var launchErr error
	go func() { defer close(done); _, launchErr = f.svc.LaunchErrandSession(t.Context(), req) }()
	t.Cleanup(func() { release(); coordinatorStopAwaitSignal(t, done, "pre-registration worker cleanup") })
	coordinatorStopAwaitSignal(t, repo.reached, "session created before runtime registration")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err := service.Cancel(ctx, errands.CancelRequest{TaskID: f.taskID, ErrandID: req.ErrandID, Source: models.ErrandCancelSourceUser})
	require.NoError(t, err)
	release()
	coordinatorStopAwaitSignal(t, done, "cancelled runtime registration")
	require.Error(t, launchErr)
	require.Empty(t, f.manager.launchedWith, "a cancelled launch must be rejected before allocating its runtime")
}

func (r *errandCreationGateRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	r.once.Do(func() { close(r.reached); <-r.allow })
	return r.Repository.GetTask(ctx, id)
}

func TestQueuedErrandCancellationWinsBeforeSessionCreation(t *testing.T) {
	f := newErrandLaunchFixture(t)
	req := f.request()
	service, _ := newErrandServiceOnRealStack(t, f)
	repo := &errandCreationGateRepo{Repository: f.repo, reached: make(chan struct{}), allow: make(chan struct{})}
	f.svc.repo = repo
	var once sync.Once
	release := func() { once.Do(func() { close(repo.allow) }) }
	t.Cleanup(release)
	done := make(chan struct{})
	var launchErr error
	go func() { defer close(done); _, launchErr = f.svc.LaunchErrandSession(t.Context(), req) }()
	t.Cleanup(func() { release(); coordinatorStopAwaitSignal(t, done, "creation worker cleanup") })
	coordinatorStopAwaitSignal(t, repo.reached, "launch before session creation")
	_, err := service.Cancel(t.Context(), errands.CancelRequest{TaskID: f.taskID, ErrandID: req.ErrandID, Source: models.ErrandCancelSourceUser})
	require.NoError(t, err)
	release()
	coordinatorStopAwaitSignal(t, done, "cancelled launch creation fence")
	require.Error(t, launchErr)
	sessions, err := f.repo.ListTaskSessions(t.Context(), f.taskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Empty(t, f.manager.launchedWith)
	row, err := f.repo.GetErrand(t.Context(), req.ErrandID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleCancelled, row.Lifecycle)
	require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
}

func TestQueuedErrandCancellationAtRegistrationNeverStartsHarness(t *testing.T) {
	f := newErrandLaunchFixture(t)
	service, _ := newErrandServiceOnRealStack(t, f)
	registered := make(chan *executor.LaunchAgentRequest, 1)
	allow, done := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(allow) }) }
	t.Cleanup(release)
	var runnable atomic.Bool
	register := f.manager.launchAgentFunc
	f.manager.launchAgentFunc = func(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		response, err := register(ctx, req)
		if err != nil {
			return nil, err
		}
		registered <- req
		<-allow
		if err := req.BeforeAgentStart(ctx, response.AgentExecutionID, req.TaskDescription); err != nil {
			return nil, err
		}
		runnable.Store(true)
		return response, nil
	}
	var result *errands.StartResult
	var startErr error
	go func() {
		defer close(done)
		result, startErr = service.Start(t.Context(), errands.StartRequest{TaskID: f.taskID, ParentSessionID: f.parent, Title: "gated", Objective: "gated work"})
	}()
	t.Cleanup(func() { release(); coordinatorStopAwaitSignal(t, done, "registration worker cleanup") })
	var request *executor.LaunchAgentRequest
	select {
	case request = <-registered:
	case <-time.After(10 * time.Second):
		t.Fatal("initial runtime was not registered")
	}
	_, err := service.Cancel(t.Context(), errands.CancelRequest{TaskID: f.taskID, ErrandID: request.SessionID, Source: models.ErrandCancelSourceUser})
	require.NoError(t, err)
	release()
	coordinatorStopAwaitSignal(t, done, "cancelled registered launch")
	require.NoError(t, startErr)
	require.Equal(t, models.ErrandStatusCancelled, result.Status)
	require.False(t, runnable.Load())
	row, err := f.repo.GetErrand(t.Context(), request.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleCancelled, row.Lifecycle)
	require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
}

type errandAcceptancePublisher struct {
	svc      *Service
	check    atomic.Bool
	observed chan error
}

func (p *errandAcceptancePublisher) PublishErrandUpdated(ctx context.Context, row *models.Errand, _ models.ErrandStatus) {
	if !p.check.Load() {
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	guard, err := p.svc.lockCancelInFlightGuardWithContext(probeCtx, row.ID)
	if err == nil {
		guard.release()
	}
	p.observed <- err
}

func TestQueuedErrandCancellationAcceptancePublishesAfterGuardRelease(t *testing.T) {
	f := newErrandLaunchFixture(t)
	adapter := NewErrandAdapter(f.svc)
	publisher := &errandAcceptancePublisher{svc: f.svc, observed: make(chan error, 2)}
	service, err := errands.New(errands.Dependencies{Store: f.repo, Launcher: adapter, Stopper: adapter,
		MCP: adapter, Profiles: errandProfileStub{}, Events: publisher, Logger: testLogger()})
	require.NoError(t, err)
	f.svc.SetErrandController(service)
	startErrand(t, service, f, "initial acceptance")
	f.manager.mu.Lock()
	before, accepted, failed := f.manager.initialPromptAdmissionCallback, f.manager.initialPromptDispatchCallback, f.manager.initialPromptFailureCallback
	f.manager.mu.Unlock()
	require.NotNil(t, before)
	t.Cleanup(failed)
	require.NoError(t, before(t.Context(), "initial payload", nil))
	publisher.check.Store(true)
	accepted()
	require.NoError(t, <-publisher.observed, "publication must allow a synchronous subscriber to reacquire admission")
}

func TestQueuedErrandCancellationRejectsDispatchAndAcceptanceAfterCancel(t *testing.T) {
	f := newErrandLaunchFixture(t)
	service, _ := newErrandServiceOnRealStack(t, f)
	result := startErrand(t, service, f, "cancel before initial dispatch")
	f.manager.mu.Lock()
	before, accepted, failed := f.manager.initialPromptAdmissionCallback, f.manager.initialPromptDispatchCallback, f.manager.initialPromptFailureCallback
	f.manager.mu.Unlock()
	t.Cleanup(failed)
	_, err := service.Cancel(t.Context(), errands.CancelRequest{TaskID: f.taskID, ErrandID: result.Errand.ID, Source: models.ErrandCancelSourceUser})
	require.NoError(t, err)
	row, err := f.repo.GetErrand(t.Context(), result.Errand.ID)
	require.NoError(t, err)
	require.Error(t, before(t.Context(), "initial payload", nil))
	accepted()
	after, err := f.repo.GetErrand(t.Context(), result.Errand.ID)
	require.NoError(t, err)
	require.Equal(t, row, after)
	f.manager.mu.Lock()
	require.Len(t, f.manager.stopAgentWithReasonArgs, 1)
	f.manager.mu.Unlock()
}

func cancelReservedLaunch(t *testing.T, f *errandLaunchFixture, row *models.Errand) {
	t.Helper()
	row.Lifecycle, row.CancelSource = models.ErrandLifecycleCancelled, models.ErrandCancelSourceUser
	row.TeardownIntent = &models.ErrandTeardownIntent{Version: models.ErrandTeardownIntentVersion,
		ID: "reserved-cancel", RequestedAt: time.Now().UTC(), State: models.ErrandTeardownIntentPending,
		Target: models.ErrandTeardownTarget{TaskID: row.TaskID, SessionID: row.ID,
			FirstTurnID: row.FirstTurnID, ReservedSubmissionID: row.SubmissionID}}
	require.NoError(t, f.repo.UpdateErrand(t.Context(), row))
}

func TestQueuedErrandCancellationFencesLateReservedLaunch(t *testing.T) {
	for _, variant := range []string{"missing reservation", "cancelled reservation"} {
		t.Run(variant, func(t *testing.T) {
			f := newErrandLaunchFixture(t)
			req := f.request()
			var row *models.Errand
			if variant != "missing reservation" {
				var err error
				row, err = f.repo.GetErrand(t.Context(), req.ErrandID)
				require.NoError(t, err)
			} else {
				require.NoError(t, f.repo.DeleteErrand(t.Context(), req.ErrandID))
			}
			if variant == "cancelled reservation" {
				cancelReservedLaunch(t, f, row)
			}
			_, err := f.svc.LaunchErrandSession(t.Context(), req)
			require.Error(t, err)
			sessions, err := f.repo.ListTaskSessions(t.Context(), f.taskID)
			require.NoError(t, err)
			require.Len(t, sessions, 1, "a refused reservation cannot create a session")
			require.Empty(t, f.manager.launchedWith)
		})
	}
}
