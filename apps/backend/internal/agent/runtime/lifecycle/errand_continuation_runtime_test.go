package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestErrandContinuationRequiresExactStoppedPredecessor(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*LaunchRequest, *models.ExecutorRunning)
	}{
		{name: "valid"},
		{name: "rotated", change: func(_ *LaunchRequest, row *models.ExecutorRunning) { row.AgentExecutionID = "other" }},
		{name: "live", change: func(_ *LaunchRequest, row *models.ExecutorRunning) { row.Status = models.ExecutorRunningStatusReady }},
		{name: "different provider", change: func(_ *LaunchRequest, row *models.ExecutorRunning) { row.Runtime = executor.NameDocker }},
		{name: "different workspace", change: func(req *LaunchRequest, _ *models.ExecutorRunning) { req.WorkspacePath = "/other" }},
		{name: "different owner", change: func(req *LaunchRequest, _ *models.ExecutorRunning) { req.Owner.WorkspaceID = "other" }},
		{name: "wrong native ID", change: func(req *LaunchRequest, _ *models.ExecutorRunning) { req.ACPSessionID = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", WorkspaceID: "ws", ErrandID: "errand", Generation: 1}
			row := &models.ExecutorRunning{
				SessionID: models.ErrandExecutionIdentity(owner.ErrandID), AgentExecutionID: "old-execution",
				Runtime: executor.NameStandalone, Status: models.ExecutorRunningStatusStopped, WorktreePath: "/workspace",
				Metadata: map[string]any{
					runExecutionOwnerMetadataKey: owner, MetadataKeyOriginalWorkspacePath: "/original",
					MetadataKeyTaskEnvironmentID: "env",
				},
			}
			req := &LaunchRequest{
				Owner: owner, ExecutorType: string(models.ExecutorTypeWorktree), TaskEnvironmentID: "env",
				WorkspacePath: "/workspace", ACPSessionID: "native", RequiredNativeConversationID: "native",
				ErrandPredecessor: &models.ErrandTeardownTarget{
					TaskID: owner.TaskID, ErrandID: owner.ErrandID, OwnerGeneration: 1, ExecutionID: "old-execution",
				},
			}
			req.Owner.Generation++
			mgr := newTestManager(t)
			mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{prior: row})
			if tc.change != nil {
				tc.change(req, row)
				require.ErrorIs(t, mgr.validateColdLaunchRuntime(t.Context(), req), ErrExecutorIdentityUnverified)
				require.Empty(t, req.OriginalWorkspacePath)
				return
			}
			require.NoError(t, mgr.validateColdLaunchRuntime(t.Context(), req))
			require.Equal(t, "/original", req.OriginalWorkspacePath)
			require.Empty(t, req.SessionID)
		})
	}
}

func TestErrandContinuationPersistsSuccessorWithoutTaskSession(t *testing.T) {
	ctx := t.Context()
	repo, database := runOwnerTestRepository(t)
	manager := newTestManager(t)
	manager.SetExecutorRunningWriter(repo)
	manager.SetAgentDeliveryRepository(repo)
	owner := ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", WorkspaceID: "ws", ErrandID: "errand", Generation: 1}
	previous := &AgentExecution{ID: "predecessor", Owner: owner, Status: v1.AgentStatusRunning, RuntimeName: executor.NameStandalone}
	require.NoError(t, manager.persistExecutorRunningResult(ctx, previous))
	owner.Generation++
	next := &AgentExecution{ID: "successor", Owner: owner, Status: v1.AgentStatusStarting, RuntimeName: executor.NameStandalone}
	require.ErrorIs(t, manager.persistExecutorRunningResult(ctx, next), models.ErrExecutionRotated)
	require.NoError(t, repo.RepairExecutorRunningDead(ctx, models.ErrandExecutionIdentity(owner.ErrandID)))
	require.NoError(t, manager.persistExecutorRunningResult(ctx, next))
	require.ErrorIs(t, manager.persistExecutorRunningResult(ctx, previous), models.ErrExecutionRotated)
	row, err := repo.GetExecutorRunningBySessionID(ctx, models.ErrandExecutionIdentity(owner.ErrandID))
	require.NoError(t, err)
	require.Equal(t, next.ID, row.AgentExecutionID)
	require.NoError(t, validateRunRecoveryOwner(row, owner))
	var count int
	require.NoError(t, database.Get(&count, "SELECT COUNT(*) FROM task_sessions"))
	require.Zero(t, count)
}
