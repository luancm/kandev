package lifecycle

import (
	"errors"
	"sync/atomic"
	"testing"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestKubernetesErrandRequiresSharedEnvironmentStore(t *testing.T) {
	runtime := newFakeKubernetesExecutor(t, &fakeKubernetesResources{}, &recordingKubernetesExec{}, nil)
	providerAccessed := false
	runtime.clientFactory = func(kubeexecutor.ExecutorConfig) (*kubernetesRuntimeClient, error) {
		providerAccessed = true
		return nil, errors.New("unexpected Kubernetes access")
	}
	req := validKubernetesCreateRequest()
	req.WorkspaceReuseRequired = true
	req.EnvironmentOwnershipGeneration = 1
	req.ErrandMCPOwner = errandMCPOwner(ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task-1", ErrandID: "errand", Generation: 1})
	_, err := runtime.CreateInstance(t.Context(), req)
	require.ErrorIs(t, err, models.ErrWorkspaceReuseUnsafe)
	require.False(t, providerAccessed)
}

func TestKubernetesErrandCheckpointPersistsTheExactOwner(t *testing.T) {
	repo, database := runOwnerTestRepository(t)
	manager := newTestManager(t)
	manager.SetExecutorRunningWriter(repo)
	owner := ExecutionOwner{
		Kind: ExecutionOwnerErrand, TaskID: "owning-task", WorkspaceID: "workspace",
		ErrandID: "errand", Generation: 2, AgentProfileID: "agent",
	}
	req := validKubernetesCreateRequest()
	req.TaskID = "workspace-owner"
	req.SessionID = models.ErrandExecutionIdentity(owner.ErrandID)
	req.WorkspaceID = owner.WorkspaceID
	req.AgentProfileID = owner.AgentProfileID
	req.OfficeAgentProfileID = owner.AgentProfileID
	req.ErrandMCPOwner = errandMCPOwner(owner)
	require.NoError(t, manager.checkpointKubernetesRuntimeInventory(t.Context(), req, map[string]interface{}{
		MetadataKeyKubernetesInventoryState: KubernetesInventoryStateReady,
	}))
	row, err := repo.GetExecutorRunningBySessionID(t.Context(), req.SessionID)
	require.NoError(t, err)
	require.Empty(t, row.TaskID)
	require.NoError(t, validateRunRecoveryOwner(row, owner))
	require.Equal(t, req.InstanceID, row.AgentExecutionID)
	var sessions int
	require.NoError(t, database.Get(&sessions, "SELECT COUNT(*) FROM task_sessions"))
	require.Zero(t, sessions)
}

func TestKubernetesErrandAttachesToInheritedPodAndStopsOnlyItsInstance(t *testing.T) {
	f := newTaskPodFixture(t)
	parent := f.launch(t, 1)
	owner := ExecutionOwner{
		Kind: ExecutionOwnerErrand, TaskID: "descendant-task", WorkspaceID: "workspace",
		ErrandID: "owned-errand", Generation: 1, AgentProfileID: "agent",
	}
	req := taskPodRequest(2)
	req.SessionID = models.ErrandExecutionIdentity(owner.ErrandID)
	req.ErrandMCPOwner = errandMCPOwner(owner)
	req.EnvironmentOwnershipGeneration = 2
	_, err := f.runtime.CreateInstance(t.Context(), req)
	require.ErrorIs(t, err, models.ErrWorkspaceReuseUnsafe)
	req.EnvironmentOwnershipGeneration = 1
	child, err := f.runtime.CreateInstance(t.Context(), req)
	require.NoError(t, err)
	require.Len(t, f.resources.createdPods, 1)
	require.Len(t, f.resources.createdPVCs, 1)
	remoteID := getMetadataString(child.Metadata, MetadataKeyKubernetesAgentctlInstanceID)
	_, active, _ := f.control.snapshot()
	require.Equal(t, req.ErrandMCPOwner, active[remoteID].ErrandMCPOwner)
	require.NotContains(t, kubernetesSessionEnvironment(req), kubernetesEnvTaskID)
	require.NotContains(t, kubernetesSessionEnvironment(req), kubernetesEnvSessionID)
	row := &models.ExecutorRunning{
		AgentExecutionID: child.InstanceID, SessionID: req.SessionID, Runtime: child.RuntimeName, Metadata: child.Metadata,
	}
	foreign := *req.ErrandMCPOwner
	foreign.TaskID = "foreign-task"
	require.Error(t, f.runtime.stopErrandInstance(t.Context(), row, foreign))
	require.NoError(t, f.runtime.stopErrandInstance(t.Context(), row, *req.ErrandMCPOwner))
	_, active, _ = f.control.snapshot()
	require.NotContains(t, active, remoteID)
	require.Contains(t, active, getMetadataString(parent.Metadata, MetadataKeyKubernetesAgentctlInstanceID))
	require.Empty(t, f.resources.deletedPods)
	require.Empty(t, f.resources.deletedPVCs)
}

func TestKubernetesErrandRollbackRequiresExactStop(t *testing.T) {
	for _, stage := range []string{"forward", "secret persistence"} {
		t.Run(stage, func(t *testing.T) {
			f := newTaskPodFixture(t)
			f.launch(t, 1)
			runtime := f.runtime
			if stage == "forward" {
				runtime = newFakeKubernetesExecutor(t, f.resources, f.execs, map[uint16]uint16{
					uint16(kubeexecutor.DefaultAgentctlPort): f.port,
				})
				runtime.environmentStore, runtime.secretStore = f.repo, f.secretStore
				t.Cleanup(func() { require.NoError(t, runtime.Close()) })
			} else {
				runtime.secretStore = taskPodFailedSecrets{f.secretStore}
			}
			var ordinaryDeletes atomic.Int64
			f.control.beforeDelete = func() { ordinaryDeletes.Add(1) }
			req := taskPodRequest(2)
			req.SessionID = models.ErrandExecutionIdentity("errand")
			req.EnvironmentOwnershipGeneration = 1
			req.ErrandMCPOwner = errandMCPOwner(ExecutionOwner{
				Kind: ExecutionOwnerErrand, TaskID: "task-1", ErrandID: "errand", Generation: 1,
			})
			_, err := runtime.CreateInstance(t.Context(), req)
			require.Error(t, err)
			_, active, _ := f.control.snapshot()
			require.NotContains(t, active, req.InstanceID)
			require.Contains(t, active, "instance-1")
			require.Zero(t, ordinaryDeletes.Load(), "the rollback must ask the helper to confirm the exact Errand")
			require.Empty(t, f.resources.deletedPods)
			require.Empty(t, f.resources.deletedPVCs)
		})
	}
}
