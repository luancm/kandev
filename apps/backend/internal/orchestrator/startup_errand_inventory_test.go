package orchestrator

import (
	"testing"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestStartupTaskSessionCleanupLeavesOwnedErrandRuntime(t *testing.T) {
	ctx := t.Context()
	repo := setupTestRepo(t)
	manager := &mockAgentManager{}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	owned := &models.ExecutorRunning{
		ID: "owned-runtime", SessionID: models.ErrandExecutionIdentity("owned"), AgentExecutionID: "owned-execution",
		Status: models.ExecutorRunningStatusRunning, Metadata: map[string]any{
			"execution_owner": runtimeapi.ExecutionOwner{
				Kind: runtimeapi.ExecutionOwnerErrand, TaskID: "task", WorkspaceID: "workspace", ErrandID: "owned", Generation: 1,
			},
		},
	}
	unowned := &models.ExecutorRunning{
		ID: "ordinary-runtime", SessionID: models.ErrandExecutionIdentity("ordinary"),
		TaskID: "ordinary-task", AgentExecutionID: "ordinary-execution", Status: models.ExecutorRunningStatusRunning,
	}
	require.NoError(t, repo.UpsertExecutorRunning(ctx, owned))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, unowned))

	service.reconcileSessionsOnStartup(ctx)

	manager.mu.Lock()
	calls := append([]stopAgentCall(nil), manager.stopAgentWithReasonArgs...)
	manager.mu.Unlock()
	require.Equal(t, []stopAgentCall{{
		ExecutionID: unowned.AgentExecutionID, Reason: "startup missing session cleanup", Force: true,
	}}, calls)
	row, err := repo.GetExecutorRunningBySessionID(ctx, owned.SessionID)
	require.NoError(t, err)
	require.Equal(t, owned.AgentExecutionID, row.AgentExecutionID)
	require.Equal(t, models.ExecutorRunningStatusRunning, row.Status)
}
