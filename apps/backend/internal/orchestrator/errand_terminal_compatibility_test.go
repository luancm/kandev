package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

type errandCompletionAbsenceInventory struct {
	*settlementDispatchManager
	absentExecutionID string
}

func (m *errandCompletionAbsenceInventory) GetExecution(executionID string) (*agentruntime.AgentExecution, bool) {
	if executionID == m.absentExecutionID {
		return nil, false
	}
	return &agentruntime.AgentExecution{ID: executionID}, true
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.4
func TestErrandCompletionLegacyRepairRejectsTerminalSessionWithExactReceipt(t *testing.T) {
	for _, state := range []models.TaskSessionState{models.TaskSessionStateCancelled, models.TaskSessionStateFailed} {
		t.Run(string(state), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := errandCompletionFixtureWithoutQueuedSuccessor(t)
				f.ready(t)
				synctest.Wait()

				before, err := f.repo.GetErrand(t.Context(), f.child.ID)
				require.NoError(t, err)
				require.Equal(t, models.ErrandTeardownIntentCompleted, before.TeardownIntent.State)
				session, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
				require.NoError(t, err)
				effect, err := f.repo.GetErrandTurnSettlementDecision(t.Context(), f.child.ID,
					session.QueueIncarnationID, before.LatestSettledTurnID)
				require.NoError(t, err)
				require.True(t, successfulCompletionDecision(effect), "the historical receipt is exact and successful")

				_, err = f.repo.DB().ExecContext(t.Context(), "UPDATE task_session_errands SET teardown_intent = '' WHERE id = ?", f.child.ID)
				require.NoError(t, err)
				require.NoError(t, f.repo.UpdateTaskSessionState(t.Context(), f.child.ID, state, ""))
				f.manager.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
					return "", agentruntime.ErrNoExecutionForSession
				}

				require.NoError(t, f.errands.RetryErrandCompletion(t.Context(), f.child.ID))

				after, err := f.repo.GetErrand(t.Context(), f.child.ID)
				require.NoError(t, err)
				require.Nil(t, after.TeardownIntent, "a terminal session cannot acquire a legacy success-close obligation")
				require.Equal(t, before.ResultRevision, after.ResultRevision)
				require.Equal(t, before.ResultText, after.ResultText)
				require.Equal(t, before.ResultTurnID, after.ResultTurnID)
				afterSession, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
				require.NoError(t, err)
				require.Equal(t, state, afterSession.State)
			})
		})
	}
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.4
func TestErrandCompletionPendingCloseKeepsCancellationAndRetriesExactStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := errandCompletionFixtureWithoutQueuedSuccessor(t)
		stopErr := errors.New("runtime stop unavailable")
		var stopMu sync.Mutex
		failStop := true
		var stops []string
		manager := f.manager.mockAgentManager
		manager.mu.Lock()
		manager.stopAgentWithReasonFunc = func(_ context.Context, executionID, _ string, _ bool) error {
			stopMu.Lock()
			defer stopMu.Unlock()
			stops = append(stops, executionID)
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
		require.Equal(t, models.ErrandTeardownIntentPending, pending.TeardownIntent.State)
		beforeRevision := pending.ResultRevision
		require.NoError(t, f.repo.UpdateTaskSessionState(t.Context(), f.child.ID, models.TaskSessionStateCancelled, ""))

		manager.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
			return "", agentruntime.ErrNoExecutionForSession
		}
		require.Error(t, f.errands.RetryErrandCompletion(t.Context(), f.child.ID),
			"a CANCELLED label cannot replace missing exact runtime-absence evidence")
		stillPending, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.ErrandTeardownIntentPending, stillPending.TeardownIntent.State)
		require.Equal(t, models.TaskSessionStateCancelled, requireTaskSessionState(t, f, f.child.ID))
		stopMu.Lock()
		require.Equal(t, []string{"settlement-execution"}, stops, "unknown inventory cannot cause another stop")
		failStop = false
		stopMu.Unlock()

		manager.getExecutionIDForSessionFunc = nil
		require.NoError(t, f.errands.RetryErrandCompletion(t.Context(), f.child.ID))
		completed, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.ErrandTeardownIntentCompleted, completed.TeardownIntent.State)
		require.Equal(t, beforeRevision, completed.ResultRevision)
		require.Equal(t, models.TaskSessionStateCancelled, requireTaskSessionState(t, f, f.child.ID),
			"cleanup must preserve the existing cancellation projection")
		stopMu.Lock()
		require.Equal(t, []string{"settlement-execution", "settlement-execution"}, stops,
			"the same durable obligation retries its exact execution once after inventory recovers")
		stopMu.Unlock()
	})
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.4
func TestErrandCompletionPendingCancelledWithConfirmedAbsenceSettlesWithoutAnotherStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := errandCompletionFixtureWithoutQueuedSuccessor(t)
		stopErr := errors.New("runtime stop acknowledgement unavailable")
		var stops []string
		manager := f.manager.mockAgentManager
		manager.mu.Lock()
		manager.stopAgentWithReasonFunc = func(_ context.Context, executionID, _ string, _ bool) error {
			stops = append(stops, executionID)
			return stopErr
		}
		manager.mu.Unlock()

		f.ready(t)
		synctest.Wait()
		pending, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.ErrandTeardownIntentPending, pending.TeardownIntent.State)
		beforeRevision := pending.ResultRevision
		require.NoError(t, f.repo.UpdateTaskSessionState(t.Context(), f.child.ID, models.TaskSessionStateCancelled, ""))

		running, err := f.repo.GetExecutorRunningBySessionID(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.NoError(t, f.repo.DeleteExecutorRunningIfCurrent(t.Context(), f.child.ID, running.AgentExecutionID, running.UpdatedAt),
			"the test removes only the exact persisted execution after runtime disappearance")
		manager.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
			return "", agentruntime.ErrNoExecutionForSession
		}
		f.svc.agentManager = &errandCompletionAbsenceInventory{
			settlementDispatchManager: f.manager,
			absentExecutionID:         running.AgentExecutionID,
		}

		require.NoError(t, f.errands.RetryErrandCompletion(t.Context(), f.child.ID))
		completed, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.ErrandTeardownIntentCompleted, completed.TeardownIntent.State)
		require.Equal(t, beforeRevision, completed.ResultRevision)
		require.Equal(t, models.TaskSessionStateCancelled, requireTaskSessionState(t, f, f.child.ID))
		require.Equal(t, []string{"settlement-execution"}, stops,
			"confirmed absence settles the existing obligation without repeating an unacknowledged stop")
	})
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.4
func TestErrandCompletionClosedReceiptSettlesWithoutOverwritingCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := errandCompletionFixtureWithoutQueuedSuccessor(t)
		fault := errors.New("session projection unavailable")
		wrapped := &errandCompletionFaultRepo{Repository: f.repo, failSessionProject: true, projectionErr: fault}
		f.svc.repo = wrapped
		gate := installErrandCompletionStopGate(f)
		gate.allow()
		f.ready(t)
		synctest.Wait()
		closed, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.ErrandTeardownIntentClosed, closed.TeardownIntent.State)
		select {
		case stopped := <-gate.entered:
			require.Equal(t, "settlement-execution", stopped.call.ExecutionID)
		default:
			t.Fatal("successful completion did not stop its exact execution")
		}
		require.NoError(t, f.repo.UpdateTaskSessionState(t.Context(), f.child.ID, models.TaskSessionStateCancelled, ""))
		f.svc.repo = f.repo

		require.NoError(t, f.errands.RetryErrandCompletion(t.Context(), f.child.ID))
		settled, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.ErrandTeardownIntentCompleted, settled.TeardownIntent.State)
		require.Equal(t, models.TaskSessionStateCancelled, requireTaskSessionState(t, f, f.child.ID))
		select {
		case duplicate := <-gate.entered:
			t.Fatalf("closed receipt repeated physical stop for %s", duplicate.call.ExecutionID)
		default:
		}
	})
}

func requireTaskSessionState(t *testing.T, f *queueSettlementFixture, sessionID string) models.TaskSessionState {
	t.Helper()
	session, err := f.repo.GetTaskSession(t.Context(), sessionID)
	require.NoError(t, err)
	return session.State
}
