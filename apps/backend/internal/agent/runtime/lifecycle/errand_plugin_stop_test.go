package lifecycle

import (
	"context"
	"errors"
	"testing"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

type errandPluginStopOperations struct {
	pluginExecutorOperationsFake
	connection *pluginsdk.ResolveExecutorConnectionRequest
}

func (o *errandPluginStopOperations) ResolveExecutorConnection(_ context.Context, req *pluginsdk.ResolveExecutorConnectionRequest) (*pluginsdk.ResolveExecutorConnectionResponse, error) {
	o.connection = req
	return nil, errors.New("provider connection unavailable")
}

func TestPluginErrandTeardownRequestsExactLeaseWithoutProviderMutation(t *testing.T) {
	provider := testPluginExecutorLaunchProvider()
	resource := &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1,
	}
	row := pluginExecutorRecoveryRecord(t, pluginExecutorPhaseReady, resource)
	owner := agentctltypes.ErrandMCPOwner{TaskID: row.TaskID, ErrandID: "child", Generation: 4}
	row.SessionID, row.TaskID = "errand:child", ""
	operations := &errandPluginStopOperations{}
	exec := NewPluginRemoteExecutor(operations, newTestLogger())
	exec.SetRecoveryDependencies(&pluginExecutorRecoveryProfileLoaderFake{
		profile: models.ExecutorProviderLaunchProfile{Provider: provider, ProfileID: "profile-plugin-recovery"},
	}, nil)
	exact, ok := any(exec).(errandInstanceStopper)
	require.True(t, ok, "plugin teardown must use the saved resource control lease")

	uncredentialed := *row
	uncredentialed.TransientAuthToken = ""
	require.Error(t, exact.stopErrandInstance(t.Context(), &uncredentialed, owner))
	require.Nil(t, operations.connection)

	require.ErrorContains(t, exact.stopErrandInstance(t.Context(), row, owner), "connection lease")
	req := operations.connection
	require.NotNil(t, req)
	require.Equal(t, resource.GetResourceHandle(), req.GetResource().GetResourceHandle())
	require.EqualValues(t, pluginExecutorRuntimePort, req.GetRuntimePort())
	require.Equal(t, owner.TaskID, req.GetContext().GetTaskId())
	require.Equal(t, row.SessionID, req.GetContext().GetSessionId())
	require.Equal(t, row.AgentExecutionID, req.GetContext().GetExecutionId())
	require.EqualValues(t, 7, req.GetContext().GetEnvironmentGeneration())
	require.Empty(t, row.TaskID, "provider scope cannot change recorded execution ownership")
	require.Nil(t, operations.destroyRequest)
	require.Nil(t, operations.provisionRequest)
	require.Nil(t, operations.attachRequest)
}
