package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// fakeErrandStateController is an ErrandController whose parent errand state
// is fixed, so the session events can be tested without errand rows.
type fakeErrandStateController struct {
	fakeErrandComposer
	hasErrands  bool
	outstanding bool
}

func (f *fakeErrandStateController) ParentErrandState(context.Context, string) (bool, bool, error) {
	return f.hasErrands, f.outstanding, nil
}

// reconcilingErrandController records startup reconciliation calls.
type reconcilingErrandController struct {
	fakeErrandComposer
	calls int
}

func (r *reconcilingErrandController) Reconcile(context.Context) error {
	r.calls++
	return nil
}

// @covers AC-TASKS-ERRANDS-006.3
func TestParentSessionEventReportsWaitingOnErrandsOnlyWhileIdleAndOutstanding(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "parent-1", "")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	bus := &mockEventBus{}
	svc.eventBus = bus
	now := time.Now().UTC()

	publish := func(t *testing.T, state models.TaskSessionState, controller *fakeErrandStateController) map[string]interface{} {
		t.Helper()
		svc.SetErrandController(controller)
		parent, err := repo.GetTaskSession(ctx, "parent-1")
		require.NoError(t, err)
		parent.State = state
		bus.events = nil
		svc.publishTaskSessionStateChanged(ctx, "task-1", "parent-1",
			models.TaskSessionStateRunning, state, "", &now, parent)
		return lastSessionStatePayload(t, bus, "parent-1")
	}

	idle := publish(t, models.TaskSessionStateWaitingForInput,
		&fakeErrandStateController{hasErrands: true, outstanding: true})
	require.Equal(t, true, idle["waiting_on_errands"], "an idle parent with outstanding errands waits on them")

	running := publish(t, models.TaskSessionStateRunning,
		&fakeErrandStateController{hasErrands: true, outstanding: true})
	require.Equal(t, false, running["waiting_on_errands"], "a running parent does not report waiting")

	settled := publish(t, models.TaskSessionStateWaitingForInput,
		&fakeErrandStateController{hasErrands: true, outstanding: false})
	require.Equal(t, false, settled["waiting_on_errands"], "nothing outstanding clears the report")

	ordinary := publish(t, models.TaskSessionStateWaitingForInput,
		&fakeErrandStateController{hasErrands: false})
	require.NotContains(t, ordinary, "waiting_on_errands", "a parent without errand rows keeps its payload unchanged")
}

// @covers AC-TASKS-ERRANDS-006.2
func TestParentTurnCompletingWithNothingOutstandingRunsItsWorkflow(t *testing.T) {
	svc := &Service{logger: testLogger()}
	controller := &settlingErrandController{outstanding: false}
	svc.SetErrandController(controller)

	require.False(t, svc.settleParentTurn(context.Background(), "parent-1", errands.SettleCauseTurnDone),
		"a turn that completes with nothing outstanding must not skip workflow completion")
	require.Equal(t, []errands.SettleCause{errands.SettleCauseTurnDone}, controller.causes)
}

// @covers AC-TASKS-ERRANDS-006.5
func TestParentWithOutstandingErrandsAcceptsAPromptAsAnIdleSession(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.SetErrandController(&fakeErrandStateController{hasErrands: true, outstanding: true})

	seedTaskAndSession(t, repo, "task1", "parent1", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(context.Background(), "parent1")
	require.NoError(t, err)
	session.AgentExecutionID = "exec-parent-1"
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-parent-1")
	require.NoError(t, repo.UpdateTaskSession(context.Background(), session))

	_, err = svc.PromptTask(context.Background(), "task1", "parent1", "follow-up", "", false, nil, false)
	require.NoError(t, err, "an idle parent with outstanding errands must accept a prompt")
	require.Len(t, agentMgr.capturedPrompts, 1)
}

// @covers AC-TASKS-ERRANDS-004.8
func TestStartupRepairsErrandStateThroughTheController(t *testing.T) {
	ctx := context.Background()
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	controller := &reconcilingErrandController{}
	svc.SetErrandController(controller)

	svc.reconcileErrandsOnStartup(ctx)

	require.Equal(t, 1, controller.calls, "startup must reconcile undelivered results before serving prompts")
}
