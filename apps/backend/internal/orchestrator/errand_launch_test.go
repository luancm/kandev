package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// errandLaunchManager records the reserved identity the runtime receives.
type errandLaunchManager struct {
	*mockAgentManager
	captured     sync.Mutex
	bound        []string
	launchedWith []*executor.LaunchAgentRequest
}

func (m *errandLaunchManager) SetInitialDeliverySubmissionID(_ context.Context, _, submissionID string) error {
	m.captured.Lock()
	defer m.captured.Unlock()
	m.bound = append(m.bound, submissionID)
	return nil
}

type errandLaunchFixture struct {
	svc      *Service
	repo     *sqliterepo.Repository
	taskRepo *mockTaskRepo
	manager  *errandLaunchManager
	parent   string
	taskID   string
	// launchError is the task-owned launch error seeded before the errand starts.
	launchError models.TaskLaunchError
}

func newErrandLaunchFixture(t *testing.T) *errandLaunchFixture {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "parent-1", "step-1")
	require.NoError(t, repo.SetSessionPrimary(ctx, "parent-1"))
	parent, err := repo.GetTaskSession(ctx, "parent-1")
	require.NoError(t, err)
	parent.State = models.TaskSessionStateWaitingForInput
	parent.AgentProfileID = "parent-profile"
	require.NoError(t, repo.UpdateTaskSession(ctx, parent))

	// A task in TODO would be moved to SCHEDULING by an ordinary start.
	task, err := repo.GetTask(ctx, "task-1")
	require.NoError(t, err)
	task.State = v1.TaskStateTODO
	launchError := models.TaskLaunchError{
		Message: "earlier launch failure", Scope: models.ErrorScopeTask, StampValue: "stamp-1",
	}
	task.Metadata = map[string]interface{}{models.MetaKeyLastLaunchError: launchError}
	require.NoError(t, repo.UpdateTask(ctx, task))
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-1"] = &v1.Task{
		ID: "task-1", WorkspaceID: "ws1", Title: "Test Task", State: v1.TaskStateTODO,
		Metadata: map[string]interface{}{models.MetaKeyLastLaunchError: launchError},
	}
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-1"] = &wfmodels.WorkflowStep{ID: "step-1", WorkflowID: "wf1", Name: "Work", AgentProfileID: "step-profile"}
	f := &errandLaunchFixture{repo: repo, taskRepo: taskRepo, parent: "parent-1", taskID: "task-1", launchError: launchError}
	f.manager = &errandLaunchManager{mockAgentManager: &mockAgentManager{}}
	f.manager.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		f.manager.captured.Lock()
		f.manager.launchedWith = append(f.manager.launchedWith, req)
		f.manager.captured.Unlock()
		return &executor.LaunchAgentResponse{AgentExecutionID: "errand-execution"}, nil
	}
	f.manager.getExecutionIDForSessionFunc = func(_ context.Context, sessionID string) (string, error) {
		return "execution-" + sessionID, nil
	}
	f.svc = createTestServiceWithScheduler(repo, stepGetter, taskRepo, f.manager)
	f.svc.SetTurnService(taskservice.NewService(taskservice.Repos{
		Tasks: repo, TaskRepos: repo, Workspaces: repo, Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, Executors: repo, TaskEnvironments: repo,
	}, nil, testLogger(), taskservice.RepositoryDiscoveryConfig{}))
	f.svc.executor.SetOnTaskRuntimeStateReconcile(f.svc.reconcileTaskStateForRuntime)
	return f
}

func (f *errandLaunchFixture) request() errands.LaunchRequest {
	return errands.LaunchRequest{
		ErrandID: "errand-1", TaskID: f.taskID, ParentSessionID: f.parent, Title: "Audit", Objective: "audit the handlers",
		AgentProfileID: "errand-profile", TurnID: "turn-1", SubmissionID: "prompt:errand:sub-1",
	}
}

// @covers AC-TASKS-ERRANDS-001.1 AC-TASKS-ERRANDS-001.2
func TestLaunchErrandSessionCreatesOneNonPrimaryErrandSessionWithReservedIdentity(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	taskBefore, err := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)

	_, err = f.svc.LaunchErrandSession(ctx, f.request())
	require.NoError(t, err)

	sessions, err := f.repo.ListTaskSessions(ctx, f.taskID)
	require.NoError(t, err)
	require.Len(t, sessions, 2, "the parent and exactly one errand session")
	errand, err := f.repo.GetTaskSession(ctx, "errand-1")
	require.NoError(t, err)
	require.Equal(t, models.SessionRoleErrand, errand.Role)
	require.Equal(t, "parent-1", errand.ParentSessionID)
	require.False(t, errand.IsPrimary)
	require.Equal(t, "errand-profile", errand.AgentProfileID, "the explicit profile, not the workflow's")
	parent, err := f.repo.GetTaskSession(ctx, "parent-1")
	require.NoError(t, err)
	require.True(t, parent.IsPrimary, "the parent keeps primary")

	require.Len(t, f.manager.launchedWith, 1)
	launched := f.manager.launchedWith[0]
	require.Equal(t, "errand-1", launched.SessionID)
	require.Equal(t, "prompt:errand:sub-1", launched.InitialDeliverySubmissionID)
	require.Contains(t, f.manager.bound, "prompt:errand:sub-1")

	turn, err := f.repo.GetActiveTurnBySessionID(ctx, "errand-1")
	require.NoError(t, err)
	require.NotNil(t, turn)
	require.Equal(t, "turn-1", turn.ID, "the first turn honors the reserved turn ID")

	taskAfter, err := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)
	require.Equal(t, taskBefore.State, taskAfter.State, "an errand launch never writes task state")
	require.Zero(t, f.taskRepo.stateWrites[f.taskID], "no SCHEDULING or other task state write at any point")
	require.Equal(t, taskBefore.WorkflowStepID, taskAfter.WorkflowStepID)
	require.NotContains(t, taskAfter.Metadata, models.MetaKeyDeferredLaunch)
	kept, found := models.LoadTaskLaunchError(taskAfter.Metadata)
	require.True(t, found, "the task's launch error card survives an errand launch")
	require.Equal(t, f.launchError.Stamp(), kept.Stamp())
	binding, err := f.repo.GetWorkflowSessionBinding(ctx, f.taskID, workflowSessionBindingTargetKey("step-1"))
	require.NoError(t, err)
	require.Nil(t, binding, "the errand is never bound to the workflow step")
}

func TestLaunchErrandSessionFirstTurnStatesIdentityWithoutCompletionSignalOrWorkflowStep(t *testing.T) {
	f := newErrandLaunchFixture(t)
	_, err := f.svc.LaunchErrandSession(context.Background(), f.request())
	require.NoError(t, err)

	prompt := f.manager.launchedWith[0].TaskDescription
	require.Contains(t, prompt, "audit the handlers")
	require.Contains(t, prompt, "errand of session parent-1")
	require.NotContains(t, strings.ToLower(prompt), "step_complete_kandev",
		"an errand never signals workflow step completion")
}

func TestLaunchErrandSessionRequiresTheFullReservedIdentity(t *testing.T) {
	f := newErrandLaunchFixture(t)
	for name, mutate := range map[string]func(*errands.LaunchRequest){
		"no errand":     func(r *errands.LaunchRequest) { r.ErrandID = "" },
		"no turn":       func(r *errands.LaunchRequest) { r.TurnID = "" },
		"no submission": func(r *errands.LaunchRequest) { r.SubmissionID = "" },
		"no profile":    func(r *errands.LaunchRequest) { r.AgentProfileID = "" },
		"no objective":  func(r *errands.LaunchRequest) { r.Objective = "  " },
		"no parent":     func(r *errands.LaunchRequest) { r.ParentSessionID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			request := f.request()
			mutate(&request)
			_, err := f.svc.LaunchErrandSession(context.Background(), request)
			require.Error(t, err)
		})
	}
	sessions, err := f.repo.ListTaskSessions(context.Background(), f.taskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1, "a refused launch creates nothing")
}

// @covers AC-TASKS-ERRANDS-008.2
func TestLaunchErrandSessionCeilingRefusalWritesNoDeferredLaunchOrSession(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	f.svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	require.True(t, f.svc.sessionCeiling.admit(ctx, admissionRequest{
		taskID: "occupier", sessionID: "occupier-session", origin: launchOriginAutomatic, seam: "test-setup",
	}).admitted)

	_, err := f.svc.LaunchErrandSession(ctx, f.request())
	require.ErrorIs(t, err, ErrSessionCeilingRefused)
	require.NotErrorIs(t, err, ErrCeilingLaunchDeferred)

	task, err := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)
	require.NotContains(t, task.Metadata, models.MetaKeyDeferredLaunch, "the errand row is the queue entry")
	sessions, err := f.repo.ListTaskSessions(ctx, f.taskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Empty(t, f.manager.launchedWith)

	// With capacity released the same request launches.
	f.svc.sessionCeiling.release("occupier-session")
	_, err = f.svc.LaunchErrandSession(ctx, f.request())
	require.NoError(t, err)
}

func TestOrdinaryStartStillDefersAtTheCeiling(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	f.svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	require.True(t, f.svc.sessionCeiling.admit(ctx, admissionRequest{
		taskID: "occupier", sessionID: "occupier-session", origin: launchOriginAutomatic, seam: "test-setup",
	}).admitted)

	_, deferred, err := f.svc.admitStartSeam1(ctx, f.taskID, launchOriginAutomatic, startTaskOptions{},
		func() map[string]interface{} { return map[string]interface{}{"prompt": "ordinary"} })
	require.NoError(t, err)
	require.True(t, deferred, "only errands are refused instead of deferred")
}

func TestErrandAdapterMapsCeilingRefusalAndStopsOnlyExistingSessions(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	adapter := NewErrandAdapter(f.svc)

	f.svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	require.True(t, f.svc.sessionCeiling.admit(ctx, admissionRequest{
		taskID: "occupier", sessionID: "occupier-session", origin: launchOriginAutomatic, seam: "test-setup",
	}).admitted)
	require.ErrorIs(t, adapter.LaunchErrand(ctx, f.request()), errands.ErrCeilingRefused)

	require.NoError(t, adapter.StopErrandSession(ctx, "never-created", errands.StopReasonErrandCancelled),
		"a queued errand has no session to stop")
}

type fakeMCPSelectionReader struct {
	stored  []string
	enabled []string
	listed  [][2]string
	asked   [][]string
}

func (r *fakeMCPSelectionReader) List(_ context.Context, scope mcpconfig.SelectionScope, workspaceID, ownerID string) ([]string, error) {
	r.listed = append(r.listed, [2]string{string(scope) + ":" + workspaceID, ownerID})
	return r.stored, nil
}

func (r *fakeMCPSelectionReader) EnabledDefinitionIDs(_ context.Context, _ string, ids []string) ([]string, error) {
	r.asked = append(r.asked, ids)
	return r.enabled, nil
}

type recordingSelectionWriter struct {
	calls [][]string
	owner []string
}

func (w *recordingSelectionWriter) Replace(_ context.Context, scope mcpconfig.SelectionScope, workspaceID, ownerID string, ids []string) error {
	w.owner = append(w.owner, string(scope)+":"+workspaceID+":"+ownerID)
	w.calls = append(w.calls, ids)
	return nil
}

// @covers AC-TASKS-ERRANDS-001.6
func TestErrandMCPInheritanceStoresOnlyStillEnabledServersAsTheSessionScope(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	reader := &fakeMCPSelectionReader{stored: []string{"mcp-1", "mcp-2"}, enabled: []string{"mcp-1"}}
	writer := &recordingSelectionWriter{}
	f.svc.SetMCPSelectionReader(reader)
	f.svc.SetMCPSelectionWriter(writer)

	ids, err := NewErrandAdapter(f.svc).SessionServerIDs(ctx, f.taskID, f.parent)
	require.NoError(t, err)
	require.Equal(t, []string{"mcp-1", "mcp-2"}, ids)
	require.Equal(t, [2]string{"task_session:ws1", "parent-1"}, reader.listed[0], "the parent's task-session scope")

	request := f.request()
	request.MCPServerIDs = ids
	_, err = f.svc.LaunchErrandSession(ctx, request)
	require.NoError(t, err)
	require.Equal(t, [][]string{{"mcp-1", "mcp-2"}}, reader.asked)
	require.Equal(t, [][]string{{"mcp-1"}}, writer.calls, "only the enabled definition becomes the errand's scope")
	require.Equal(t, []string{"task_session:ws1:errand-1"}, writer.owner)
}

func TestErrandMCPInheritanceLeavesTheDefaultScopeWhenNothingRemainsEnabled(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	f.svc.SetMCPSelectionReader(&fakeMCPSelectionReader{enabled: nil})
	writer := &recordingSelectionWriter{}
	f.svc.SetMCPSelectionWriter(writer)

	request := f.request()
	request.MCPServerIDs = []string{"gone"}
	_, err := f.svc.LaunchErrandSession(ctx, request)
	require.NoError(t, err)
	require.Empty(t, writer.calls, "nil selections do not replace the session scope")

	f2 := newErrandLaunchFixture(t)
	writer2 := &recordingSelectionWriter{}
	f2.svc.SetMCPSelectionWriter(writer2)
	_, err = f2.svc.LaunchErrandSession(ctx, f2.request())
	require.NoError(t, err)
	require.Empty(t, writer2.calls)
	_ = errors.New
}

func TestErrandAdapterTreatsAnAlreadyGoneRuntimeAsStopped(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	_, err := f.svc.LaunchErrandSession(ctx, f.request())
	require.NoError(t, err)
	f.manager.getExecutionIDForSessionFunc = func(_ context.Context, sessionID string) (string, error) {
		return "", fmt.Errorf("%w: %s", agentruntime.ErrNoExecutionForSession, sessionID)
	}
	require.NoError(t, NewErrandAdapter(f.svc).StopErrandSession(ctx, "errand-1", errands.StopReasonErrandCancelled))
}
