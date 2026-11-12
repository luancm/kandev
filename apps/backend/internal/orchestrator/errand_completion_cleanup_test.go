package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type errandCompletionFaultRepo struct {
	*sqliterepo.Repository
	mu                 sync.Mutex
	failTransition     models.ErrandTeardownIntentState
	transitionErr      error
	failSessionProject bool
	projectionErr      error
}

func (r *errandCompletionFaultRepo) TransitionErrandTeardownIntent(
	ctx context.Context,
	errandID, intentID string,
	expectedVersion int64,
	state models.ErrandTeardownIntentState,
) (*models.Errand, error) {
	r.mu.Lock()
	fail := state == r.failTransition && r.transitionErr != nil
	err := r.transitionErr
	r.mu.Unlock()
	if fail {
		return nil, err
	}
	return r.Repository.TransitionErrandTeardownIntent(ctx, errandID, intentID, expectedVersion, state)
}

func (r *errandCompletionFaultRepo) UpdateTaskSessionStateIfCurrentIdentity(
	ctx context.Context,
	taskID, sessionID, incarnationID string,
	expected, next models.TaskSessionState,
	errorMessage string,
) (bool, time.Time, error) {
	r.mu.Lock()
	fail := r.failSessionProject && next == models.TaskSessionStateCompleted
	err := r.projectionErr
	r.mu.Unlock()
	if fail {
		return false, time.Time{}, err
	}
	return r.Repository.UpdateTaskSessionStateIfCurrentIdentity(ctx, taskID, sessionID, incarnationID, expected, next, errorMessage)
}

type errandCompletionStopGate struct {
	entered  chan errandCompletionStopObservation
	release  chan struct{}
	returned chan stopAgentCall
	once     sync.Once
}

type errandCompletionStopObservation struct {
	call      stopAgentCall
	errand    *models.Errand
	errandErr error
}

func installErrandCompletionStopGate(f *queueSettlementFixture) *errandCompletionStopGate {
	manager := f.manager.mockAgentManager
	gate := &errandCompletionStopGate{
		entered:  make(chan errandCompletionStopObservation, 4),
		release:  make(chan struct{}),
		returned: make(chan stopAgentCall, 4),
	}
	manager.mu.Lock()
	manager.stopAgentWithReasonFunc = func(ctx context.Context, executionID, reason string, force bool) error {
		call := stopAgentCall{ExecutionID: executionID, Reason: reason, Force: force}
		errand, err := f.repo.GetErrand(ctx, f.child.ID)
		gate.entered <- errandCompletionStopObservation{call: call, errand: errand, errandErr: err}
		<-gate.release
		gate.returned <- call
		return nil
	}
	manager.mu.Unlock()
	return gate
}

func (g *errandCompletionStopGate) allow() {
	g.once.Do(func() { close(g.release) })
}

func errandCompletionFixtureWithoutQueuedSuccessor(t *testing.T) *queueSettlementFixture {
	t.Helper()
	f := newQueueSettlementFixture(t, false)
	removed, err := f.svc.messageQueue.PurgeSession(t.Context(), f.child.ID)
	require.NoError(t, err)
	require.Equal(t, 1, removed)
	return f
}

func runErrandCompletionEvent(t *testing.T, f *queueSettlementFixture, event string) {
	t.Helper()
	switch event {
	case "ready":
		f.svc.handleAgentReady(t.Context(), watcher.AgentEventData{
			TaskID: f.taskID, SessionID: f.child.ID, AgentExecutionID: "settlement-execution", TurnID: "settlement-A",
		})
	case "process exit":
		f.svc.handleAgentCompleted(t.Context(), watcher.AgentEventData{
			TaskID: f.taskID, SessionID: f.child.ID, AgentExecutionID: "settlement-execution", TurnID: "settlement-A",
		})
	default:
		panic("unknown errand completion event")
	}
}

func requireErrandCompletionStop(
	t *testing.T,
	gate *errandCompletionStopGate,
	eventDone <-chan struct{},
) (errandCompletionStopObservation, bool) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case observation := <-gate.entered:
			return observation, true
		case <-eventDone:
			eventDone = nil
		case <-timer.C:
			return errandCompletionStopObservation{}, false
		}
	}
}

func requireSuccessfulErrandSettlementRow(t *testing.T, row *models.Errand) *models.Errand {
	t.Helper()
	require.NotNil(t, row)
	require.Equal(t, models.ErrandOutcomeDone, row.LastOutcome)
	require.Equal(t, "settlement-A", row.LatestSettledTurnID)
	require.Equal(t, "settlement-A", row.ResultTurnID)
	require.Equal(t, "canonical result A", row.ResultText)
	require.Equal(t, models.ErrandResultPending, row.ResultState)
	require.Empty(t, row.CancelSource)
	require.Equal(t, models.ErrandLifecycleLaunched, row.Lifecycle)
	return row
}

func requireSuccessfulErrandSettlement(t *testing.T, f *queueSettlementFixture) *models.Errand {
	t.Helper()
	row, err := f.repo.GetErrand(t.Context(), f.child.ID)
	require.NoError(t, err)
	return requireSuccessfulErrandSettlementRow(t, row)
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.1 AC-TASKS-ERRAND-COMPLETION-001.2
func TestErrandCompletionCleanupClosesAfterDurableSettlement(t *testing.T) {
	for _, event := range []string{"ready", "process exit"} {
		t.Run(event, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := errandCompletionFixtureWithoutQueuedSuccessor(t)
				gate := installErrandCompletionStopGate(f)
				eventDone := make(chan struct{})
				t.Cleanup(func() {
					gate.allow()
					select {
					case <-eventDone:
					case <-time.After(3 * time.Second):
						t.Error("successful completion handler remained blocked during cleanup")
					}
				})
				go func() {
					defer close(eventDone)
					runErrandCompletionEvent(t, f, event)
				}()

				observation, stopped := requireErrandCompletionStop(t, gate, eventDone)
				if !stopped {
					synctest.Wait()
					requireSuccessfulErrandSettlement(t, f)
					session, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
					require.NoError(t, err)
					t.Errorf("successful %s settlement did not close execution settlement-execution; session state is %s", event, session.State)
					return
				}
				require.NoError(t, observation.errandErr, "read durable errand result at stop callback entry")
				call := observation.call
				require.Equal(t, "settlement-execution", call.ExecutionID)
				require.NotEqual(t, errands.StopReasonErrandCancelled, call.Reason,
					"successful completion must not use cancellation semantics")
				requireSuccessfulErrandSettlementRow(t, observation.errand)
				requireErrandCompletionIntentAtStop(t, observation.errand)
				// Drain runnable fixture goroutines while physical stop is still blocked.
				// This prevents a single early read from missing a premature COMPLETED write.
				synctest.Wait()
				beforeClose, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
				require.NoError(t, err)
				require.NotEqual(t, models.TaskSessionStateCompleted, beforeClose.State,
					"completion is projected only after stop acknowledgement")

				gate.allow()
				select {
				case <-gate.returned:
				case <-time.After(3 * time.Second):
					t.Fatal("runtime stop did not return after the test released it")
				}
				select {
				case <-eventDone:
				case <-time.After(3 * time.Second):
					t.Fatal("successful completion handler did not finish")
				}
				synctest.Wait()
				afterClose, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
				require.NoError(t, err)
				require.Equal(t, models.TaskSessionStateCompleted, afterClose.State,
					"confirmed successful cleanup publishes COMPLETED")
				requireSuccessfulErrandSettlement(t, f)
			})
		})
	}
}

func requireErrandCompletionIntentAtStop(t *testing.T, row *models.Errand) {
	t.Helper()
	require.NotNil(t, row.TeardownIntent, "close authority must be durable before runtime I/O")
	encoded, err := json.Marshal(row.TeardownIntent)
	require.NoError(t, err)
	var intent struct {
		Purpose string `json:"purpose"`
		State   string `json:"state"`
		Target  struct {
			TurnID             string `json:"turn_id"`
			ExecutionID        string `json:"execution_id"`
			SessionIncarnation string `json:"session_incarnation"`
		} `json:"target"`
	}
	require.NoError(t, json.Unmarshal(encoded, &intent))
	require.Equal(t, "completion", intent.Purpose)
	require.Equal(t, "pending", intent.State)
	require.Equal(t, "settlement-A", intent.Target.TurnID)
	require.Equal(t, "settlement-execution", intent.Target.ExecutionID)
	require.NotEmpty(t, intent.Target.SessionIncarnation)
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.3
func TestErrandCompletionCleanupDoesNotStopAnAdmittedSuccessor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newQueueSettlementFixture(t, false)
		gate := installErrandCompletionStopGate(f)
		gate.allow()

		f.ready(t)
		f.awaitDispatch(t)
		// The ready path can own detached cleanup; quiesce it after B is active.
		synctest.Wait()
		active, err := f.repo.GetActiveTurnBySessionID(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.NotNil(t, active)
		require.NotEqual(t, "settlement-A", active.ID)
		session, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.TaskSessionStateRunning, session.State)
		row := requireSuccessfulErrandSettlement(t, f)
		require.Equal(t, "settlement-A", row.ResultTurnID, "the prior result remains available while the successor runs")
		assertErrandCompletionWasNotStopped(t, gate, "completion cleanup", active.ID)

		f.svc.handleAgentCompleted(t.Context(), watcher.AgentEventData{
			TaskID: f.taskID, SessionID: f.child.ID, AgentExecutionID: "settlement-execution", TurnID: "settlement-A",
		})
		// A stale terminal event may enqueue detached cleanup before returning.
		synctest.Wait()
		assertErrandCompletionWasNotStopped(t, gate, "stale completion cleanup", active.ID)
		after, err := f.repo.GetActiveTurnBySessionID(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, active.ID, after.ID)
	})
}

func assertErrandCompletionWasNotStopped(t *testing.T, gate *errandCompletionStopGate, owner, successorTurnID string) {
	t.Helper()
	select {
	case observation := <-gate.entered:
		t.Fatalf("%s stopped %s after successor %s was admitted", owner, observation.call.ExecutionID, successorTurnID)
	default:
	}
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.3
func TestErrandCompletionCleanupDoesNotReplaceCancellationOnProcessExit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := errandCompletionFixtureWithoutQueuedSuccessor(t)
		before, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		before.Lifecycle = models.ErrandLifecycleCancelled
		before.CancelSource = models.ErrandCancelSourceUser
		before.AwaitingResult = false
		before.ResultText = "The cancellation notice."
		before.ResultKind = models.ErrandResultCancelled
		before.ResultState = models.ErrandResultPending
		require.NoError(t, f.repo.UpdateErrand(t.Context(), before))
		before, err = f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)

		gate := installErrandCompletionStopGate(f)
		eventDone := make(chan struct{})
		t.Cleanup(func() {
			gate.allow()
			select {
			case <-eventDone:
			case <-time.After(3 * time.Second):
				t.Error("process-exit handler remained blocked during cleanup")
			}
		})
		go func() {
			defer close(eventDone)
			runErrandCompletionEvent(t, f, "process exit")
		}()
		observation, stopped := requireErrandCompletionStop(t, gate, eventDone)
		if stopped {
			require.Equal(t, "settlement-execution", observation.call.ExecutionID)
			gate.allow()
			select {
			case <-gate.returned:
			case <-time.After(3 * time.Second):
				t.Fatal("runtime stop did not return after the test released it")
			}
		}
		select {
		case <-eventDone:
		case <-time.After(3 * time.Second):
			gate.allow()
			t.Fatal("process-exit handler did not finish")
		}
		// Drain detached cleanup before comparing durable cancellation state.
		synctest.Wait()
		after, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, before, after, "a late successful terminal event cannot supersede cancellation")
		session, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.NotEqual(t, models.TaskSessionStateCompleted, session.State)
	})
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.1 AC-TASKS-ERRAND-COMPLETION-001.2
func TestErrandCompletionCleanupCancellationOvertakesPendingClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := errandCompletionFixtureWithoutQueuedSuccessor(t)
		gate := installErrandCompletionStopGate(f)
		eventDone := make(chan struct{})
		cancelDone := make(chan error, 1)
		t.Cleanup(func() {
			gate.allow()
			select {
			case <-eventDone:
			case <-time.After(3 * time.Second):
				t.Error("successful completion handler remained blocked during cleanup")
			}
		})
		go func() {
			defer close(eventDone)
			runErrandCompletionEvent(t, f, "ready")
		}()

		observation, stopped := requireErrandCompletionStop(t, gate, eventDone)
		require.True(t, stopped, "the successful turn should enter its exact physical close")
		require.NoError(t, observation.errandErr)
		requireErrandCompletionIntentAtStop(t, observation.errand)
		go func() {
			_, err := f.errands.Cancel(t.Context(), errands.CancelRequest{
				ErrandID: f.child.ID, TaskID: f.taskID, ParentSessionID: f.parent,
				Source: models.ErrandCancelSourceUser,
			})
			cancelDone <- err
		}()

		// Cancellation commits its semantic decision before it waits for the
		// already-owned physical stop to return.
		synctest.Wait()
		cancelled, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.ErrandLifecycleCancelled, cancelled.Lifecycle)
		require.Equal(t, models.ErrandCancelSourceUser, cancelled.CancelSource)
		require.Equal(t, models.ErrandResultCancelled, cancelled.ResultKind)
		require.Equal(t, models.ErrandResultPending, cancelled.ResultState)
		require.NotNil(t, cancelled.TeardownIntent)
		require.Equal(t, models.ErrandTeardownPurposeCancellation, cancelled.TeardownIntent.Purpose)
		require.Equal(t, models.ErrandTeardownIntentPending, cancelled.TeardownIntent.State)
		require.Equal(t, observation.errand.TeardownIntent.Target, cancelled.TeardownIntent.Target)
		sessionBeforeAck, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.NotEqual(t, models.TaskSessionStateCompleted, sessionBeforeAck.State)

		gate.allow()
		select {
		case <-gate.returned:
		case <-time.After(3 * time.Second):
			t.Fatal("runtime stop did not return after the test released it")
		}
		select {
		case <-eventDone:
		case <-time.After(3 * time.Second):
			t.Fatal("successful completion handler did not finish after cancellation")
		}
		select {
		case err := <-cancelDone:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Fatal("cancellation did not finish after the close owner released the runtime")
		}
		synctest.Wait()
		final, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.ErrandLifecycleCancelled, final.Lifecycle)
		require.Equal(t, models.ErrandTeardownPurposeCancellation, final.TeardownIntent.Purpose)
		require.Equal(t, models.ErrandTeardownIntentCompleted, final.TeardownIntent.State)
		completedSession, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.TaskSessionStateCancelled, completedSession.State)
	})
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.1 AC-TASKS-ERRAND-COMPLETION-001.2
func TestErrandCompletionCleanupRetriesStopFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := errandCompletionFixtureWithoutQueuedSuccessor(t)
		stopErr := errors.New("runtime stop unavailable")
		var failMu sync.Mutex
		failStop := true
		manager := f.manager.mockAgentManager
		manager.mu.Lock()
		manager.stopAgentWithReasonFunc = func(context.Context, string, string, bool) error {
			failMu.Lock()
			defer failMu.Unlock()
			if failStop {
				return stopErr
			}
			return nil
		}
		manager.mu.Unlock()

		f.ready(t)
		synctest.Wait()
		pending, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.NotNil(t, pending.TeardownIntent)
		require.Equal(t, models.ErrandTeardownPurposeCompletion, pending.TeardownIntent.Purpose)
		require.Equal(t, models.ErrandTeardownIntentPending, pending.TeardownIntent.State)
		session, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.NotEqual(t, models.TaskSessionStateCompleted, session.State)

		failMu.Lock()
		failStop = false
		failMu.Unlock()
		f.errands.SetEnabled(func() bool { return false })
		require.NoError(t, f.errands.RetryErrandCompletion(t.Context(), f.child.ID))
		completed, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.ErrandTeardownIntentCompleted, completed.TeardownIntent.State)
		session, err = f.repo.GetTaskSession(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.TaskSessionStateCompleted, session.State)
	})
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.2 AC-TASKS-ERRAND-COMPLETION-001.4
func TestErrandCompletionCleanupRetriesReceiptAndProjectionFailures(t *testing.T) {
	for _, boundary := range []string{"closed receipt", "completed receipt", "session projection"} {
		t.Run(boundary, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := errandCompletionFixtureWithoutQueuedSuccessor(t)
				fault := errors.New("durable completion boundary unavailable")
				wrapped := &errandCompletionFaultRepo{Repository: f.repo, transitionErr: fault, projectionErr: fault}
				switch boundary {
				case "closed receipt":
					wrapped.failTransition = models.ErrandTeardownIntentClosed
				case "completed receipt":
					wrapped.failTransition = models.ErrandTeardownIntentCompleted
				case "session projection":
					wrapped.failSessionProject = true
				}
				f.svc.repo = wrapped

				f.ready(t)
				synctest.Wait()
				row, err := f.repo.GetErrand(t.Context(), f.child.ID)
				require.NoError(t, err)
				session, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
				require.NoError(t, err)
				switch boundary {
				case "closed receipt":
					require.Equal(t, models.ErrandTeardownIntentPending, row.TeardownIntent.State)
					require.NotEqual(t, models.TaskSessionStateCompleted, session.State)
				case "completed receipt":
					require.Equal(t, models.ErrandTeardownIntentClosed, row.TeardownIntent.State)
					require.Equal(t, models.TaskSessionStateCompleted, session.State)
				case "session projection":
					require.Equal(t, models.ErrandTeardownIntentClosed, row.TeardownIntent.State)
					require.NotEqual(t, models.TaskSessionStateCompleted, session.State)
				}

				wrapped.mu.Lock()
				wrapped.failTransition = ""
				wrapped.failSessionProject = false
				wrapped.mu.Unlock()
				require.NoError(t, f.svc.ValidateErrandContinuation(t.Context(), f.child.ID))
				row, err = f.repo.GetErrand(t.Context(), f.child.ID)
				require.NoError(t, err)
				require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
				session, err = f.repo.GetTaskSession(t.Context(), f.child.ID)
				require.NoError(t, err)
				require.Equal(t, models.TaskSessionStateCompleted, session.State)
			})
		})
	}
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.1 AC-TASKS-ERRAND-COMPLETION-001.2
func TestErrandCompletionCleanupUsesExactUserNoResultDecision(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := errandCompletionFixtureWithoutQueuedSuccessor(t)
		row, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		row.AwaitingResult = false
		row.LastOutcome = models.ErrandOutcomeFailed
		require.NoError(t, f.repo.UpdateErrand(t.Context(), row))

		// An aggregate outcome without an exact terminal receipt cannot close an
		// execution or determine the outcome of a later user turn.
		gate := installErrandCompletionStopGate(f)
		require.NoError(t, f.errands.RetryErrandCompletion(t.Context(), f.child.ID))
		assertErrandCompletionWasNotStopped(t, gate, "LastOutcome-only retry", "settlement-A")
		gate.allow()
		session, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.NotEqual(t, models.TaskSessionStateCompleted, session.State)

		session, err = f.repo.GetTaskSession(t.Context(), f.child.ID)
		require.NoError(t, err)
		target := errands.ErrandTeardownTarget{
			TaskID: f.taskID, SessionID: f.child.ID, SessionIncarnation: session.QueueIncarnationID,
			ExecutionID: "settlement-execution", TurnID: "settlement-A", SubmissionID: row.SubmissionID,
		}
		require.NoError(t, f.svc.reserveErrandTerminalIntent(t.Context(), session,
			errands.ParentPromptOrigin{}, target.TurnID, target.SubmissionID, target.ExecutionID))
		require.NoError(t, f.svc.turnService.CompleteTurn(t.Context(), target.TurnID))
		require.NoError(t, f.errands.OnErrandTurnSettled(t.Context(), errands.TurnSettlement{
			SessionID: f.child.ID, TurnID: target.TurnID, SubmissionID: target.SubmissionID,
			Outcome: models.ErrandOutcomeDone, CompletionTarget: &target,
		}))
		settled, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.ErrandOutcomeFailed, settled.LastOutcome, "user follow-up preserves the prior parent outcome")
		require.Equal(t, models.ErrandResultNone, settled.ResultState, "user-origin completion creates no parent result")
		require.Empty(t, settled.ResultText)
		require.Equal(t, target.TurnID, settled.LatestSettledTurnID)
		require.NotNil(t, settled.TeardownIntent)
		require.Equal(t, models.ErrandTeardownPurposeCompletion, settled.TeardownIntent.Purpose)

		require.NoError(t, f.errands.RetryErrandCompletion(t.Context(), f.child.ID))
		completed, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.ErrandTeardownIntentCompleted, completed.TeardownIntent.State)
		require.Equal(t, models.ErrandResultNone, completed.ResultState)
		require.Empty(t, completed.ResultText)
		session, err = f.repo.GetTaskSession(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.TaskSessionStateCompleted, session.State)
	})
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.3
func TestErrandCompletionCleanupInspectionDoesNotResume(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := errandCompletionFixtureWithoutQueuedSuccessor(t)
		gate := installErrandCompletionStopGate(f)
		gate.allow()
		f.ready(t)
		synctest.Wait()
		execution, session, resumed, err := f.svc.focusTaskSession(t.Context(), f.taskID, f.child.ID)
		require.NoError(t, err)
		require.Nil(t, execution)
		require.False(t, resumed)
		require.Equal(t, models.TaskSessionStateCompleted, session.State)
	})
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.3
func TestErrandCompletionCleanupClosedReceiptCannotCompleteReplacement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := errandCompletionFixtureWithoutQueuedSuccessor(t)
		wrapped := &errandCompletionFaultRepo{Repository: f.repo, failSessionProject: true, projectionErr: errors.New("projection unavailable")}
		f.svc.repo = wrapped
		gate := installErrandCompletionStopGate(f)
		gate.allow()
		f.ready(t)
		synctest.Wait()
		row, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.ErrandTeardownIntentClosed, row.TeardownIntent.State)
		f.manager.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) { return "replacement-execution", nil }
		wrapped.mu.Lock()
		wrapped.failSessionProject = false
		wrapped.mu.Unlock()
		require.Error(t, f.errands.RetryErrandCompletion(t.Context(), f.child.ID))
		session, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.NotEqual(t, models.TaskSessionStateCompleted, session.State)
	})
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.4
func TestErrandCompletionCleanupLegacyRepairRequiresExactReceipt(t *testing.T) {
	for _, exact := range []bool{true, false} {
		t.Run(map[bool]string{true: "exact", false: "unknown"}[exact], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := errandCompletionFixtureWithoutQueuedSuccessor(t)
				gate := installErrandCompletionStopGate(f)
				gate.allow()
				f.ready(t)
				synctest.Wait()
				_, err := f.repo.DB().ExecContext(t.Context(), "UPDATE task_session_errands SET teardown_intent = '' WHERE id = ?", f.child.ID)
				require.NoError(t, err)
				require.NoError(t, f.repo.UpdateTaskSessionState(t.Context(), f.child.ID, models.TaskSessionStateWaitingForInput, ""))
				if !exact {
					_, err = f.repo.DB().ExecContext(t.Context(), "DELETE FROM agent_delivery_effects WHERE session_id = ?", f.child.ID)
					require.NoError(t, err)
				}
				f.manager.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) { return "", agentruntime.ErrNoExecutionForSession }
				require.NoError(t, f.errands.RetryErrandCompletion(t.Context(), f.child.ID))
				session, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
				require.NoError(t, err)
				if exact {
					require.Equal(t, models.TaskSessionStateCompleted, session.State)
				} else {
					require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
				}
			})
		})
	}
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.1 AC-TASKS-ERRAND-COMPLETION-001.4
func TestErrandCompletionCleanupStorageFailureRetainsRuntimeUntilRecovery(t *testing.T) {
	for _, event := range []string{"ready", "process exit"} {
		t.Run(event, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := errandCompletionFixtureWithoutQueuedSuccessor(t)
				sessionBefore, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
				require.NoError(t, err)
				initial, err := f.repo.GetErrand(t.Context(), f.child.ID)
				require.NoError(t, err)
				require.NoError(t, f.svc.reserveErrandTerminalIntent(t.Context(), sessionBefore, errands.ParentPromptOrigin{SenderSessionID: f.parent}, "settlement-A", initial.SubmissionID, "settlement-execution"))
				gate := installErrandCompletionStopGate(f)
				gate.allow()
				_, err = f.repo.DB().ExecContext(t.Context(), `CREATE TRIGGER reject_completion BEFORE UPDATE ON task_session_errands WHEN NEW.teardown_intent LIKE '%"purpose":"completion"%' BEGIN SELECT RAISE(ABORT,'completion storage unavailable'); END`)
				require.NoError(t, err)
				runErrandCompletionEvent(t, f, event)
				synctest.Wait()
				assertErrandCompletionWasNotStopped(t, gate, "failed durable settlement", "settlement-A")
				_, err = f.repo.DB().ExecContext(t.Context(), "DROP TRIGGER reject_completion")
				require.NoError(t, err)
				require.NoError(t, f.errands.ReconcileTerminalDecisions(t.Context(), f.child.ID))
				require.NoError(t, f.errands.RetryErrandCompletion(t.Context(), f.child.ID))
				session, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
				require.NoError(t, err)
				require.Equal(t, models.TaskSessionStateCompleted, session.State)
			})
		})
	}
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.3
func TestErrandCompletionCleanupCancellationTargetsActiveSuccessor(t *testing.T) {
	for _, resolved := range []bool{false, true} {
		t.Run(fmt.Sprintf("resolved=%t", resolved), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newQueueSettlementFixture(t, false)
				gate := installErrandCompletionStopGate(f)
				gate.allow()
				f.ready(t)
				f.awaitDispatch(t)
				synctest.Wait()
				active, err := f.repo.GetActiveTurnBySessionID(t.Context(), f.child.ID)
				require.NoError(t, err)
				require.NotNil(t, active)
				require.NotEqual(t, "settlement-A", active.ID)
				row, err := f.repo.GetErrand(t.Context(), f.child.ID)
				require.NoError(t, err)
				require.NotNil(t, row.TeardownIntent)
				if resolved && row.TeardownIntent.State == models.ErrandTeardownIntentPending {
					_, err = f.repo.TransitionErrandTeardownIntent(t.Context(), row.ID, row.TeardownIntent.ID, row.Version, models.ErrandTeardownIntentSuperseded)
					require.NoError(t, err)
				}
				f.manager.currentPromptExecutionID = "settlement-execution"
				_, err = f.errands.Cancel(t.Context(), errands.CancelRequest{TaskID: f.taskID, ErrandID: f.child.ID, Source: models.ErrandCancelSourceUser})
				require.NoError(t, err)
				synctest.Wait()
				cancelled, err := f.repo.GetErrand(t.Context(), f.child.ID)
				require.NoError(t, err)
				require.Equal(t, models.ErrandLifecycleCancelled, cancelled.Lifecycle)
				require.Equal(t, models.ErrandTeardownPurposeCancellation, cancelled.TeardownIntent.Purpose)
				require.Equal(t, active.ID, cancelled.TeardownIntent.Target.TurnID)
				require.Equal(t, models.ErrandTeardownIntentCompleted, cancelled.TeardownIntent.State)
				session, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
				require.NoError(t, err)
				require.Equal(t, models.TaskSessionStateCancelled, session.State)
				select {
				case stopped := <-gate.entered:
					require.Equal(t, errands.StopReasonErrandCancelled, stopped.call.Reason)
				default:
					t.Fatal("cancelling the active successor did not stop its execution")
				}
			})
		})
	}
}
