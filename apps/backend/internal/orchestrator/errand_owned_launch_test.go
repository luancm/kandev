package orchestrator

import (
	"context"
	"testing"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type ownedErrandLaunchFixtureRuntime struct {
	runtimeapi.Runtime
	launch func(context.Context, runtimeapi.LaunchSpec) (runtimeapi.ExecutionRef, error)
	read   func(context.Context, string) (*runtimeapi.Execution, error)
	start  func(context.Context, string) error
}

func (r ownedErrandLaunchFixtureRuntime) Launch(ctx context.Context, spec runtimeapi.LaunchSpec) (runtimeapi.ExecutionRef, error) {
	return r.launch(ctx, spec)
}

func (r ownedErrandLaunchFixtureRuntime) GetExecution(ctx context.Context, id string) (*runtimeapi.Execution, error) {
	return r.read(ctx, id)
}

func (r ownedErrandLaunchFixtureRuntime) StartExecution(ctx context.Context, id string) error {
	return r.start(ctx, id)
}

func TestErrandAdapterLaunchesOwnedTurnWithoutChildSession(t *testing.T) {
	f := newErrandLaunchFixture(t)
	req := f.request()
	workspace := t.TempDir()
	var execution *runtimeapi.Execution
	started := false
	rt := ownedErrandLaunchFixtureRuntime{
		launch: func(ctx context.Context, spec runtimeapi.LaunchSpec) (runtimeapi.ExecutionRef, error) {
			require.Equal(t, req.AgentProfileID, spec.AgentProfileID)
			require.Equal(t, req.TaskID, spec.Owner.TaskID)
			require.Equal(t, req.ErrandID, spec.Owner.ErrandID)
			require.Empty(t, spec.Owner.SessionID)
			require.Equal(t, workspace, spec.Workspace.Path)
			require.NoError(t, spec.OwnerAdmission.AdmitExecution(ctx, spec.Owner))
			launch, ok := spec.Metadata["launch_request"].(*runtimeapi.LaunchRequest)
			require.True(t, ok)
			require.Empty(t, launch.TaskID)
			require.Empty(t, launch.SessionID)
			require.Equal(t, req.TurnID, launch.TurnID)
			require.True(t, launch.StartAgent)
			require.NotEmpty(t, launch.CandidateExecutionID)
			require.NoError(t, launch.OnExecutionAllocated(ctx, launch.CandidateExecutionID))
			execution = &runtimeapi.Execution{ID: launch.CandidateExecutionID, Owner: spec.Owner, WorkspacePath: workspace}
			return runtimeapi.ExecutionRef{ID: execution.ID}, nil
		},
		read: func(_ context.Context, id string) (*runtimeapi.Execution, error) {
			require.NotNil(t, execution)
			require.Equal(t, execution.ID, id)
			return execution, nil
		},
		start: func(ctx context.Context, id string) error {
			require.NotNil(t, execution)
			require.Equal(t, execution.ID, id)
			descriptor, err := f.repo.GetErrandExecutionDescriptor(ctx, req.ErrandID)
			require.NoError(t, err)
			require.Equal(t, id, descriptor.ExecutionID)
			require.Equal(t, execution.Owner.Generation, descriptor.Generation)
			turn, err := f.repo.GetErrandTurn(ctx, req.ErrandID, req.TurnID)
			require.NoError(t, err)
			require.Empty(t, turn.TaskSessionID)
			started = true
			return nil
		},
	}
	adapter := NewErrandAdapter(f.svc)
	adapter.SetOwnedLaunchRuntime(rt, func(_ context.Context, taskID, parentID string) (*runtimeapi.WorkspaceInfo, error) {
		require.Equal(t, req.TaskID, taskID)
		require.Equal(t, req.ParentSessionID, parentID)
		return &runtimeapi.WorkspaceInfo{
			TaskID: taskID, SessionID: parentID, WorkspaceID: "ws1",
			WorkspacePath: workspace, ExecutorType: string(models.ExecutorTypeLocal),
		}, nil
	})

	result, err := adapter.LaunchErrand(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, errands.LaunchAdmitted, result.Disposition)
	sessions, err := f.repo.ListTaskSessions(t.Context(), req.TaskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1, "the owned conversation needs no child TaskSession")
	require.Equal(t, req.ParentSessionID, sessions[0].ID)
	require.True(t, started, "launch must start the exact bound Errand execution")
}
