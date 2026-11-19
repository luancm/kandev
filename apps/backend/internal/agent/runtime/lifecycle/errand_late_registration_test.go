package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type refusedErrandRegistration struct{ err error }

func (r refusedErrandRegistration) AdmitExecution(context.Context, ExecutionOwner) error {
	return r.err
}

func TestErrandPostAllocationRefusalRetainsExactCleanupTarget(t *testing.T) {
	repo, _ := runOwnerTestRepository(t)
	manager := newTestManager(t)
	manager.SetExecutorRunningWriter(repo)
	refused := errors.New("errand cancelled after allocation")
	backend := &errandLaunchRollbackBackend{
		MockExecutor: MockExecutor{name: executor.NameDocker},
		stopErr:      errors.New("runtime remains unconfirmed"),
	}
	owner := ExecutionOwner{Kind: ExecutionOwnerErrand, WorkspaceID: "ws", TaskID: "task", ErrandID: "child", Generation: 1}
	execution := &AgentExecution{
		ID: "execution", Owner: owner, OwnerAdmission: refusedErrandRegistration{refused},
		WorkspaceID: owner.WorkspaceID, Status: v1.AgentStatusStarting, RuntimeName: executor.NameDocker,
		ContainerID: "retained-parent-container",
	}
	instance := &ExecutorInstance{
		InstanceID: execution.ID, ContainerID: execution.ContainerID, RuntimeName: executor.NameDocker,
		SessionID: "errand:child",
	}
	err := manager.registerAndPublishExecution(t.Context(), execution, backend, instance, "")
	require.ErrorIs(t, err, refused)
	row, err := repo.GetExecutorRunningBySessionID(t.Context(), "errand:child")
	require.NoError(t, err, "cancellation must have an exact provider target after incomplete rollback")
	require.Equal(t, execution.ID, row.AgentExecutionID)
	require.Equal(t, execution.ContainerID, row.ContainerID)
	require.Empty(t, row.TaskID)
}

func TestErrandLaunchKeepsJournalOwnersSeparateInTheSameWorkspace(t *testing.T) {
	manager := newTestManager(t)
	manager.dataDir = t.TempDir()
	backend := &createInstanceExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
	manager.executorRegistry = NewExecutorRegistry(manager.logger)
	manager.executorRegistry.Register(backend)
	request := &LaunchRequest{
		WorkspaceID: "ws", TaskEnvironmentID: "parent-environment", WorkspacePath: t.TempDir(),
		AgentProfileID: "profile-1", ExecutorType: "local_pc", OwnerAdmission: allowRunOwner{},
		Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, WorkspaceID: "ws", TaskID: "parent", ErrandID: "one", Generation: 1},
	}
	_, err := manager.Launch(t.Context(), request)
	require.NoError(t, err)
	first := backend.lastRequest.DurableJournalOwnerID
	request.Owner.ErrandID = "two"
	_, err = manager.Launch(t.Context(), request)
	require.NoError(t, err)
	require.NotEmpty(t, first)
	require.NotEqual(t, request.TaskEnvironmentID, first)
	require.NotEqual(t, first, backend.lastRequest.DurableJournalOwnerID,
		"two Errands sharing the same workspace must not contend for the same journal")
}
