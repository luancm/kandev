package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type ownedErrandLaunchFixtureRuntime struct {
	runtimeapi.Runtime
	launch func(context.Context, runtimeapi.LaunchSpec) (runtimeapi.ExecutionRef, error)
	read   func(context.Context, string) (*runtimeapi.Execution, error)
	start  func(context.Context, string) error
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
			workspace := t.TempDir()
			adapter := NewErrandAdapter(f.svc)
			adapter.SetOwnedLaunchRuntime(ownedErrandLaunchFixtureRuntime{
				launch: func(context.Context, runtimeapi.LaunchSpec) (runtimeapi.ExecutionRef, error) {
					t.Fatal("an existing reservation must never dispatch another launch")
					return runtimeapi.ExecutionRef{}, nil
				},
			}, func(_ context.Context, taskID, parentID string) (*runtimeapi.WorkspaceInfo, error) {
				return &runtimeapi.WorkspaceInfo{TaskID: taskID, SessionID: parentID, WorkspaceID: "ws1",
					ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: workspace}, nil
			})
			result, err := adapter.LaunchErrand(t.Context(), req)
			require.Equal(t, tc.disposition, result.Disposition, "recovering launch: %v", err)
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

func seedOwnedLaunchReceipt(t *testing.T, f *errandLaunchFixture, req errands.LaunchRequest, state models.DeliverySubmissionState, attempt string, finish bool) {
	t.Helper()
	ctx := t.Context()
	_, err := f.repo.ReserveErrandExecutionGeneration(ctx, req.ErrandID, 0)
	require.NoError(t, err)
	bound, err := f.repo.BindErrandExecution(ctx, req.ErrandID, 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	binding := &models.ErrandHarnessBinding{
		ErrandID: req.ErrandID, Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation",
		HarnessGeneration: 1, StreamID: "stream", NativeSessionID: "native",
	}
	require.NoError(t, f.repo.AppendErrandHarnessBinding(ctx, binding, 0))
	require.NoError(t, prepareOwnedErrandFirstTurn(ctx, f.repo, req))
	payload := []byte(req.Objective)
	submission := &models.AgentDeliverySubmission{
		ID: req.SubmissionID, DispatchAttemptID: attempt, Payload: payload, PayloadHash: journal.SubmissionHash(payload), State: state,
	}
	admitted, err := f.repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{
		ErrandID: req.ErrandID, Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation",
		HarnessGeneration: 1, StreamID: "stream", NativeSessionID: "native",
		PromptGeneration: 1, TurnID: req.TurnID, Submission: submission,
	})
	require.NoError(t, err)
	require.True(t, admitted)
	if finish {
		completeOwnedLaunchReceipt(t, f, req)
	}
}

func completeOwnedLaunchReceipt(t *testing.T, f *errandLaunchFixture, req errands.LaunchRequest) {
	t.Helper()
	ctx := t.Context()
	payload, err := json.Marshal(streams.AgentEvent{
		Type: streams.EventTypeComplete, SessionID: "native", TurnID: req.TurnID, PromptGeneration: 1,
	})
	require.NoError(t, err)
	terminal := &models.AgentDeliveryEvent{
		SessionID: models.ErrandExecutionIdentity(req.ErrandID), ExecutionID: "execution", TaskID: req.TaskID,
		IncarnationID: "incarnation", HarnessGeneration: 1, OwnerGeneration: 1, StreamID: "stream",
		ExpectedStreamID: "stream", SubmissionID: req.SubmissionID, Sequence: 1, Terminal: true,
		EventType: streams.EventTypeComplete, Payload: payload,
	}
	received, err := f.repo.ReceiveAgentDeliveryEvent(ctx, terminal, 1)
	require.NoError(t, err)
	require.True(t, received)
	projected, err := f.repo.ProjectAgentDeliveryEvent(ctx, terminal, nil)
	require.NoError(t, err)
	require.True(t, projected)
	settled, err := f.repo.SettleAgentDeliveryTerminal(ctx, "stream", 1, models.DeliverySubmissionCompleted, time.Now())
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
	workspace := t.TempDir()
	var execution *runtimeapi.Execution
	started := false
	rt := ownedErrandLaunchFixtureRuntime{
		launch: func(ctx context.Context, spec runtimeapi.LaunchSpec) (runtimeapi.ExecutionRef, error) {
			require.Equal(t, req.AgentProfileID, spec.AgentProfileID)
			require.Equal(t, req.TaskID, spec.Owner.TaskID)
			require.Equal(t, req.ErrandID, spec.Owner.ErrandID)
			require.Empty(t, spec.Owner.SessionID)
			require.Equal(t, workspace, spec.Workspace.Path)
			require.NoError(t, spec.OwnerAdmission.AdmitExecution(ctx, spec.Owner))
			launch, ok := spec.Metadata["launch_request"].(*runtimeapi.LaunchRequest)
			require.True(t, ok)
			require.Empty(t, launch.TaskID)
			require.Empty(t, launch.SessionID)
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
