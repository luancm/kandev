package lifecycle

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

type errandPluginRecoveryOperations struct {
	pluginExecutorOperationsFake
	connections []*pluginsdk.ResolveExecutorConnectionRequest
}

func (o *errandPluginRecoveryOperations) ResolveExecutorConnection(ctx context.Context, request *pluginsdk.ResolveExecutorConnectionRequest) (*pluginsdk.ResolveExecutorConnectionResponse, error) {
	o.connections = append(o.connections, request)
	return o.pluginExecutorOperationsFake.ResolveExecutorConnection(ctx, request)
}

func TestPluginErrandRecoveryReattachesOnlyTheSavedHelper(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1,
	}
	row := pluginExecutorRecoveryRecord(t, pluginExecutorPhaseReady, resource)
	owner := ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: row.TaskID, ErrandID: "child", Generation: 4,
		WorkspaceID: "workspace", AgentProfileID: row.ExecutionProfileID}
	row.SessionID, row.TaskID = models.ErrandExecutionIdentity(owner.ErrandID), ""
	row.WorktreePath = pluginExecutorWorkspacePath
	row.Metadata[runExecutionOwnerMetadataKey] = owner
	operations := &errandPluginRecoveryOperations{pluginExecutorOperationsFake: pluginExecutorOperationsFake{
		connectionResponse: &pluginsdk.ResolveExecutorConnectionResponse{Lease: &pluginsdk.ExecutorConnectionLease{
			BaseUrl: "https://executor.example", ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), Generation: "errand-lease",
		}},
	}}
	store := &pluginExecutorInventoryStoreFake{record: row}
	exec := NewPluginRemoteExecutor(operations, newTestLogger())
	exec.SetRecoveryDependencies(&pluginExecutorRecoveryProfileLoaderFake{
		profile: models.ExecutorProviderLaunchProfile{Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery"},
	}, store)
	wirePluginErrandRecoveryControl(t, exec, row, owner)

	instances, err := exec.RecoverInstances(t.Context(), []*models.ExecutorRunning{row})
	require.NoError(t, err)
	require.Len(t, instances, 1, "a retained owned helper should reconnect without a TaskSession")
	instance := instances[0]
	defer instance.Client.Close()
	require.Equal(t, owner, instance.recoveredErrandOwner)
	require.Equal(t, row.AgentExecutionID, instance.InstanceID)
	require.Equal(t, row.SessionID, instance.SessionID)
	require.Empty(t, instance.TaskID)
	require.Equal(t, "original-plugin-conversation", instance.ProviderSessionID)
	require.True(t, instance.ReusedWorkspace)

	require.Len(t, operations.connections, 2)
	for _, connection := range operations.connections {
		require.Equal(t, resource.ResourceHandle, connection.GetResource().GetResourceHandle())
		require.Equal(t, owner.TaskID, connection.GetContext().GetTaskId())
		require.Equal(t, row.SessionID, connection.GetContext().GetSessionId())
		require.Equal(t, row.AgentExecutionID, connection.GetContext().GetExecutionId())
		require.EqualValues(t, 7, connection.GetContext().GetEnvironmentGeneration())
	}
	require.ElementsMatch(t, []int{pluginExecutorRuntimePort, testPluginExecutorInstancePort}, operations.connectionPorts)

	wrong := *row
	wrong.Metadata = clonePluginExecutorMetadata(row.Metadata)
	foreign := owner
	foreign.Generation++
	wrong.Metadata[runExecutionOwnerMetadataKey] = foreign
	instances, err = exec.RecoverInstances(t.Context(), []*models.ExecutorRunning{&wrong})
	require.NoError(t, err)
	require.Empty(t, instances, "the authenticated helper reports a different owner generation")
	require.Nil(t, operations.attachRequest)
	require.Nil(t, operations.recoverRequest)
	require.Nil(t, operations.provisionRequest)
	require.Nil(t, operations.destroyRequest)
	require.Zero(t, store.checkpoints, "reconnecting an Errand must not rewrite provider environment ownership")
}

func wirePluginErrandRecoveryControl(t *testing.T, exec *PluginRemoteExecutor, row *models.ExecutorRunning, owner ExecutionOwner) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer "+row.TransientAuthToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/identity":
			_ = json.NewEncoder(w).Encode(agentctl.IdentityInfo{Capabilities: []string{agentctltypes.ErrandMCPOwnerCapability}})
		case req.Method == http.MethodGet && req.URL.Path == "/api/v1/instances/"+row.AgentExecutionID:
			_ = json.NewEncoder(w).Encode(agentctl.InstanceInfo{
				ID: row.AgentExecutionID, SessionID: row.SessionID, Port: testPluginExecutorInstancePort, WorkspacePath: row.WorktreePath,
				ErrandMCPOwner: errandMCPOwner(owner), ProviderSessionID: "original-plugin-conversation",
			})
		case req.Method == http.MethodGet && req.URL.Path == "/health":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(server.Close)
	port := server.Listener.Addr().(*net.TCPAddr).Port
	exec.recoveredAgentctlInfoReader = func(ctx context.Context, resolve agentctl.ConnectionLeaseResolver, log *logger.Logger, executionID, token string, expected *agentctltypes.ErrandMCPOwner) (*agentctl.InstanceInfo, error) {
		if _, err := resolve(ctx); err != nil {
			return nil, err
		}
		control := agentctl.NewControlClient("127.0.0.1", port, log, agentctl.WithControlAuthToken(token))
		defer control.Close()
		if err := requireErrandQuestionPolicy(ctx, control, expected); err != nil {
			return nil, err
		}
		return control.GetInstance(ctx, executionID)
	}
	exec.newAgentctlClient = func(ctx context.Context, resolve agentctl.ConnectionLeaseResolver, log *logger.Logger, executionID, token string) (*agentctl.Client, error) {
		if _, err := resolve(ctx); err != nil {
			return nil, err
		}
		return agentctl.NewClient("127.0.0.1", port, log, agentctl.WithExecutionID(executionID), agentctl.WithAuthToken(token)), nil
	}
}
