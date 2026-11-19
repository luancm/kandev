package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

type errandAllocationBackend struct {
	runtimeapi.Backend
	execution   *lifecycle.AgentExecution
	onAllocated func()
	started     bool
	stoppedID   string
	stopErr     error
}

func (b *errandAllocationBackend) Launch(ctx context.Context, req *lifecycle.LaunchRequest) (*lifecycle.AgentExecution, error) {
	if err := req.OnExecutionAllocated(ctx, req.CandidateExecutionID); err != nil {
		return nil, err
	}
	b.execution = &lifecycle.AgentExecution{
		ID: req.CandidateExecutionID, Owner: req.Owner, OwnerAdmission: req.OwnerAdmission,
	}
	b.onAllocated()
	return b.execution, nil
}

func (b *errandAllocationBackend) GetExecution(id string) (*lifecycle.AgentExecution, bool) {
	return b.execution, b.execution != nil && b.execution.ID == id
}

func (b *errandAllocationBackend) StartAgentProcess(context.Context, string) error {
	b.started = true
	return nil
}

func (b *errandAllocationBackend) StopAgentWithReason(_ context.Context, id, _ string, _ bool) error {
	b.stoppedID = id
	return b.stopErr
}

func TestErrandCancellationAfterAllocationFencesProcessStart(t *testing.T) {
	ctx := t.Context()
	f := newErrandLaunchFixture(t)
	req := f.request()
	stopErr := errors.New("stop remains uncertain")
	backend := &errandAllocationBackend{stopErr: stopErr}
	workspace := t.TempDir()
	adapter := NewErrandAdapter(f.svc)
	adapter.SetOwnedLaunchRuntime(runtimeapi.New(backend), func(_ context.Context, taskID, parentID string) (*runtimeapi.WorkspaceInfo, error) {
		return &runtimeapi.WorkspaceInfo{
			TaskID: taskID, SessionID: parentID, WorkspaceID: "ws1",
			WorkspacePath: workspace, ExecutorType: string(models.ExecutorTypeWorktree),
		}, nil
	})
	service, err := errands.New(errands.Dependencies{
		Store: f.repo, Launcher: adapter, Stopper: adapter, MCP: adapter,
		Profiles: errandProfileStub{}, Events: &errandEventRecorder{}, Logger: testLogger(),
	})
	require.NoError(t, err)
	backend.onAllocated = func() {
		_, err := service.Cancel(ctx, errands.CancelRequest{
			TaskID: req.TaskID, ErrandID: req.ErrandID, Source: models.ErrandCancelSourceUser,
		})
		require.ErrorIs(t, err, stopErr)
	}

	result, err := adapter.LaunchErrand(ctx, req)
	require.ErrorContains(t, err, "errand launch reservation is no longer runnable")
	require.Equal(t, errands.LaunchBlocked, result.Disposition)
	require.False(t, backend.started)
	require.Equal(t, backend.execution.ID, backend.stoppedID)
	row, err := f.repo.GetErrand(ctx, req.ErrandID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleCancelled, row.Lifecycle)
	require.NotNil(t, row.TeardownIntent)
	require.Equal(t, models.ErrandTeardownIntentPending, row.TeardownIntent.State)
	require.Equal(t, backend.execution.ID, row.TeardownIntent.Target.ExecutionID)
	require.Equal(t, backend.execution.Owner.Generation, row.TeardownIntent.Target.OwnerGeneration)
	backend.stopErr = nil
	outcome, err := adapter.RetryErrandTeardown(ctx, *row.TeardownIntent)
	require.NoError(t, err)
	require.Equal(t, errands.TeardownCompleted, outcome)
}
