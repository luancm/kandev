package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type errandLaunchRollbackBackend struct {
	MockExecutor
	genericCalled bool
	exactRow      *models.ExecutorRunning
	exactOwner    agentctltypes.ErrandMCPOwner
	stopErr       error
}

func (b *errandLaunchRollbackBackend) StopInstance(context.Context, *ExecutorInstance, bool) error {
	b.genericCalled = true
	return nil
}

func (b *errandLaunchRollbackBackend) stopErrandInstance(_ context.Context, row *models.ExecutorRunning, owner agentctltypes.ErrandMCPOwner) error {
	b.exactRow, b.exactOwner = row, owner
	return b.stopErr
}

func TestErrandLaunchRollbackUsesExactInstanceStop(t *testing.T) {
	for _, stage := range []string{"allocated", "before_registration", "after_registration"} {
		t.Run(stage, func(t *testing.T) {
			mgr := newTestManager(t)
			stopErr := errors.New("unconfirmed")
			rt := &errandLaunchRollbackBackend{MockExecutor: MockExecutor{name: executor.NameDocker}, stopErr: stopErr}
			owner := ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", WorkspaceID: "ws", ErrandID: "child", Generation: 1}
			execution := &AgentExecution{ID: "child-runtime", Owner: owner, RuntimeName: rt.Name()}
			instance := &ExecutorInstance{
				InstanceID: execution.ID, SessionID: "errand:child", RuntimeName: rt.Name(),
				ContainerID: "shared-parent-container", ReusedWorkspace: true, AuthToken: "control-token",
				Metadata: map[string]interface{}{MetadataKeyRemoteDockerAgentctlInstanceID: "remote-errand"},
			}
			var err error
			switch stage {
			case "allocated":
				req := &ExecutorCreateRequest{ErrandMCPOwner: errandMCPOwner(owner)}
				err = mgr.stopAllocatedLaunchRuntimeOnRollback(t.Context(), rt, instance, req)
			case "before_registration":
				err = mgr.stopLaunchRuntimeOnRollback(t.Context(), rt, instance, execution)
			case "after_registration":
				err = mgr.stopRegisteredLaunchRuntime(rt, instance, execution)
			}
			require.ErrorIs(t, err, stopErr)
			require.False(t, rt.genericCalled, "Errand rollback cannot stop shared compute")
			require.NotNil(t, rt.exactRow)
			require.Equal(t, execution.ID, rt.exactRow.AgentExecutionID)
			require.Equal(t, instance.ContainerID, rt.exactRow.ContainerID)
			require.Equal(t, "remote-errand", rt.exactRow.Metadata[MetadataKeyRemoteDockerAgentctlInstanceID])
			require.Equal(t, instance.AuthToken, rt.exactRow.TransientAuthToken)
			require.Equal(t, *errandMCPOwner(owner), rt.exactOwner)
		})
	}
}
