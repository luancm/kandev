package lifecycle

import (
	"context"
	"testing"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestKubernetesErrandTeardownKeepsMissingPodUnresolved(t *testing.T) {
	f := newTaskPodFixture(t)
	f.launch(t, 1)
	child := f.launch(t, 2)
	owner := agentctltypes.ErrandMCPOwner{TaskID: "task-1", ErrandID: "child", Generation: 3}
	row := &models.ExecutorRunning{
		AgentExecutionID: child.InstanceID, SessionID: models.ErrandExecutionIdentity(owner.ErrandID),
		Runtime: f.runtime.Name(), Metadata: child.Metadata,
	}
	f.resources.mu.Lock()
	f.resources.pod = nil
	f.resources.mu.Unlock()

	require.Error(t, f.runtime.stopErrandInstance(t.Context(), row, owner),
		"the API can lose the Pod object before the recorded instance stops")
	_, active, _ := f.control.snapshot()
	require.Len(t, active, 2, "an unresolved inspection must not mutate the active instances")
	require.Empty(t, f.resources.deletedPods)
	require.Empty(t, f.resources.deletedPVCs)
}

func TestKubernetesErrandTeardownPreservesParentPodAndClaim(t *testing.T) {
	f := newTaskPodFixture(t)
	parent := f.launch(t, 1)
	child := f.launch(t, 2)
	owner := agentctltypes.ErrandMCPOwner{TaskID: "task-1", ErrandID: "child", Generation: 3}
	remoteID := getMetadataString(child.Metadata, MetadataKeyKubernetesAgentctlInstanceID)
	f.control.mu.Lock()
	got := f.control.active[remoteID]
	got.ErrandMCPOwner = &owner
	f.control.active[remoteID] = got
	f.control.mu.Unlock()
	row := &models.ExecutorRunning{
		AgentExecutionID: child.InstanceID, SessionID: "errand:child",
		Runtime: f.runtime.Name(), Metadata: child.Metadata,
	}
	exact, ok := any(f.runtime).(interface {
		stopErrandInstance(context.Context, *models.ExecutorRunning, agentctltypes.ErrandMCPOwner) error
	})
	require.True(t, ok, "shared Pod teardown must support exact owned stop")

	foreign := owner
	foreign.Generation++
	require.Error(t, exact.stopErrandInstance(t.Context(), row, foreign))
	_, active, _ := f.control.snapshot()
	require.Contains(t, active, remoteID)
	changedPod := *row
	changedPod.Metadata = cloneKubernetesMetadata(row.Metadata)
	changedPod.Metadata[MetadataKeyKubernetesPodUID] = "other-pod-uid"
	require.Error(t, exact.stopErrandInstance(t.Context(), &changedPod, owner))
	_, active, _ = f.control.snapshot()
	require.Contains(t, active, remoteID)

	require.NoError(t, exact.stopErrandInstance(t.Context(), row, owner))
	_, active, _ = f.control.snapshot()
	require.NotContains(t, active, remoteID)
	require.Contains(t, active, getMetadataString(parent.Metadata, MetadataKeyKubernetesAgentctlInstanceID))
	require.Empty(t, f.resources.deletedPods)
	require.Empty(t, f.resources.deletedPVCs)
	require.NoError(t, parent.Client.Health(t.Context()))
}
