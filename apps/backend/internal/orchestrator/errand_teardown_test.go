package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type errandTargetCanceller interface {
	CancelErrandWithTarget(context.Context, *models.Errand, func(models.ErrandTeardownTarget) error) error
}

type teardownProvider struct {
	*mockAgentManager
	mu         sync.Mutex
	current    string
	runtimes   map[string]*agentruntime.AgentExecution
	stops      []string
	lookupErr  error
	promptErr  error
	stopErr    error
	beforeStop func(context.Context, string) error
}

func (m *teardownProvider) GetExecution(id string) (*agentruntime.AgentExecution, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	execution, ok := m.runtimes[id]
	return execution, ok
}

type errandTeardownFixture struct {
	svc      *Service
	repo     *sqliterepo.Repository
	provider *teardownProvider
	row      *models.Errand
}

func newErrandTeardownFixture(t *testing.T) *errandTeardownFixture {
	t.Helper()
	f := newCancelTurnFixture(t)
	seedOrdinarySession(t, f.repo, "t1", "parent", models.TaskSessionStateWaitingForInput, -time.Minute)
	markSessionErrand(t, f.repo, "s1", "parent")
	session, err := f.repo.GetTaskSession(t.Context(), "s1")
	require.NoError(t, err)
	provider := &teardownProvider{mockAgentManager: f.agentMgr, current: "exec-1", runtimes: map[string]*agentruntime.AgentExecution{
		"exec-1": {ID: "exec-1", TaskID: "t1", SessionID: "s1", DeliveryIncarnationID: session.QueueIncarnationID},
	}}
	f.agentMgr.getExecutionIDForSessionFunc = func(_ context.Context, sessionID string) (string, error) {
		provider.mu.Lock()
		defer provider.mu.Unlock()
		if provider.lookupErr != nil {
			return "", provider.lookupErr
		}
		if execution := provider.runtimes[provider.current]; execution == nil || execution.SessionID != sessionID {
			return "", agentruntime.ErrNoExecutionForSession
		}
		return provider.current, nil
	}
	f.agentMgr.getPromptActivityForSessionFunc = func(sessionID string) (string, uint64, uint64, time.Time, error) {
		provider.mu.Lock()
		defer provider.mu.Unlock()
		if provider.promptErr != nil {
			return "", 0, 0, time.Time{}, provider.promptErr
		}
		if execution := provider.runtimes[provider.current]; execution == nil || execution.SessionID != sessionID {
			return "", 0, 0, time.Time{}, agentruntime.ErrNoExecutionForSession
		}
		return provider.current, f.agentMgr.currentPromptGeneration.Load(), 0, time.Time{}, nil
	}
	f.agentMgr.stopAgentWithReasonFunc = func(ctx context.Context, id, _ string, _ bool) error {
		if provider.beforeStop != nil {
			if err := provider.beforeStop(ctx, id); err != nil {
				return err
			}
		}
		provider.mu.Lock()
		defer provider.mu.Unlock()
		provider.stops = append(provider.stops, id)
		if provider.stopErr != nil {
			return provider.stopErr
		}
		delete(provider.runtimes, id)
		if provider.current == id {
			provider.current = ""
		}
		return nil
	}
	svc := newCoordinatorStopTestService(f.repo, newMockTaskRepo(), provider)
	svc.SetTurnService(taskservice.NewService(taskservice.Repos{
		Tasks: f.repo, TaskRepos: f.repo, Workspaces: f.repo, Workflows: f.repo, Messages: f.repo,
		Turns: f.repo, Sessions: f.repo, Executors: f.repo, TaskEnvironments: f.repo,
	}, nil, testLogger(), taskservice.RepositoryDiscoveryConfig{}))
	row := &models.Errand{ID: "s1", TaskID: "t1", ParentSessionID: "parent", Title: "Exact teardown",
		Objective: "Stop the captured execution", AgentProfileID: "profile", FirstTurnID: "turn-1",
		SubmissionID: "prompt:initial", Lifecycle: models.ErrandLifecycleLaunched, ResultState: models.ErrandResultNone}
	require.NoError(t, f.repo.CreateErrand(t.Context(), row))
	seedExecutorRunning(t, f.repo, row.ID, row.TaskID, "exec-1")
	return &errandTeardownFixture{svc: svc, repo: f.repo, provider: provider, row: row}
}

func (f *errandTeardownFixture) pending(t *testing.T) models.ErrandTeardownIntent {
	t.Helper()
	fault := errors.New("interrupted after pending commit")
	err := NewErrandAdapter(f.svc).CancelErrandWithTarget(t.Context(), f.row, func(target models.ErrandTeardownTarget) error {
		if err := f.commit(t.Context(), target); err != nil {
			return err
		}
		return fault
	})
	require.ErrorIs(t, err, fault)
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentPending, row.TeardownIntent.State)
	return *row.TeardownIntent
}

// @covers AC-TASKS-ERRANDS-007.1 AC-TASKS-ERRANDS-007.6
func TestErrandTeardownRetriesPhysicalStopFailureAfterCancelledProjection(t *testing.T) {
	f := newErrandTeardownFixture(t)
	fault := errors.New("provider teardown unavailable")
	f.provider.stopErr = fault
	err := NewErrandAdapter(f.svc).CancelErrandWithTarget(t.Context(), f.row, func(target models.ErrandTeardownTarget) error {
		return f.commit(t.Context(), target)
	})
	require.ErrorIs(t, err, fault)
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleCancelled, row.Lifecycle)
	require.Equal(t, models.ErrandTeardownIntentPending, row.TeardownIntent.State)
	f.provider.stopErr = nil
	outcome, err := NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), *row.TeardownIntent)
	require.NoError(t, err)
	require.Equal(t, errands.TeardownCompleted, outcome)
	row, err = f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
	require.Equal(t, []string{"exec-1", "exec-1"}, f.provider.stops)
}

func TestErrandTeardownReadFailureAfterCommitKeepsExactIntentPending(t *testing.T) {
	f := newErrandTeardownFixture(t)
	fault := errors.New("runtime inventory temporarily unavailable")
	err := NewErrandAdapter(f.svc).CancelErrandWithTarget(t.Context(), f.row, func(target models.ErrandTeardownTarget) error {
		if err := f.commit(t.Context(), target); err != nil {
			return err
		}
		f.provider.lookupErr = fault
		return nil
	})
	require.ErrorIs(t, err, fault)
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentPending, row.TeardownIntent.State)
	require.Empty(t, f.provider.stops)
	f.provider.lookupErr = nil
	outcome, err := NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), *row.TeardownIntent)
	require.NoError(t, err)
	require.Equal(t, errands.TeardownCompleted, outcome)
}

func (f *errandTeardownFixture) successor(t *testing.T, newTurn, replaceExecution bool) *models.AgentDeliverySubmission {
	t.Helper()
	ctx := t.Context()
	if newTurn {
		session, err := f.repo.GetTaskSession(ctx, f.row.ID)
		require.NoError(t, err)
		effect := &models.AgentDeliveryEffect{EffectKey: "teardown:A:settlement", StreamID: "exec-1",
			EffectType: db.ErrandTurnSettlementEffect, State: models.DeliveryEffectPending,
			SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
			TurnID: "turn-1", SubmissionID: f.row.SubmissionID, Outcome: "user"}
		require.NoError(t, f.repo.ReserveErrandTurnSettlement(ctx, f.row.TaskID, effect))
		require.NoError(t, f.svc.turnService.CompleteTurn(ctx, "turn-1"))
		now := time.Now().UTC()
		effect.State, effect.Outcome, effect.CompletedAt = models.DeliveryEffectCompleted, "user:cancelled", &now
		row, err := f.repo.GetErrand(ctx, f.row.ID)
		require.NoError(t, err)
		_, err = f.repo.CommitErrandTurnSettlement(ctx, row, effect)
		require.NoError(t, err, "successor admission requires the exact previous-turn decision")
		f.svc.activeTurns.Delete(f.row.ID)
		require.NoError(t, f.repo.UpdateTaskSessionState(ctx, f.row.ID, models.TaskSessionStateWaitingForInput, ""))
		_, _, _, _, _, release, err := f.svc.claimSessionRunningForPrompt(ctx, f.row.TaskID, f.row.ID, "", false, nil, nil, "", false)
		if release != nil {
			release()
		}
		require.NoError(t, err)
	}
	session, err := f.repo.GetTaskSession(ctx, f.row.ID)
	require.NoError(t, err)
	submission := &models.AgentDeliverySubmission{ID: "prompt:successor", SessionID: session.ID,
		IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, OwnerGeneration: 1,
		PayloadHash: "successor-hash", Payload: []byte("successor prompt"), State: models.DeliverySubmissionAccepted}
	_, err = f.repo.PrepareAgentDeliverySubmission(ctx, submission)
	require.NoError(t, err)
	_, err = f.svc.messageQueue.QueueMessage(ctx, session.ID, session.TaskID, "successor queue", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	if replaceExecution {
		f.provider.current = "exec-B"
		f.provider.runtimes["exec-B"] = &agentruntime.AgentExecution{ID: "exec-B", TaskID: session.TaskID,
			SessionID: session.ID, DeliveryIncarnationID: session.QueueIncarnationID}
		seedExecutorRunning(t, f.repo, session.ID, session.TaskID, "exec-B")
	}
	return submission
}

// @covers AC-TASKS-ERRANDS-007.1 AC-TASKS-ERRANDS-007.6
func TestStaleErrandTeardownDoesNotStopSuccessorUserTurn(t *testing.T) {
	for _, variant := range []string{"same execution turn", "same turn submission", "replacement execution", "replacement incarnation"} {
		t.Run(variant, func(t *testing.T) {
			f := newErrandTeardownFixture(t)
			intent := f.pending(t)
			ready, allowRetry, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var outcome errands.TeardownOutcome
			var retryErr error
			var once sync.Once
			allow := func() { once.Do(func() { close(allowRetry) }) }
			t.Cleanup(func() { allow(); coordinatorStopAwaitSignal(t, done, "stale retry cleanup") })
			go func() {
				defer close(done)
				close(ready)
				<-allowRetry
				outcome, retryErr = NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), intent)
			}()
			coordinatorStopAwaitSignal(t, ready, "old intent committed, retry paused before successor admission")
			f.successor(t, variant != "same turn submission", variant == "replacement execution")
			wantOutcome := errands.TeardownSuperseded
			if variant == "replacement execution" {
				wantOutcome = errands.TeardownCompleted
			}
			if variant == "replacement incarnation" {
				_, err := f.repo.DB().ExecContext(t.Context(), "UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?", "replacement-incarnation", f.row.ID)
				require.NoError(t, err)
			}
			before, err := f.repo.GetTaskSession(t.Context(), f.row.ID)
			require.NoError(t, err)
			turn, err := f.repo.GetActiveTurnBySessionID(t.Context(), f.row.ID)
			require.NoError(t, err)
			submission, err := f.repo.GetAgentDeliverySubmission(t.Context(), "prompt:successor")
			require.NoError(t, err)
			queueBefore := f.svc.messageQueue.GetStatus(t.Context(), f.row.ID)
			allow()
			coordinatorStopAwaitSignal(t, done, "old intent retry after successor admission")
			require.NoError(t, retryErr)
			require.Equal(t, wantOutcome, outcome)
			after, err := f.repo.GetTaskSession(t.Context(), f.row.ID)
			require.NoError(t, err)
			require.Equal(t, before, after)
			turnAfter, err := f.repo.GetActiveTurnBySessionID(t.Context(), f.row.ID)
			require.NoError(t, err)
			require.Equal(t, turn, turnAfter)
			submissionAfter, err := f.repo.GetAgentDeliverySubmission(t.Context(), submission.ID)
			require.NoError(t, err)
			require.Equal(t, submission, submissionAfter)
			require.Equal(t, queueBefore, f.svc.messageQueue.GetStatus(t.Context(), f.row.ID))
			if variant == "replacement execution" {
				require.Equal(t, []string{"exec-1"}, f.provider.stops)
				require.NotNil(t, f.provider.runtimes["exec-B"])
			} else {
				require.Empty(t, f.provider.stops)
				require.NotNil(t, f.provider.runtimes["exec-1"])
			}
		})
	}
}

type teardownReceiptFailureRepo struct {
	*sqliterepo.Repository
	fault        error
	wrongVersion bool
}

func (r teardownReceiptFailureRepo) TransitionErrandTeardownIntent(ctx context.Context, id, intentID string, version int64, state models.ErrandTeardownIntentState) (*models.Errand, error) {
	if r.fault != nil {
		return nil, r.fault
	}
	if r.wrongVersion {
		version--
	}
	return r.Repository.TransitionErrandTeardownIntent(ctx, id, intentID, version, state)
}

func TestErrandTeardownCrashAfterStopBeforeReceipt(t *testing.T) {
	for _, boundary := range []string{"receipt write", "row version"} {
		t.Run(boundary, func(t *testing.T) {
			f := newErrandTeardownFixture(t)
			fault := errors.New("receipt transaction unavailable")
			wrapped := teardownReceiptFailureRepo{Repository: f.repo, fault: fault}
			if boundary == "row version" {
				wrapped.fault, wrapped.wrongVersion = nil, true
				fault = models.ErrErrandVersionConflict
			}
			f.svc.repo = wrapped
			err := NewErrandAdapter(f.svc).CancelErrandWithTarget(t.Context(), f.row, func(target models.ErrandTeardownTarget) error {
				return f.commit(t.Context(), target)
			})
			require.ErrorIs(t, err, fault)
			row, err := f.repo.GetErrand(t.Context(), f.row.ID)
			require.NoError(t, err)
			require.Equal(t, models.ErrandTeardownIntentPending, row.TeardownIntent.State)
			require.Equal(t, []string{"exec-1"}, f.provider.stops)
			f.svc.repo = f.repo
			outcome, err := NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), *row.TeardownIntent)
			require.NoError(t, err)
			require.Equal(t, errands.TeardownCompleted, outcome)
			require.Equal(t, []string{"exec-1"}, f.provider.stops)
		})
	}
}

func TestErrandTeardownWrongIntentCannotStopOrOverwrite(t *testing.T) {
	for _, field := range []string{"id", "version", "target", "request time"} {
		t.Run(field, func(t *testing.T) {
			f := newErrandTeardownFixture(t)
			intent := f.pending(t)
			before, err := f.repo.GetErrand(t.Context(), f.row.ID)
			require.NoError(t, err)
			switch field {
			case "id":
				intent.ID = "another-intent"
			case "version":
				intent.Version++
			case "target":
				intent.Target.ExecutionID = "another-execution"
			case "request time":
				intent.RequestedAt = intent.RequestedAt.Add(time.Second)
			}
			outcome, err := NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), intent)
			require.ErrorIs(t, err, models.ErrErrandVersionConflict)
			require.Empty(t, outcome)
			after, err := f.repo.GetErrand(t.Context(), f.row.ID)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Empty(t, f.provider.stops)
		})
	}
}

func TestErrandTeardownUnknownRuntimeDoesNotCompletePendingIntent(t *testing.T) {
	for _, boundary := range []string{"blank lookup", "read error", "unavailable exact inventory", "unverified owner"} {
		t.Run(boundary, func(t *testing.T) {
			f := newErrandTeardownFixture(t)
			intent := f.pending(t)
			switch boundary {
			case "blank lookup":
				f.provider.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) { return "", nil }
			case "read error":
				f.provider.lookupErr = errors.New("inventory recovering")
			case "unavailable exact inventory":
				f.provider.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) { return "exec-B", nil }
				f.svc.agentManager = f.provider.mockAgentManager
			case "unverified owner":
				f.provider.current = ""
				f.provider.runtimes["exec-1"].DeliveryIncarnationID = "wrong-owner"
			}
			outcome, err := NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), intent)
			require.Error(t, err)
			require.Empty(t, outcome)
			row, err := f.repo.GetErrand(t.Context(), f.row.ID)
			require.NoError(t, err)
			require.Equal(t, models.ErrandTeardownIntentPending, row.TeardownIntent.State)
			require.Empty(t, f.provider.stops)
		})
	}
}

func TestErrandTeardownPreparedSubmissionIsNotProvenSupersession(t *testing.T) {
	f := newErrandTeardownFixture(t)
	intent := f.pending(t)
	session, err := f.repo.GetTaskSession(t.Context(), f.row.ID)
	require.NoError(t, err)
	_, err = f.repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{
		ID: "prompt:prepared", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "prepared", Payload: []byte("prepared prompt"), State: models.DeliverySubmissionPrepared,
	})
	require.NoError(t, err)
	outcome, err := NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), intent)
	require.Error(t, err)
	require.Empty(t, outcome)
	require.Empty(t, f.provider.stops)
}

func TestErrandTeardownPersistedGenerationIsNotRestartAuthority(t *testing.T) {
	f := newErrandTeardownFixture(t)
	intent := f.pending(t)
	require.Equal(t, uint64(3), intent.Target.PromptGeneration)
	f.provider.currentPromptGeneration.Store(1)
	outcome, err := NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), intent)
	require.NoError(t, err)
	require.Equal(t, errands.TeardownCompleted, outcome)
	require.Equal(t, []string{"exec-1"}, f.provider.stops)
}

type teardownAdmissionWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *teardownAdmissionWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

// @covers AC-TASKS-ERRANDS-007.1 AC-TASKS-ERRANDS-007.6
func TestErrandTeardownOwnsAdmissionAcrossUnlockedPhysicalStop(t *testing.T) {
	f := newErrandTeardownFixture(t)
	stopReached, allowStop := make(chan struct{}), make(chan struct{})
	stopDone, admissionDone := make(chan struct{}), make(chan struct{})
	stopResult, admissionResult := make(chan error, 1), make(chan error, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var once sync.Once
	allow := func() { once.Do(func() { close(allowStop) }) }
	var reachedOnce sync.Once
	signalReached := func() { reachedOnce.Do(func() { close(stopReached) }) }
	t.Cleanup(func() {
		cancel()
		allow()
		coordinatorStopAwaitSignal(t, stopDone, "stop worker cleanup")
		coordinatorStopAwaitSignal(t, admissionDone, "admission worker cleanup")
	})
	f.provider.beforeStop = func(ctx context.Context, _ string) error {
		defer signalReached()
		guard, release := f.svc.acquireCancelInFlightGuard(f.row.ID)
		defer release()
		if !guard.TryLock() {
			return errors.New("provider stop retained the cancellation guard")
		}
		guard.Unlock()
		releaseLifecycle, acquired := f.svc.tryAcquireSessionLifecycleLock(f.row.ID)
		if !acquired {
			return errors.New("provider stop retained the lifecycle lock")
		}
		releaseLifecycle()
		// This real writer would block if the cancellation SQL transaction
		// still spanned external I/O.
		if err := f.repo.SetSessionMetadataKey(ctx, f.row.ID, "teardown_probe", true); err != nil {
			return err
		}
		signalReached()
		<-allowStop
		return nil
	}
	go func() {
		defer close(stopDone)
		stopResult <- NewErrandAdapter(f.svc).CancelErrandWithTarget(ctx, f.row, func(target models.ErrandTeardownTarget) error {
			return f.commit(ctx, target)
		})
	}()
	coordinatorStopAwaitSignal(t, stopReached, "physical stop boundary")
	waiting := &teardownAdmissionWaitContext{Context: ctx, waiting: make(chan struct{})}
	go func() {
		defer close(admissionDone)
		_, _, _, _, _, release, err := f.svc.claimSessionRunningForPrompt(waiting, f.row.TaskID, f.row.ID, "", false, nil, nil, "", false)
		if release != nil {
			release()
		}
		admissionResult <- err
	}()
	coordinatorStopAwaitSignal(t, waiting.waiting, "prompt waiting on cancellation ownership")
	select {
	case <-admissionDone:
		t.Fatal("prompt passed cancellation ownership during physical stop")
	default:
	}
	allow()
	coordinatorStopAwaitSignal(t, stopDone, "physical stop receipt")
	coordinatorStopAwaitSignal(t, admissionDone, "blocked prompt result")
	require.NoError(t, <-stopResult)
	require.Error(t, <-admissionResult)
	require.Equal(t, []string{"exec-1"}, f.provider.stops)
}

func TestErrandTeardownPublishesOutsideAdmissionAndLifecycleLocks(t *testing.T) {
	f := newErrandTeardownFixture(t)
	eventsBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(eventsBus.Close)
	f.svc.eventBus = eventsBus
	published := false
	_, err := eventsBus.Subscribe(events.TaskSessionStateChanged, func(ctx context.Context, _ *bus.Event) error {
		guard, release := f.svc.acquireCancelInFlightGuard(f.row.ID)
		defer release()
		acquired := guard.TryLock()
		if acquired {
			guard.Unlock()
		}
		require.True(t, acquired, "state publication must permit reentrant guard acquisition")
		releaseLifecycle, acquired := f.svc.tryAcquireSessionLifecycleLock(f.row.ID)
		if acquired {
			releaseLifecycle()
		}
		require.True(t, acquired)
		_, _ = f.svc.agentManager.GetExecutionIDForSession(ctx, f.row.ID)
		published = true
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, NewErrandAdapter(f.svc).CancelErrandWithTarget(t.Context(), f.row, func(target models.ErrandTeardownTarget) error {
		return f.commit(t.Context(), target)
	}))
	require.True(t, published)
}

func TestErrandTeardownMissingTurnEvidenceKeepsIntentPending(t *testing.T) {
	f := newErrandTeardownFixture(t)
	intent := f.pending(t)
	_, err := f.repo.DB().ExecContext(t.Context(), "DELETE FROM task_session_turns WHERE task_session_id = ?", f.row.ID)
	require.NoError(t, err)
	outcome, err := NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), intent)
	require.Error(t, err, "missing evidence is not a positively admitted successor")
	require.Empty(t, outcome)
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentPending, row.TeardownIntent.State)
	require.Empty(t, f.provider.stops)
}

func TestStaleErrandTeardownPreservesCompletedSuccessorSubmissionOnSameTurn(t *testing.T) {
	f := newErrandTeardownFixture(t)
	intent := f.pending(t)
	session, err := f.repo.GetTaskSession(t.Context(), f.row.ID)
	require.NoError(t, err)
	_, err = f.repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{
		ID: "prompt:finished-successor", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "successor", Payload: []byte("B"),
		State: models.DeliverySubmissionCompleted, TerminalTurnID: intent.Target.TurnID,
	})
	require.NoError(t, err)
	_, err = f.repo.DB().ExecContext(t.Context(), "UPDATE agent_delivery_submissions SET terminal_turn_id = ? WHERE id = ?", intent.Target.TurnID, "prompt:finished-successor")
	require.NoError(t, err)
	outcome, err := NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), intent)
	require.NoError(t, err)
	require.Equal(t, errands.TeardownSuperseded, outcome)
	require.Empty(t, f.provider.stops)
}

func TestErrandTeardownExactNoTurnAndLatestTurn(t *testing.T) {
	for _, variant := range []string{"no turn with runtime", "no turn confirmed absent", "latest completed turn"} {
		t.Run(variant, func(t *testing.T) {
			f := newErrandTeardownFixture(t)
			noTurn := variant != "latest completed turn"
			if noTurn {
				_, err := f.repo.DB().ExecContext(t.Context(), "DELETE FROM task_session_turns WHERE task_session_id = ?", f.row.ID)
				require.NoError(t, err)
			} else {
				require.NoError(t, f.repo.CompleteTurn(t.Context(), "turn-1"))
			}
			if variant == "no turn confirmed absent" {
				f.provider.current = ""
				delete(f.provider.runtimes, "exec-1")
			}
			err := NewErrandAdapter(f.svc).CancelErrandWithTarget(t.Context(), f.row, func(target models.ErrandTeardownTarget) error {
				require.Equal(t, noTurn, target.NoTurn)
				if noTurn {
					require.Empty(t, target.TurnID)
					require.Empty(t, target.SubmissionID)
				} else {
					require.Equal(t, "turn-1", target.TurnID)
				}
				return f.commit(t.Context(), target)
			})
			require.NoError(t, err)
			row, err := f.repo.GetErrand(t.Context(), f.row.ID)
			require.NoError(t, err)
			require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
			if variant == "no turn confirmed absent" {
				require.Empty(t, f.provider.stops)
			} else {
				require.Equal(t, []string{"exec-1"}, f.provider.stops)
			}
		})
	}
}

func TestErrandTeardownReservedTupleAndLateSession(t *testing.T) {
	for _, variant := range []string{"absent", "partial tuple", "late initial", "late unbound", "late successor"} {
		t.Run(variant, func(t *testing.T) {
			f := newErrandTeardownFixture(t)
			f.row = &models.Errand{ID: "reserved", TaskID: "t1", ParentSessionID: "parent", Title: "queued",
				FirstTurnID: "reserved-turn", SubmissionID: "prompt:reserved", Lifecycle: models.ErrandLifecycleQueued,
				ResultState: models.ErrandResultNone}
			if variant == "partial tuple" {
				f.row.SubmissionID = ""
			}
			require.NoError(t, f.repo.CreateErrand(t.Context(), f.row))
			if variant == "absent" || variant == "partial tuple" {
				err := NewErrandAdapter(f.svc).CancelErrandWithTarget(t.Context(), f.row, func(target models.ErrandTeardownTarget) error {
					require.Empty(t, target.ExecutionID)
					require.Empty(t, target.SessionIncarnation)
					require.Equal(t, f.row.FirstTurnID, target.FirstTurnID)
					require.Equal(t, f.row.SubmissionID, target.ReservedSubmissionID)
					return f.commit(t.Context(), target)
				})
				if variant == "partial tuple" {
					require.Error(t, err)
					require.Equal(t, models.ErrandLifecycleQueued, f.row.Lifecycle)
				} else {
					require.NoError(t, err)
				}
				require.Empty(t, f.provider.stops)
				return
			}
			intent := f.pending(t)
			seedErrandSession(t, f.repo, f.row.TaskID, f.row.ParentSessionID, f.row.ID, models.TaskSessionStateRunning)
			session, err := f.repo.GetTaskSession(t.Context(), f.row.ID)
			require.NoError(t, err)
			turnID := f.row.FirstTurnID
			if variant == "late successor" {
				turnID = "successor-turn"
			}
			require.NoError(t, f.repo.CreateTurn(t.Context(), &models.Turn{ID: turnID, TaskID: f.row.TaskID,
				TaskSessionID: f.row.ID, StartedAt: time.Now().UTC()}))
			if variant == "late initial" {
				require.NoError(t, f.svc.turnService.PatchTurnMetadata(t.Context(), f.row.ID, turnID, map[string]interface{}{
					errandAdmissionTurnMetadataKey: map[string]interface{}{"submission_id": f.row.SubmissionID,
						"sender_session_id": f.row.ParentSessionID, "state": string(models.ErrandAdmissionAccepted)},
				}))
			}
			f.provider.current = "exec-reserved"
			f.provider.runtimes["exec-reserved"] = &agentruntime.AgentExecution{ID: "exec-reserved",
				TaskID: f.row.TaskID, SessionID: f.row.ID, DeliveryIncarnationID: session.QueueIncarnationID}
			outcome, err := NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), intent)
			if variant == "late unbound" {
				require.Error(t, err, "reserved identity alone cannot target a newly resolved runtime")
				require.Empty(t, outcome)
				require.Empty(t, f.provider.stops)
				return
			}
			require.NoError(t, err)
			if variant == "late initial" {
				require.Equal(t, errands.TeardownCompleted, outcome)
				require.Equal(t, []string{"exec-reserved"}, f.provider.stops)
			} else {
				require.Equal(t, errands.TeardownSuperseded, outcome)
				require.Empty(t, f.provider.stops)
			}
		})
	}
}

func TestErrandTeardownRejectsLegacyMissingAuthorityAndUncommittedCallback(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "callback omitted commit", true: "legacy cancellation"}[legacy], func(t *testing.T) {
			f := newErrandTeardownFixture(t)
			if legacy {
				f.row.Lifecycle, f.row.CancelSource = models.ErrandLifecycleCancelled, models.ErrandCancelSourceUser
				require.NoError(t, f.repo.UpdateErrand(t.Context(), f.row))
			}
			err := NewErrandAdapter(f.svc).CancelErrandWithTarget(t.Context(), f.row, func(models.ErrandTeardownTarget) error { return nil })
			require.Error(t, err)
			require.Empty(t, f.provider.stops)
		})
	}
}

type teardownReadFailureRepo struct {
	*sqliterepo.Repository
	err error
}

func (r teardownReadFailureRepo) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return nil, r.err
}

func TestErrandTeardownReadFailureBeforeCommitLeavesLogicalRowUnchanged(t *testing.T) {
	for _, boundary := range []string{"session", "execution", "prompt"} {
		t.Run(boundary, func(t *testing.T) {
			f := newErrandTeardownFixture(t)
			fault := errors.New("transient " + boundary + " read")
			switch boundary {
			case "session":
				f.svc.repo = teardownReadFailureRepo{Repository: f.repo, err: fault}
			case "execution":
				f.provider.lookupErr = fault
			case "prompt":
				f.provider.promptErr = fault
			}
			adapter, ok := any(NewErrandAdapter(f.svc)).(errandTargetCanceller)
			require.True(t, ok)
			called := false
			err := adapter.CancelErrandWithTarget(t.Context(), f.row, func(models.ErrandTeardownTarget) error {
				called = true
				return nil
			})
			require.ErrorIs(t, err, fault)
			require.False(t, called)
			row, readErr := f.repo.GetErrand(t.Context(), f.row.ID)
			require.NoError(t, readErr)
			require.Equal(t, f.row, row)
			require.Empty(t, f.provider.stops)
		})
	}
}

func TestErrandTeardownCommitFailureDoesNotStopOrMutateSession(t *testing.T) {
	f := newErrandTeardownFixture(t)
	before, err := f.repo.GetTaskSession(t.Context(), f.row.ID)
	require.NoError(t, err)
	adapter, ok := any(NewErrandAdapter(f.svc)).(errandTargetCanceller)
	require.True(t, ok)
	fault := errors.New("cancellation transaction rolled back")
	err = adapter.CancelErrandWithTarget(t.Context(), f.row, func(models.ErrandTeardownTarget) error {
		guard, release := f.svc.acquireCancelInFlightGuard(f.row.ID)
		defer release()
		acquired := guard.TryLock()
		if acquired {
			guard.Unlock()
		}
		require.False(t, acquired, "only the durable callback runs under admission ownership")
		require.True(t, f.svc.isCancelInFlight(f.row.ID))
		return fault
	})
	require.ErrorIs(t, err, fault)
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, f.row, row)
	after, err := f.repo.GetTaskSession(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Empty(t, f.provider.stops)
	require.False(t, f.svc.isCancelInFlight(f.row.ID))
}

func (f *errandTeardownFixture) commit(ctx context.Context, target models.ErrandTeardownTarget) error {
	f.row.Lifecycle, f.row.CancelSource = models.ErrandLifecycleCancelled, models.ErrandCancelSourceUser
	f.row.TeardownIntent = &models.ErrandTeardownIntent{Version: models.ErrandTeardownIntentVersion, ID: "intent-A",
		State: models.ErrandTeardownIntentPending, RequestedAt: time.Now().UTC(), Target: target}
	return f.repo.UpdateErrand(ctx, f.row)
}

// @covers AC-TASKS-ERRANDS-007.1 AC-TASKS-ERRANDS-007.6
func TestErrandTeardownStopsExactExecutionAndPersistsReceipt(t *testing.T) {
	f := newErrandTeardownFixture(t)
	f.provider.beforeStop = func(_ context.Context, id string) error {
		identity, ready := f.svc.cancellationIdentitySnapshot(f.svc.currentCancellation(f.row.ID))
		if !ready || identity.executionID != id || identity.turnID != "turn-1" {
			return errors.New("physical stop has no captured cancellation ownership")
		}
		return nil
	}
	adapter, ok := any(NewErrandAdapter(f.svc)).(errandTargetCanceller)
	require.True(t, ok, "production adapter must support guarded exact-target cancellation")
	require.NoError(t, adapter.CancelErrandWithTarget(t.Context(), f.row, func(target models.ErrandTeardownTarget) error {
		require.Equal(t, "exec-1", target.ExecutionID)
		require.Equal(t, "turn-1", target.TurnID)
		require.Equal(t, "prompt:initial", target.SubmissionID)
		require.NotEmpty(t, target.SessionIncarnation)
		return f.commit(t.Context(), target)
	}))
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
	require.Equal(t, []string{"exec-1"}, f.provider.stops)
}
