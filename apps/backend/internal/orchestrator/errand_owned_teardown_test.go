package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type ownedErrandStopRuntime struct {
	runtimeapi.Runtime
	execution *runtimeapi.Execution
	stop      func(string) error
}

func (r ownedErrandStopRuntime) GetExecution(_ context.Context, id string) (*runtimeapi.Execution, error) {
	if id != r.execution.ID {
		return nil, runtimeapi.ErrNotFound
	}
	return r.execution, nil
}

func (r ownedErrandStopRuntime) Stop(_ context.Context, id, _ string) error {
	return r.stop(id)
}

func TestCancelOwnedErrandRetainsTargetUntilRuntimeConfirmsStop(t *testing.T) {
	ctx := t.Context()
	f := newErrandLaunchFixture(t)
	req := f.request()
	f.svc.repo = ownedLaunchRepository{Repository: f.repo, errandID: req.ErrandID}
	row, err := f.repo.GetErrand(ctx, req.ErrandID)
	require.NoError(t, err)
	descriptor, err := f.repo.ReserveErrandExecutionGeneration(ctx, req.ErrandID, 0)
	require.NoError(t, err)
	execution := &runtimeapi.Execution{ID: "owned-instance", Owner: runtimeapi.ExecutionOwner{
		Kind: runtimeapi.ExecutionOwnerErrand, WorkspaceID: "ws1", TaskID: req.TaskID,
		ErrandID: req.ErrandID, Generation: descriptor.Generation, AgentProfileID: req.AgentProfileID,
	}}
	bound, err := f.repo.BindErrandExecution(ctx, row.ID, descriptor.Generation, execution.ID)
	require.NoError(t, err)
	require.True(t, bound)
	row.Lifecycle = models.ErrandLifecycleLaunched
	require.NoError(t, f.repo.UpdateErrand(ctx, row))

	uncertain := errors.New("stop confirmation unavailable")
	calls := 0
	runtime := ownedErrandStopRuntime{execution: execution, stop: func(id string) error {
		require.Equal(t, execution.ID, id)
		current, err := f.repo.GetErrand(ctx, row.ID)
		require.NoError(t, err)
		require.Equal(t, models.ErrandLifecycleCancelled, current.Lifecycle)
		require.Equal(t, models.ErrandTeardownIntentPending, current.TeardownIntent.State)
		require.Equal(t, execution.ID, current.TeardownIntent.Target.ExecutionID)
		calls++
		if calls == 1 {
			return uncertain
		}
		return nil
	}}
	adapter := NewErrandAdapter(f.svc)
	adapter.SetOwnedLaunchRuntime(runtime, nil)
	err = adapter.CancelErrandWithTarget(ctx, row, func(target models.ErrandTeardownTarget) error {
		next := *row
		next.Lifecycle, next.CancelSource = models.ErrandLifecycleCancelled, models.ErrandCancelSourceUser
		next.TeardownIntent = &models.ErrandTeardownIntent{
			Version: models.ErrandTeardownIntentVersion, ID: "owned-stop", RequestedAt: time.Now().UTC(),
			State: models.ErrandTeardownIntentPending, Purpose: models.ErrandTeardownPurposeCancellation, Target: target,
		}
		return f.repo.UpdateErrand(ctx, &next)
	})
	require.ErrorIs(t, err, uncertain)
	current, err := f.repo.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentPending, current.TeardownIntent.State)
	_, err = adapter.RetryErrandTeardown(ctx, *current.TeardownIntent)
	require.NoError(t, err)
	current, err = f.repo.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentCompleted, current.TeardownIntent.State)
	require.Equal(t, 2, calls)
	sessions, err := f.repo.ListTaskSessions(ctx, req.TaskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, req.ParentSessionID, sessions[0].ID)
}
