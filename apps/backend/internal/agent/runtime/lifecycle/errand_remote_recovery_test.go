package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDeferredStartupAdoptsRemoteDockerErrandJournal(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	owner := ExecutionOwner{
		Kind: ExecutionOwnerErrand, WorkspaceID: "workspace", TaskID: "task", ErrandID: "errand", Generation: 1,
		AgentProfileID: "profile-1",
	}
	row := remoteErrandRecoveryTestInventory(owner)
	repo, _ := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: owner.TaskID, Title: "task"}))
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{
		ID: owner.ErrandID, TaskID: owner.TaskID, ParentSessionID: "ordinary-parent",
		Lifecycle: models.ErrandLifecycleLaunched, ResultState: models.ErrandResultNone,
	}))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, owner.ErrandID, 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, owner.ErrandID, owner.Generation, row.AgentExecutionID)
	require.NoError(t, err)
	require.True(t, bound)
	binding := &models.ErrandHarnessBinding{
		ErrandID: owner.ErrandID, Generation: owner.Generation, ExecutionID: row.AgentExecutionID,
		IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "errand-stream", NativeSessionID: "native",
	}
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, row))

	peer := newMockAgentServer(t)
	t.Cleanup(peer.Close)
	peer.deliveryStatus = &agentctl.DeliveryStatus{
		StorageCapability: journal.StorageCapability{Durable: true, Version: journal.CurrentProtocolVersion},
		SessionID:         row.SessionID, IncarnationID: binding.IncarnationID,
		HarnessGeneration: uint64(binding.HarnessGeneration), StreamID: binding.StreamID,
	}
	var client *agentctl.Client
	workspacePath := "/foreign-workspace"
	backend := &task08DetailedRecoveryBackend{
		MockExecutor: &MockExecutor{name: executor.NameRemoteDocker},
		recoveryResult: func([]*models.ExecutorRunning) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
			client = createTestClient(t, peer.server.URL)
			t.Cleanup(client.Close)
			return []*ExecutorInstance{{
				InstanceID: row.AgentExecutionID, SessionID: row.SessionID, AgentProfileID: row.ExecutionProfileID,
				RuntimeName: row.Runtime, ContainerID: row.ContainerID, WorkspacePath: workspacePath,
				recoveredErrandOwner: owner, ProviderSessionID: binding.NativeSessionID, Client: client,
			}}, nil, nil
		},
	}
	manager, _ := task08ManagerWithBackend(t, backend)
	manager.SetExecutorRunningWriter(repo)
	manager.SetAgentDeliveryRepository(repo)
	require.NoError(t, manager.PrepareStartupRecovery(ctx))
	pending := manager.pendingRemoteRecoveryForSession(row.SessionID)
	require.NotNil(t, pending)

	manager.retryRemoteRecovery(ctx, pending)
	_, tracked := manager.GetExecution(row.AgentExecutionID)
	require.False(t, tracked)
	require.Same(t, pending, manager.pendingRemoteRecoveryForSession(row.SessionID))

	workspacePath = row.WorktreePath
	manager.retryRemoteRecovery(ctx, pending)

	execution, tracked := manager.GetExecution(row.AgentExecutionID)
	require.True(t, tracked)
	require.Equal(t, owner, execution.Owner)
	require.Empty(t, execution.TaskID)
	require.Empty(t, execution.SessionID)
	require.Equal(t, binding.NativeSessionID, execution.ACPSessionID)
	require.Equal(t, DurableDeliveryV1, execution.DeliveryMode)
	require.True(t, client.HasAgentStream())
	require.Nil(t, manager.pendingRemoteRecoveryForSession(row.SessionID))
	require.NoError(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID))
	require.NotContains(t, peer.getActionLog(), "agent.initialize")
	require.NotContains(t, peer.getActionLog(), "agent.prompt")
}

func TestPendingRemoteDockerErrandStopChecksItsExactOwner(t *testing.T) {
	ctx := t.Context()
	owner := ExecutionOwner{Kind: ExecutionOwnerErrand, WorkspaceID: "workspace", TaskID: "task", ErrandID: "errand", Generation: 1}
	row := remoteErrandRecoveryTestInventory(owner)
	repo, _ := runOwnerTestRepository(t)
	require.NoError(t, repo.UpsertExecutorRunning(ctx, row))
	backend := &exactErrandRecoveryExecutor{
		runOwnerRecoveryExecutor: runOwnerRecoveryExecutor{MockExecutor: MockExecutor{name: executor.NameRemoteDocker}},
	}
	manager, _ := task08ManagerWithBackend(t, backend)
	manager.SetExecutorRunningWriter(repo)
	require.NoError(t, manager.PrepareStartupRecovery(ctx))
	require.NotNil(t, manager.pendingRemoteRecoveryForSession(row.SessionID))

	wrong := owner
	wrong.Generation++
	require.ErrorIs(t, manager.StopErrandOwnerForRecovery(ctx, wrong, row.AgentExecutionID), models.ErrExecutionRotated)
	require.Empty(t, backend.stoppedID)
	require.ErrorIs(t, manager.recoveryGuard.CheckLaunchAllowed(row.SessionID), ErrSessionRecoveryGuarded)
	require.NoError(t, manager.StopErrandOwnerForRecovery(ctx, owner, row.AgentExecutionID))
	require.Equal(t, row.AgentExecutionID, backend.stoppedID)
	require.Nil(t, manager.pendingRemoteRecoveryForSession(row.SessionID))
	saved, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusStopped, saved.Status)
}

func remoteErrandRecoveryTestInventory(owner ExecutionOwner) *models.ExecutorRunning {
	return &models.ExecutorRunning{
		SessionID: models.ErrandExecutionIdentity(owner.ErrandID), AgentExecutionID: "errand-execution",
		Runtime: agentruntime.RuntimeRemoteDocker, Status: models.ExecutorRunningStatusRunning,
		ExecutionProfileID: owner.AgentProfileID, ContainerID: "retained-container", WorktreePath: "/workspace",
		Metadata: map[string]interface{}{runExecutionOwnerMetadataKey: owner},
	}
}
