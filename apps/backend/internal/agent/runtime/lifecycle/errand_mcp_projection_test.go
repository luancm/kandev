package lifecycle

import (
	"context"
	"encoding/json"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestErrandMCPProjectionReachesInstanceCreateRequests(t *testing.T) {
	owner := ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "child", Generation: 2}
	projection := errandMCPOwner(owner)
	req := &ExecutorCreateRequest{TaskID: "", SessionID: "errand:child", ErrandMCPOwner: projection}
	standalone := buildStandaloneCreateInstanceRequest(req, nil, "", false, false, false, false, nil)
	ssh := buildSSHCreateInstanceRequest(req, "/workspace", "agentctl")
	sprites := agentctlInstanceRequest(req, "/workspace")
	reconnect := buildReconnectCreateInstanceRequest(req, "instance")
	docker, err := buildDockerContainerConfig(req, "local_docker")
	require.NoError(t, err)
	container := buildContainerCreateInstanceRequest(docker, "", false, false, false, false, nil)
	for _, create := range []*agentctl.CreateInstanceRequest{standalone, &ssh, &sprites, reconnect, container} {
		require.Equal(t, projection, create.ErrandMCPOwner)
		require.Empty(t, create.TaskID)
		require.Equal(t, "errand:child", create.SessionID)
		data, err := json.Marshal(create)
		require.NoError(t, err)
		var decoded agentctl.CreateInstanceRequest
		require.NoError(t, json.Unmarshal(data, &decoded))
		require.Equal(t, projection, decoded.ErrandMCPOwner)
	}
	require.Nil(t, errandMCPOwner(ExecutionOwner{Kind: ExecutionOwnerTask, TaskID: "task"}))
}

func TestErrandMCPRemoteHelperRequiresCapability(t *testing.T) {
	req := &ExecutorCreateRequest{ErrandMCPOwner: errandMCPOwner(ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "child", Generation: 1})}
	fetch := func(context.Context) ([]byte, error) { return []byte(`{"capabilities":[]}`), nil }
	require.Error(t, requireBuiltinRemoteHelper(context.Background(), req, fetch))
	fetch = func(context.Context) ([]byte, error) { return []byte(`{"capabilities":["errand-mcp-owner.v2"]}`), nil }
	require.NoError(t, requireBuiltinRemoteHelper(context.Background(), req, fetch))
}

func TestKubernetesRecoveredErrandRequiresPersistedOwnerEvidence(t *testing.T) {
	owner := ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", WorkspaceID: "workspace", ErrandID: "child", Generation: 1}
	record := &models.ExecutorRunning{SessionID: "errand:child", AgentExecutionID: "execution", Metadata: map[string]interface{}{runExecutionOwnerMetadataKey: owner}}
	recorded := kubernetesRecordedState{agentctlInstanceID: "execution", remotePort: 1234}
	info := &agentctl.InstanceInfo{ID: "execution", SessionID: "errand:child", WorkspacePath: kubernetesWorkspacePath, Port: 1234}
	require.False(t, validKubernetesRecoveredInstance(info, record, recorded))
	info.ErrandMCPOwner = errandMCPOwner(owner)
	require.True(t, validKubernetesRecoveredInstance(info, record, recorded))
	wrong := owner
	wrong.Generation++
	info.ErrandMCPOwner = errandMCPOwner(wrong)
	require.False(t, validKubernetesRecoveredInstance(info, record, recorded))
	record.Metadata[runExecutionOwnerMetadataKey] = map[string]interface{}{"Kind": "errand", "ErrandID": "child"}
	require.False(t, validKubernetesRecoveredInstance(info, record, recorded))
}
