package orchestrator

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

type recordingErrandController struct {
	mu        sync.Mutex
	events    []string
	stopErr   error
	drainCall int
}

func (c *recordingErrandController) record(event string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *recordingErrandController) DrainQueued(context.Context) {
	c.mu.Lock()
	c.drainCall++
	c.mu.Unlock()
}

func (c *recordingErrandController) CascadeParent(parentSessionID string) bool {
	c.record("cascade-parent:" + parentSessionID)
	return true
}

func (c *recordingErrandController) CancelOnSessionStop(_ context.Context, sessionID string) error {
	c.record("cancel-on-stop:" + sessionID)
	return c.stopErr
}

func (c *recordingErrandController) CascadeTask(taskID string) bool {
	c.record("cascade-task:" + taskID)
	return true
}

func (c *recordingErrandController) seen() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.events...)
}

type recordingAwaitEnder struct{ parents []string }

func (r *recordingAwaitEnder) EndParentAwaits(parentSessionID string) {
	r.parents = append(r.parents, parentSessionID)
}

// @covers AC-TASKS-ERRANDS-008.2
func TestCeilingSweepTickDrainsQueuedErrandsAfterDeferredLaunches(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	controller := &recordingErrandController{}
	svc.SetErrandController(controller)
	svc.ceilingSweepTick(context.Background())
	require.Equal(t, 1, controller.drainCall)

	svc.SetErrandController(nil)
	require.NotPanics(t, func() { svc.ceilingSweepTick(context.Background()) }, "no controller means no errand drain")
}

// @covers AC-TASKS-ERRANDS-007.1 AC-TASKS-ERRANDS-007.2
func TestStopSessionRecordsTheUserStopBeforeStoppingAndCascadesAfterSuccess(t *testing.T) {
	ctx := context.Background()
	svc, _, _, sessionID, _ := newExecutionLessStopService(t, models.TaskSessionStateWaitingForInput)
	controller := &recordingErrandController{}
	svc.SetErrandController(controller)

	require.NoError(t, svc.StopSession(ctx, sessionID, "stopped via API", false))
	require.Equal(t, []string{"cancel-on-stop:" + sessionID, "cascade-parent:" + sessionID}, controller.seen())
}

func TestStopSessionDoesNotCascadeWhenTheStopFails(t *testing.T) {
	ctx := context.Background()
	svc, _, _, sessionID, _ := newExecutionLessStopService(t, models.TaskSessionStateRunning)
	controller := &recordingErrandController{}
	svc.SetErrandController(controller)

	require.Error(t, svc.StopSession(ctx, sessionID, "stopped via API", false))
	require.Equal(t, []string{"cancel-on-stop:" + sessionID}, controller.seen(),
		"a failed stop leaves the parent running, so its errands keep running")
}

func TestTerminalTransitionsCascadeToErrandsButOtherTransitionsDoNot(t *testing.T) {
	ctx := context.Background()
	f := newCancelTurnFixture(t)
	controller := &recordingErrandController{}
	f.svc.SetErrandController(controller)

	f.svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput, "", true)
	require.Empty(t, controller.seen(), "an idle parent keeps its errands")

	f.svc.updateTaskSessionState(ctx, "t1", "s1", models.TaskSessionStateFailed, "boom", false)
	require.Equal(t, []string{"cascade-parent:s1"}, controller.seen())
}

func TestDeleteSessionRecordsTheDeletionAndCascadesToItsErrands(t *testing.T) {
	ctx := context.Background()
	f := newCancelTurnFixture(t)
	require.NoError(t, f.repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""))
	controller := &recordingErrandController{}
	f.svc.SetErrandController(controller)

	require.NoError(t, f.svc.DeleteSession(ctx, "s1"))
	require.Equal(t, []string{"cancel-on-stop:s1", "cascade-parent:s1"}, controller.seen())
}

func TestCancelErrandsForTaskQueuesTheTaskCascade(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	controller := &recordingErrandController{}
	svc.SetErrandController(controller)
	svc.CancelErrandsForTask(context.Background(), "task-1")
	require.Equal(t, []string{"cascade-task:task-1"}, controller.seen())

	svc.SetErrandController(nil)
	require.NotPanics(t, func() { svc.CancelErrandsForTask(context.Background(), "task-1") })
}

// @covers AC-TASKS-ERRANDS-007.5
func TestCancellingAParentTurnEndsItsAwaitsAndCancelsNoErrand(t *testing.T) {
	ctx := context.Background()
	f := newCancelTurnFixture(t)
	controller := &recordingErrandController{}
	f.svc.SetErrandController(controller)
	ender := &recordingAwaitEnder{}
	f.svc.SetParentAwaitEnder(ender)

	require.NoError(t, f.svc.CancelTurn(ctx, "s1", "turn-1"))
	require.Equal(t, []string{"s1"}, ender.parents)

	f2 := newCancelTurnFixture(t)
	controller2 := &recordingErrandController{}
	f2.svc.SetErrandController(controller2)
	ender2 := &recordingAwaitEnder{}
	f2.svc.SetParentAwaitEnder(ender2)
	require.NoError(t, f2.svc.CancelAgent(ctx, "s1"))
	require.Equal(t, []string{"s1"}, ender2.parents)

	require.Empty(t, controller.seen(), "cancelling a turn must not cancel or cascade to any errand")
	require.Empty(t, controller2.seen())
}

func TestCancelTurnMismatchLeavesAwaitsRunning(t *testing.T) {
	f := newCancelTurnFixture(t)
	ender := &recordingAwaitEnder{}
	f.svc.SetParentAwaitEnder(ender)
	require.ErrorIs(t, f.svc.CancelTurn(context.Background(), "s1", "another-turn"), ErrTurnNotActive)
	require.Empty(t, ender.parents)
}
