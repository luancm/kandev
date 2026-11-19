package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type ownedErrandLaunchFixtureRuntime struct {
	runtimeapi.Runtime
	launch func(context.Context, runtimeapi.LaunchSpec) (runtimeapi.ExecutionRef, error)
	read   func(context.Context, string) (*runtimeapi.Execution, error)
	start  func(context.Context, string) error
}

func TestErrandAdapterRequiresOwnedRuntime(t *testing.T) {
	f := newErrandLaunchFixture(t)
	req := f.request()
	result, err := NewErrandAdapter(f.svc).LaunchErrand(t.Context(), req)
	require.Error(t, err)
	require.Equal(t, errands.LaunchBlocked, result.Disposition)
	sessions, err := f.repo.ListTaskSessions(t.Context(), req.TaskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
}

func TestErrandAdapterRequiresTheFullReservedIdentity(t *testing.T) {
	f := newErrandLaunchFixture(t)
	adapter := NewErrandAdapter(f.svc)
	for name, mutate := range map[string]func(*errands.LaunchRequest){
		"no errand":     func(r *errands.LaunchRequest) { r.ErrandID = "" },
		"no task":       func(r *errands.LaunchRequest) { r.TaskID = "" },
		"no turn":       func(r *errands.LaunchRequest) { r.TurnID = "" },
		"no submission": func(r *errands.LaunchRequest) { r.SubmissionID = "" },
		"no profile":    func(r *errands.LaunchRequest) { r.AgentProfileID = "" },
		"no objective":  func(r *errands.LaunchRequest) { r.Objective = "  " },
		"no parent":     func(r *errands.LaunchRequest) { r.ParentSessionID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			request := f.request()
			mutate(&request)
			result, err := adapter.LaunchErrand(t.Context(), request)
			require.ErrorIs(t, err, models.ErrInvalidErrand)
			require.Equal(t, errands.LaunchBlocked, result.Disposition)
		})
	}
	sessions, err := f.repo.ListTaskSessions(t.Context(), f.taskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
}

func TestErrandAdapterAttachesDockerTaskEnvironment(t *testing.T) {
	for _, tc := range []ownedWorkspaceLaunchCase{
		{name: "local saved environment", provider: models.ExecutorTypeLocalDocker},
		{name: "local environment rotated", provider: models.ExecutorTypeLocalDocker, rotateWorkspace: true},
		{name: "remote saved environment and target", provider: models.ExecutorTypeRemoteDocker},
		{name: "remote target rotated", provider: models.ExecutorTypeRemoteDocker, rotateTarget: true},
	} {
		t.Run(tc.name, func(t *testing.T) { exerciseOwnedWorkspaceLaunch(t, tc) })
	}
}

func TestErrandAdapterAttachesSSHTaskEnvironment(t *testing.T) {
	for _, tc := range []ownedWorkspaceLaunchCase{
		{name: "saved directory and target", provider: models.ExecutorTypeSSH},
		{name: "directory changed", provider: models.ExecutorTypeSSH, rotateWorkspace: true},
		{name: "target changed", provider: models.ExecutorTypeSSH, rotateTarget: true},
	} {
		t.Run(tc.name, func(t *testing.T) { exerciseOwnedWorkspaceLaunch(t, tc) })
	}
}

func TestErrandAdapterAttachesSpritesTaskEnvironment(t *testing.T) {
	for _, tc := range []ownedWorkspaceLaunchCase{
		{name: "saved sandbox", provider: models.ExecutorTypeSprites},
		{name: "sandbox changed", provider: models.ExecutorTypeSprites, rotateWorkspace: true},
	} {
		t.Run(tc.name, func(t *testing.T) { exerciseOwnedWorkspaceLaunch(t, tc) })
	}
}

func TestErrandAdapterAttachesKubernetesTaskEnvironment(t *testing.T) {
	for _, tc := range []ownedWorkspaceLaunchCase{
		{name: "saved shared environment", provider: models.ExecutorTypeKubernetes},
		{name: "workspace changed", provider: models.ExecutorTypeKubernetes, rotateWorkspace: true},
	} {
		t.Run(tc.name, func(t *testing.T) { exerciseOwnedWorkspaceLaunch(t, tc) })
	}
}

type ownedWorkspaceLaunchCase struct {
	name            string
	provider        models.ExecutorType
	rotateWorkspace bool
	rotateTarget    bool
}

func exerciseOwnedWorkspaceLaunch(t *testing.T, tc ownedWorkspaceLaunchCase) {
	t.Helper()
	ctx := t.Context()
	f := newErrandLaunchFixture(t)
	req := f.request()
	executor := &models.Executor{
		ID: "saved-executor", Name: "Saved executor", Type: tc.provider, Status: models.ExecutorStatusActive,
		Config: map[string]string{
			lifecycle.MetadataKeySSHHost:            "workspace.example",
			lifecycle.MetadataKeySSHUser:            "workspace-user",
			lifecycle.MetadataKeySSHHostFingerprint: "SHA256:saved-host",
			lifecycle.MetadataKeySSHRemoteTaskDir:   "/unrelated-task",
		},
	}
	require.NoError(t, f.repo.CreateExecutor(ctx, executor))
	env := &models.TaskEnvironment{
		ID: "shared-environment", TaskID: req.TaskID, OwnershipGeneration: 3,
		ExecutorType: string(tc.provider), ExecutorID: executor.ID, Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: "/workspace",
	}
	switch tc.provider {
	case models.ExecutorTypeLocalDocker, models.ExecutorTypeRemoteDocker:
		env.ContainerID = "environment-container"
		env.ContainerControlAuthTokenSecretID = "environment-control-secret"
		env.ContainerBootstrapNonceSecretID = "environment-bootstrap-secret"
	case models.ExecutorTypeSprites, models.ExecutorTypeKubernetes:
		profile := &models.ExecutorProfile{ID: "saved-profile", ExecutorID: executor.ID, Name: "Saved profile"}
		require.NoError(t, f.repo.CreateExecutorProfile(ctx, profile))
		env.ExecutorProfileID = profile.ID
		if tc.provider == models.ExecutorTypeSprites {
			env.SandboxID = "saved-sandbox"
		} else {
			env.TaskID = "workspace-owner"
			require.NoError(t, f.repo.CreateTask(ctx, &models.Task{ID: env.TaskID, WorkspaceID: "ws1", Title: "Inherited workspace"}))
		}
	}
	require.NoError(t, f.repo.CreateTaskEnvironment(ctx, env))
	var launch *runtimeapi.LaunchRequest
	var execution *runtimeapi.Execution
	adapter := NewErrandAdapter(f.svc)
	adapter.SetOwnedLaunchRuntime(ownedErrandLaunchFixtureRuntime{
		launch: func(ctx context.Context, spec runtimeapi.LaunchSpec) (runtimeapi.ExecutionRef, error) {
			launch = spec.Metadata["launch_request"].(*runtimeapi.LaunchRequest)
			if tc.rotateWorkspace {
				rotated := *env
				switch tc.provider {
				case models.ExecutorTypeSSH, models.ExecutorTypeKubernetes:
					rotated.WorkspacePath = "/changed-workspace"
				case models.ExecutorTypeSprites:
					rotated.SandboxID = "changed-sandbox"
				default:
					rotated.ContainerID = "rotated-container"
				}
				require.NoError(t, f.repo.UpdateTaskEnvironment(ctx, &rotated))
			}
			if tc.rotateTarget {
				executor.Config[lifecycle.MetadataKeySSHHost] = "different.example"
				require.NoError(t, f.repo.UpdateExecutor(ctx, executor))
			}
			if err := spec.OwnerAdmission.AdmitExecution(ctx, spec.Owner); err != nil {
				return runtimeapi.ExecutionRef{}, err
			}
			require.NoError(t, launch.OnExecutionAllocated(ctx, launch.CandidateExecutionID))
			execution = &runtimeapi.Execution{ID: launch.CandidateExecutionID, Owner: spec.Owner}
			return runtimeapi.ExecutionRef{ID: execution.ID}, nil
		},
		read:  func(context.Context, string) (*runtimeapi.Execution, error) { return execution, nil },
		start: func(context.Context, string) error { return nil },
	}, func(_ context.Context, taskID, parentID string) (*runtimeapi.WorkspaceInfo, error) {
		return &runtimeapi.WorkspaceInfo{
			TaskID: taskID, SessionID: parentID, WorkspaceID: "ws1", ExecutorType: env.ExecutorType, ExecutorProfileID: "parent-runtime-profile",
			WorkspacePath: "/parent-runtime-directory", TaskEnvironmentID: env.ID, EnvironmentOwnerTaskID: env.TaskID,
			OwnershipGeneration: env.OwnershipGeneration, AgentExecutionID: "parent-execution",
			Metadata: map[string]any{
				lifecycle.MetadataKeyContainerID:                "parent-instance-container",
				lifecycle.MetadataKeyContainerControlAuthSecret: "parent-runtime-control-secret",
				lifecycle.MetadataKeyBootstrapNonceSecret:       "parent-runtime-bootstrap-secret",
				lifecycle.MetadataKeyAuthTokenSecret:            "parent-instance-secret",
				lifecycle.MetadataKeySSHHost:                    "parent-runtime.example",
				lifecycle.MetadataKeySSHRemoteTaskDir:           "/parent-runtime-directory",
				lifecycle.MetadataKeySpriteName:                 "parent-runtime-sandbox",
				"executor_id":                                   "parent-runtime-executor",
			},
		}, nil
	})
	result, err := adapter.LaunchErrand(ctx, req)
	require.NotNil(t, launch, "saved environment must follow the owned launch path")
	if tc.rotateWorkspace || tc.rotateTarget {
		require.Equal(t, errands.LaunchBlocked, result.Disposition)
		require.ErrorIs(t, err, models.ErrWorkspaceReuseUnsafe)
		require.Nil(t, execution)
		return
	}
	require.Equal(t, errands.LaunchAdmitted, result.Disposition, "launch: %v", err)
	require.NoError(t, err)
	require.True(t, launch.WorkspaceReuseRequired)
	require.Equal(t, env.ID, launch.TaskEnvironmentID)
	require.Equal(t, env.WorkspacePath, launch.WorkspacePath)
	switch tc.provider {
	case models.ExecutorTypeSSH:
		require.Equal(t, env.WorkspacePath, launch.Metadata[lifecycle.MetadataKeySSHRemoteTaskDir])
		require.NotContains(t, launch.Metadata, lifecycle.MetadataKeyContainerID)
	case models.ExecutorTypeSprites:
		require.Equal(t, env.SandboxID, launch.Metadata[lifecycle.MetadataKeySpriteName])
		require.Equal(t, env.ExecutorProfileID, launch.Metadata[lifecycle.MetadataKeyExecutorProfileID])
		require.NotContains(t, launch.Metadata, lifecycle.MetadataKeyContainerID)
	case models.ExecutorTypeKubernetes:
		require.Equal(t, env.ExecutorProfileID, launch.Metadata[lifecycle.MetadataKeyExecutorProfileID])
		require.NotContains(t, launch.Metadata, lifecycle.MetadataKeyKubernetesPodName)
		require.Equal(t, env.TaskID, launch.EnvironmentOwnerTaskID)
		require.Equal(t, env.OwnershipGeneration, launch.EnvironmentOwnershipGeneration)
	default:
		require.Equal(t, env.ContainerID, launch.Metadata[lifecycle.MetadataKeyContainerID])
		require.Equal(t, env.ContainerControlAuthTokenSecretID, launch.Metadata[lifecycle.MetadataKeyContainerControlAuthSecret])
		require.Equal(t, env.ContainerBootstrapNonceSecretID, launch.Metadata[lifecycle.MetadataKeyBootstrapNonceSecret])
	}
	require.Equal(t, executor.ID, launch.Metadata["executor_id"])
	if tc.provider == models.ExecutorTypeRemoteDocker || tc.provider == models.ExecutorTypeSSH {
		require.Equal(t, executor.Config[lifecycle.MetadataKeySSHHost], launch.ExecutorConfig[lifecycle.MetadataKeySSHHost])
		require.Equal(t, executor.Config[lifecycle.MetadataKeySSHUser], launch.ExecutorConfig[lifecycle.MetadataKeySSHUser])
		require.Equal(t, executor.Config[lifecycle.MetadataKeySSHHostFingerprint], launch.ExecutorConfig[lifecycle.MetadataKeySSHHostFingerprint])
		require.NotContains(t, launch.ExecutorConfig, lifecycle.MetadataKeySSHRemoteTaskDir)
	}
	require.NotContains(t, launch.Metadata, lifecycle.MetadataKeyAuthTokenSecret)
	require.Empty(t, launch.PreviousExecutionID)
	require.Empty(t, launch.TaskID)
	require.Empty(t, launch.SessionID)
	sessions, err := f.repo.ListTaskSessions(ctx, req.TaskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
}

type ownedLaunchRepository struct {
	*sqliterepo.Repository
	errandID string
}

func (r ownedLaunchRepository) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	if id == r.errandID {
		return nil, errors.New("errand identity is not in task session storage")
	}
	return r.Repository.GetTaskSession(ctx, id)
}

func (r ownedLaunchRepository) GetExecutorRunningBySessionID(ctx context.Context, id string) (*models.ExecutorRunning, error) {
	if id == r.errandID {
		return nil, errors.New("errand inventory requires its transport identity")
	}
	return r.Repository.GetExecutorRunningBySessionID(ctx, id)
}

func TestErrandAdapterRecoversOwnedLaunchReservation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		state       models.DeliverySubmissionState
		attemptID   string
		finish      bool
		disposition errands.LaunchDisposition
	}{
		{name: "accepted", state: models.DeliverySubmissionAccepted, attemptID: "execution", disposition: errands.LaunchAdmitted},
		{name: "terminal", state: models.DeliverySubmissionDispatching, attemptID: "execution", finish: true, disposition: errands.LaunchSettled},
		{name: "unknown", state: models.DeliverySubmissionDispatching, attemptID: "execution", disposition: errands.LaunchBlocked},
		{name: "different execution", state: models.DeliverySubmissionAccepted, attemptID: "foreign", disposition: errands.LaunchBlocked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newErrandLaunchFixture(t)
			req := f.request()
			seedOwnedLaunchReceipt(t, f, req, tc.state, tc.attemptID, tc.finish)
			f.svc.repo = ownedLaunchRepository{Repository: f.repo, errandID: req.ErrandID}
			workspaceReads := 0
			adapter := NewErrandAdapter(f.svc)
			adapter.SetOwnedLaunchRuntime(ownedErrandLaunchFixtureRuntime{
				launch: func(context.Context, runtimeapi.LaunchSpec) (runtimeapi.ExecutionRef, error) {
					t.Fatal("an existing reservation must never dispatch another launch")
					return runtimeapi.ExecutionRef{}, nil
				},
			}, func(context.Context, string, string) (*runtimeapi.WorkspaceInfo, error) {
				workspaceReads++
				return nil, errors.New("parent workspace has changed")
			})
			result, err := adapter.LaunchErrand(t.Context(), req)
			require.Equal(t, tc.disposition, result.Disposition, "recovering launch: %v", err)
			require.Zero(t, workspaceReads, "adjudicating the saved launch must not depend on a new workspace read")
			if tc.disposition == errands.LaunchBlocked {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, req.ErrandID, result.SessionID)
			require.Equal(t, req.TurnID, result.TurnID)
			require.Equal(t, req.SubmissionID, result.SubmissionID)
			sessions, err := f.repo.ListTaskSessions(t.Context(), req.TaskID)
			require.NoError(t, err)
			require.Len(t, sessions, 1)
		})
	}
}

func TestErrandAdapterRejectsFreshPluginWorkspaceWithoutChildSession(t *testing.T) {
	f := newErrandLaunchFixture(t)
	req := f.request()
	adapter := NewErrandAdapter(f.svc)
	adapter.SetOwnedLaunchRuntime(ownedErrandLaunchFixtureRuntime{
		launch: func(context.Context, runtimeapi.LaunchSpec) (runtimeapi.ExecutionRef, error) {
			t.Fatal("unsupported executor must never reach runtime launch")
			return runtimeapi.ExecutionRef{}, nil
		},
	}, func(_ context.Context, taskID, parentID string) (*runtimeapi.WorkspaceInfo, error) {
		return &runtimeapi.WorkspaceInfo{
			TaskID: taskID, SessionID: parentID, WorkspaceID: "ws1",
			ExecutorType: string(models.ExecutorTypePluginRemote), WorkspacePath: "/workspace",
		}, nil
	})
	result, err := adapter.LaunchErrand(t.Context(), req)
	require.Equal(t, errands.LaunchBlocked, result.Disposition)
	require.ErrorAs(t, err, new(*errands.ErrLaunchRejected))
	require.ErrorIs(t, err, models.ErrWorkspaceReuseUnsafe)
	sessions, err := f.repo.ListTaskSessions(t.Context(), req.TaskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Empty(t, f.manager.launchedWith)
}

func seedOwnedLaunchReceipt(t *testing.T, f *errandLaunchFixture, req errands.LaunchRequest, state models.DeliverySubmissionState, attempt string, finish bool) {
	t.Helper()
	ctx := t.Context()
	_, err := f.repo.ReserveErrandExecutionGeneration(ctx, req.ErrandID, 0)
	require.NoError(t, err)
	bound, err := f.repo.BindErrandExecution(ctx, req.ErrandID, 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	binding := prepareBoundOwnedLaunchReceipt(t, f.repo, req, 1, "execution", state, attempt)
	if finish {
		completeOwnedLaunchReceipt(t, f.repo, req, binding)
	}
}

func prepareBoundOwnedLaunchReceipt(t *testing.T, repo *sqliterepo.Repository, req errands.LaunchRequest, generation uint64, executionID string, state models.DeliverySubmissionState, attempt string) *models.ErrandHarnessBinding {
	t.Helper()
	ctx := t.Context()
	binding := &models.ErrandHarnessBinding{
		ErrandID: req.ErrandID, Generation: generation, ExecutionID: executionID, IncarnationID: "incarnation",
		HarnessGeneration: 1, StreamID: "stream", NativeSessionID: "native",
	}
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	require.NoError(t, prepareOwnedErrandFirstTurn(ctx, repo, req))
	payload := []byte(req.Objective)
	submission := &models.AgentDeliverySubmission{
		ID: req.SubmissionID, DispatchAttemptID: attempt, Payload: payload, PayloadHash: journal.SubmissionHash(payload), State: state,
	}
	admitted, err := repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{
		ErrandID: req.ErrandID, Generation: generation, ExecutionID: executionID, IncarnationID: binding.IncarnationID,
		HarnessGeneration: binding.HarnessGeneration, StreamID: binding.StreamID, NativeSessionID: binding.NativeSessionID,
		PromptGeneration: 1, TurnID: req.TurnID, Submission: submission,
	})
	require.NoError(t, err)
	require.True(t, admitted)
	return binding
}

func completeOwnedLaunchReceipt(t *testing.T, repo *sqliterepo.Repository, req errands.LaunchRequest, binding *models.ErrandHarnessBinding) {
	t.Helper()
	ctx := t.Context()
	payload, err := json.Marshal(streams.AgentEvent{
		Type: streams.EventTypeComplete, SessionID: binding.NativeSessionID, TurnID: req.TurnID, PromptGeneration: 1,
	})
	require.NoError(t, err)
	terminal := &models.AgentDeliveryEvent{
		SessionID: models.ErrandExecutionIdentity(req.ErrandID), ExecutionID: binding.ExecutionID, TaskID: req.TaskID,
		IncarnationID: binding.IncarnationID, HarnessGeneration: binding.HarnessGeneration, OwnerGeneration: int64(binding.Generation), StreamID: binding.StreamID,
		ExpectedStreamID: binding.StreamID, SubmissionID: req.SubmissionID, Sequence: 1, Terminal: true,
		EventType: streams.EventTypeComplete, Payload: payload,
	}
	received, err := repo.ReceiveAgentDeliveryEvent(ctx, terminal, binding.HarnessGeneration)
	require.NoError(t, err)
	require.True(t, received)
	projected, err := repo.ProjectAgentDeliveryEvent(ctx, terminal, nil)
	require.NoError(t, err)
	require.True(t, projected)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, binding.StreamID, terminal.Sequence, models.DeliverySubmissionCompleted, time.Now())
	require.NoError(t, err)
	require.True(t, settled)
}

func (r ownedErrandLaunchFixtureRuntime) Launch(ctx context.Context, spec runtimeapi.LaunchSpec) (runtimeapi.ExecutionRef, error) {
	return r.launch(ctx, spec)
}

func (r ownedErrandLaunchFixtureRuntime) GetExecution(ctx context.Context, id string) (*runtimeapi.Execution, error) {
	return r.read(ctx, id)
}

func (r ownedErrandLaunchFixtureRuntime) StartExecution(ctx context.Context, id string) error {
	return r.start(ctx, id)
}

func TestErrandAdapterLaunchesOwnedTurnWithoutChildSession(t *testing.T) {
	f := newErrandLaunchFixture(t)
	req := f.request()
	req.MCPServerIDs = []string{"saved-addition"}
	req.MCPExcludedServerIDs = []string{"saved-exclusion"}
	workspace := t.TempDir()
	var execution *runtimeapi.Execution
	started := false
	rt := ownedErrandLaunchFixtureRuntime{
		launch: func(ctx context.Context, spec runtimeapi.LaunchSpec) (runtimeapi.ExecutionRef, error) {
			require.Equal(t, req.AgentProfileID, spec.AgentProfileID)
			require.Contains(t, spec.Prompt, req.Objective)
			require.Contains(t, spec.Prompt, "errand of session "+req.ParentSessionID)
			require.NotContains(t, spec.Prompt, "step_complete_kandev")
			require.Equal(t, req.TaskID, spec.Owner.TaskID)
			require.Equal(t, req.ErrandID, spec.Owner.ErrandID)
			require.Empty(t, spec.Owner.SessionID)
			require.Equal(t, workspace, spec.Workspace.Path)
			require.NoError(t, spec.OwnerAdmission.AdmitExecution(ctx, spec.Owner))
			launch, ok := spec.Metadata["launch_request"].(*runtimeapi.LaunchRequest)
			require.True(t, ok)
			require.Empty(t, launch.TaskID)
			require.Empty(t, launch.SessionID)
			require.NotNil(t, launch.ErrandMCPSelection)
			require.Equal(t, req.MCPServerIDs, launch.ErrandMCPSelection.DefinitionIDs)
			require.Equal(t, req.MCPExcludedServerIDs, launch.ErrandMCPSelection.ExcludedDefinitionIDs)
			require.Empty(t, launch.PreviousExecutionID)
			require.Empty(t, launch.ACPSessionID)
			require.NotEqual(t, "parent-stream", launch.DeliveryStreamID)
			require.True(t, launch.WorkspaceReuseRequired)
			require.False(t, launch.UseWorktree, "attach the existing worktree")
			require.Equal(t, "existing-task-root", launch.TaskDirName)
			require.Equal(t, "parent-env", launch.TaskEnvironmentID)
			require.Len(t, launch.RepoSpecs(), 1)
			require.Equal(t, "source-repo", launch.RepoSpecs()[0].RepositoryID)
			require.Equal(t, "existing-checkout", launch.RepoSpecs()[0].WorktreeID)
			require.Equal(t, "existing-branch", launch.RepoSpecs()[0].CheckoutBranch)
			require.Equal(t, "host-executor", launch.Metadata["executor_id"])
			require.Equal(t, "host-profile", launch.Metadata[lifecycle.MetadataKeyExecutorProfileID])
			require.NotContains(t, launch.Metadata, "acp_session_id")
			require.Equal(t, req.TurnID, launch.TurnID)
			require.True(t, launch.StartAgent)
			require.NotEmpty(t, launch.CandidateExecutionID)
			require.NoError(t, launch.OnExecutionAllocated(ctx, launch.CandidateExecutionID))
			execution = &runtimeapi.Execution{ID: launch.CandidateExecutionID, Owner: spec.Owner, WorkspacePath: workspace}
			return runtimeapi.ExecutionRef{ID: execution.ID}, nil
		},
		read: func(_ context.Context, id string) (*runtimeapi.Execution, error) {
			require.NotNil(t, execution)
			require.Equal(t, execution.ID, id)
			return execution, nil
		},
		start: func(ctx context.Context, id string) error {
			require.NotNil(t, execution)
			require.Equal(t, execution.ID, id)
			descriptor, err := f.repo.GetErrandExecutionDescriptor(ctx, req.ErrandID)
			require.NoError(t, err)
			require.Equal(t, id, descriptor.ExecutionID)
			require.Equal(t, execution.Owner.Generation, descriptor.Generation)
			turn, err := f.repo.GetErrandTurn(ctx, req.ErrandID, req.TurnID)
			require.NoError(t, err)
			require.Empty(t, turn.TaskSessionID)
			started = true
			return nil
		},
	}
	adapter := NewErrandAdapter(f.svc)
	adapter.SetOwnedLaunchRuntime(rt, func(_ context.Context, taskID, parentID string) (*runtimeapi.WorkspaceInfo, error) {
		require.Equal(t, req.TaskID, taskID)
		require.Equal(t, req.ParentSessionID, parentID)
		return &runtimeapi.WorkspaceInfo{
			TaskID: taskID, SessionID: parentID, WorkspaceID: "ws1",
			WorkspacePath: workspace, ExecutorType: string(models.ExecutorTypeWorktree),
			TaskEnvironmentID: "parent-env", TaskDirName: "existing-task-root",
			AgentExecutionID: "parent-execution", ACPSessionID: "parent-native",
			DeliveryStreamID:  "parent-stream",
			ExecutorProfileID: "host-profile",
			Metadata:          map[string]any{"executor_id": "host-executor", "acp_session_id": "parent-native"},
			WorkspaceRepositories: []lifecycle.WorkspaceRepositorySpec{{
				RepositoryID: "source-repo", RepositoryPath: workspace,
				WorktreeID: "existing-checkout", WorktreePath: workspace, WorktreeBranch: "existing-branch",
			}},
		}, nil
	})

	result, err := adapter.LaunchErrand(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, errands.LaunchAdmitted, result.Disposition)
	sessions, err := f.repo.ListTaskSessions(t.Context(), req.TaskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1, "the owned conversation needs no child TaskSession")
	require.Equal(t, req.ParentSessionID, sessions[0].ID)
	require.True(t, started, "launch must start the exact bound Errand execution")
}
