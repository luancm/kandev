package lifecycle

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestErrandOwnerKindIsNotInferredAsTask(t *testing.T) {
	var owner ExecutionOwner
	require.NoError(t, json.Unmarshal([]byte(`{"Kind":"errand","WorkspaceID":"ws","TaskID":"parent","ErrandID":"child","Generation":1}`), &owner))
	execution := &AgentExecution{TaskID: "parent", Owner: owner}
	require.Equal(t, ExecutionOwnerKind("errand"), executionOwnerKind(execution))
	payload := newAgentEventPayload(execution)
	require.Equal(t, "child", payload.ErrandID)
	require.EqualValues(t, 1, payload.ErrandGeneration)
}

func TestErrandOwnerRegistrationPreservesInventoryIdentity(t *testing.T) {
	ctx := context.Background()
	repo, database := runOwnerTestRepository(t)
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(repo)
	var owner ExecutionOwner
	require.NoError(t, json.Unmarshal([]byte(`{"Kind":"errand","WorkspaceID":"ws","TaskID":"parent","ErrandID":"child","Generation":1}`), &owner))
	execution := &AgentExecution{ID: "execution", WorkspaceID: "ws", Owner: owner, OwnerAdmission: allowRunOwner{}, Status: v1.AgentStatusRunning, RuntimeName: executor.NameStandalone}
	backend := &runOwnerRecoveryExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
	mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
	mgr.executorRegistry.Register(backend)
	require.NoError(t, mgr.registerAndPublishExecution(ctx, execution, backend, &ExecutorInstance{InstanceID: execution.ID}, ""))
	row, err := repo.GetExecutorRunningBySessionID(ctx, "errand:child")
	require.NoError(t, err)
	require.Equal(t, "execution", row.AgentExecutionID)
	require.Empty(t, execution.SessionID)
	require.Empty(t, row.TaskID)
	raw, err := json.Marshal(row.Metadata["execution_owner"])
	require.NoError(t, err)
	require.JSONEq(t, `{"Kind":"errand","WorkspaceID":"ws","TaskID":"parent","SessionID":"","ErrandID":"child","Generation":1,"RunID":"","RunSessionID":"","Attempt":0,"AgentProfileID":""}`, string(raw))
	var count int
	require.NoError(t, database.Get(&count, "SELECT COUNT(*) FROM task_sessions"))
	require.Zero(t, count)
}

func TestErrandOwnerRejectsInvalidIdentityAndMissingAdmission(t *testing.T) {
	mgr := newTestManager(t)
	for _, raw := range []string{
		`{"Kind":"errand","WorkspaceID":"ws","TaskID":"parent","ErrandID":"child","Generation":1}`,
		`{"Kind":"errand","WorkspaceID":"ws","TaskID":"parent"}`,
		`{"Kind":"errand","WorkspaceID":"ws","TaskID":"parent","ErrandID":"child","Generation":1,"SessionID":"task-session"}`,
	} {
		var owner ExecutionOwner
		require.NoError(t, json.Unmarshal([]byte(raw), &owner))
		require.Error(t, mgr.admitExecutionOwner(context.Background(), &LaunchRequest{Owner: owner}))
	}
	var malformed ExecutionOwner
	require.NoError(t, json.Unmarshal([]byte(`{"Kind":"errand","WorkspaceID":"ws","TaskID":"parent","ErrandID":"child","Generation":1,"RunID":"run"}`), &malformed))
	require.Error(t, mgr.admitExecutionOwner(context.Background(), &LaunchRequest{Owner: malformed, OwnerAdmission: allowRunOwner{}}))
}

func TestErrandStreamUsesNamespacedIdentity(t *testing.T) {
	require.Equal(t, "errand:child", agentStreamSubjectID(ExecutionOwnerKind("errand"), "", "", "child"))
}

func TestErrandRecoveryStopValidatesOwner(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(repo)
	owner := ExecutionOwner{Kind: ExecutionOwnerErrand, WorkspaceID: "ws", TaskID: "parent", ErrandID: "child", Generation: 1}
	execution := &AgentExecution{ID: "execution", WorkspaceID: "ws", Owner: owner, OwnerAdmission: allowRunOwner{}, Status: v1.AgentStatusRunning, RuntimeName: executor.NameStandalone}
	backend := &exactErrandRecoveryExecutor{runOwnerRecoveryExecutor: runOwnerRecoveryExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}}
	mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
	mgr.executorRegistry.Register(backend)
	require.NoError(t, mgr.registerAndPublishExecution(ctx, execution, backend, &ExecutorInstance{InstanceID: execution.ID}, ""))
	mgr.RemoveExecution(execution.ID)
	recovery, ok := any(mgr).(interface {
		StopErrandOwnerForRecovery(context.Context, ExecutionOwner, string) error
	})
	require.True(t, ok, "errand owner must support exact inventory recovery stop")
	wrong := owner
	wrong.Generation++
	require.Error(t, recovery.StopErrandOwnerForRecovery(ctx, wrong, execution.ID))
	require.ErrorIs(t, recovery.StopErrandOwnerForRecovery(ctx, owner, "foreign-execution"), models.ErrExecutionRotated)
	require.Empty(t, backend.stoppedID)
	require.NoError(t, recovery.StopErrandOwnerForRecovery(ctx, owner, execution.ID))
	require.Equal(t, "execution", backend.stoppedID)
	row, err := repo.GetExecutorRunningBySessionID(ctx, "errand:child")
	require.NoError(t, err)
	require.Equal(t, "stopped", row.Status)
}

func TestErrandLaunchRequiresAttachedWorkspace(t *testing.T) {
	mgr := newTestManager(t)
	mgr.dataDir = t.TempDir()
	backend := &createInstanceExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
	mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
	mgr.executorRegistry.Register(backend)
	req := &LaunchRequest{WorkspaceID: "ws", AgentProfileID: "profile-1", ExecutorType: "local_pc", Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, WorkspaceID: "ws", TaskID: "parent", ErrandID: "child", Generation: 1}, OwnerAdmission: allowRunOwner{}}
	_, err := mgr.Launch(context.Background(), req)
	require.Error(t, err)
	req.WorkspacePath = t.TempDir()
	execution, err := mgr.Launch(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, req.WorkspacePath, execution.WorkspacePath)
	require.Empty(t, execution.SessionID)
	require.Empty(t, execution.TaskID)
	require.Equal(t, "errand:child", backend.lastRequest.SessionID)
	require.True(t, backend.lastRequest.WorkspaceReuseRequired)
}

func TestErrandExecutionStoreIndexesAndRejectsDuplicateOwners(t *testing.T) {
	store := NewExecutionStore()
	one := &AgentExecution{ID: "one", Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, ErrandID: "child-1"}}
	two := &AgentExecution{ID: "two", Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, ErrandID: "child-2"}}
	duplicate := &AgentExecution{ID: "duplicate", Owner: one.Owner}
	require.NoError(t, store.Add(one))
	require.NoError(t, store.Add(two))
	require.ErrorIs(t, store.Add(duplicate), ErrExecutionAlreadyExistsForSession)
	got, ok := store.GetBySessionID("errand:child-1")
	require.True(t, ok)
	require.Same(t, one, got)
	store.Remove("one")
	got, ok = store.GetBySessionID("errand:child-2")
	require.True(t, ok)
	require.Same(t, two, got)
}

func TestErrandOwnerRequiresReservedGeneration(t *testing.T) {
	mgr := newTestManager(t)
	owner := ExecutionOwner{Kind: ExecutionOwnerErrand, WorkspaceID: "ws", TaskID: "parent", ErrandID: "child"}
	require.Error(t, mgr.admitExecutionOwner(context.Background(), &LaunchRequest{Owner: owner, OwnerAdmission: allowRunOwner{}}))
}

type exactErrandRecoveryExecutor struct {
	runOwnerRecoveryExecutor
}

func (b *exactErrandRecoveryExecutor) stopErrandInstance(_ context.Context, row *models.ExecutorRunning, _ agentctltypes.ErrandMCPOwner) error {
	b.stoppedID = row.AgentExecutionID
	return b.stopErr
}

type replacingErrandExecutor struct {
	runOwnerRecoveryExecutor
	replace func()
}

func (b *replacingErrandExecutor) stopErrandInstance(_ context.Context, row *models.ExecutorRunning, _ agentctltypes.ErrandMCPOwner) error {
	b.replace()
	b.stoppedID = row.AgentExecutionID
	return b.stopErr
}

func TestErrandRecoveryStopDoesNotSettleSuccessor(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(repo)
	owner := ExecutionOwner{Kind: ExecutionOwnerErrand, WorkspaceID: "ws", TaskID: "parent", ErrandID: "child", Generation: 1}
	execution := &AgentExecution{ID: "old", WorkspaceID: "ws", Owner: owner, OwnerAdmission: allowRunOwner{}, Status: v1.AgentStatusRunning, RuntimeName: executor.NameStandalone}
	backend := &replacingErrandExecutor{runOwnerRecoveryExecutor: runOwnerRecoveryExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}}
	mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
	mgr.executorRegistry.Register(backend)
	require.NoError(t, mgr.registerAndPublishExecution(ctx, execution, backend, &ExecutorInstance{InstanceID: execution.ID}, ""))
	mgr.RemoveExecution(execution.ID)
	backend.replace = func() {
		row, err := repo.GetExecutorRunningBySessionID(ctx, "errand:child")
		require.NoError(t, err)
		row.AgentExecutionID = "successor"
		require.NoError(t, repo.UpsertExecutorRunning(ctx, row))
	}
	_ = mgr.StopErrandOwnerForRecovery(ctx, owner, execution.ID)
	row, err := repo.GetExecutorRunningBySessionID(ctx, "errand:child")
	require.NoError(t, err)
	require.Equal(t, "successor", row.AgentExecutionID)
	require.NotEqual(t, "stopped", row.Status)
	require.Equal(t, "old", backend.stoppedID)
}

func TestErrandWorktreeExecutorAttachesWithoutPreparingWorktree(t *testing.T) {
	mgr := newTestManager(t)
	mgr.preparerRegistry = NewPreparerRegistry(mgr.logger)
	mgr.preparerRegistry.Register(models.ExecutorTypeLocal, NewLocalPreparer(mgr.logger))
	mgr.preparerRegistry.Register(models.ExecutorTypeWorktree, NewWorktreePreparer(nil, mgr.logger))
	workspace := initGitRepo(t)
	marker := filepath.Join(workspace, "setup-ran")
	req := &LaunchRequest{ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: workspace, RepositoryPath: workspace, WorkspaceReuseRequired: true, Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, ErrandID: "child", Generation: 1}, CheckoutBranch: "must-not-exist", SetupScript: "touch " + marker}
	var result *EnvPrepareResult
	require.NotPanics(t, func() { result = mgr.runEnvironmentPreparerWithProgress(context.Background(), req, workspace, nil) })
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Equal(t, workspace, result.WorkspacePath)
	require.NoFileExists(t, marker)
}

func TestErrandRecoveryRejectsSharedAndUnconfirmedRuntimes(t *testing.T) {
	for _, runtimeName := range []executor.Name{executor.NameDocker, executor.NameSprites, executor.NamePluginRemote} {
		t.Run(string(runtimeName), func(t *testing.T) {
			ctx := context.Background()
			repo, _ := runOwnerTestRepository(t)
			mgr := newTestManager(t)
			mgr.SetExecutorRunningWriter(repo)
			owner := ExecutionOwner{Kind: ExecutionOwnerErrand, WorkspaceID: "ws", TaskID: "parent", ErrandID: "child", Generation: 1}
			execution := &AgentExecution{ID: "old", WorkspaceID: "ws", Owner: owner, OwnerAdmission: allowRunOwner{}, Status: v1.AgentStatusRunning, RuntimeName: runtimeName}
			backend := &runOwnerRecoveryExecutor{MockExecutor: MockExecutor{name: runtimeName}}
			mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
			mgr.executorRegistry.Register(backend)
			require.NoError(t, mgr.registerAndPublishExecution(ctx, execution, backend, &ExecutorInstance{InstanceID: execution.ID}, ""))
			mgr.RemoveExecution(execution.ID)
			require.ErrorIs(t, mgr.StopErrandOwnerForRecovery(ctx, owner, execution.ID), ErrErrandRecoveryRequiresReconnect)
			require.Empty(t, backend.stoppedID)
			row, err := repo.GetExecutorRunningBySessionID(ctx, "errand:child")
			require.NoError(t, err)
			require.NotEqual(t, "stopped", row.Status)
		})
	}
}
