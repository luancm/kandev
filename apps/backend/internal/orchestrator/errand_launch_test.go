package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/events/bus"
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
	runtimes     map[string]*agentruntime.AgentExecution
}

func (m *errandLaunchManager) GetExecution(id string) (*agentruntime.AgentExecution, bool) {
	m.captured.Lock()
	defer m.captured.Unlock()
	execution, ok := m.runtimes[id]
	return execution, ok
}

func (m *errandLaunchManager) SetInitialDeliverySubmissionID(_ context.Context, _, submissionID string) error {
	m.captured.Lock()
	defer m.captured.Unlock()
	m.bound = append(m.bound, submissionID)
	return nil
}

type errandLaunchFixture struct {
	t              *testing.T
	svc            *Service
	repo           *sqliterepo.Repository
	taskRepo       *mockTaskRepo
	manager        *errandLaunchManager
	errandsRuntime *errandServiceRuntime
	parent         string
	taskID         string
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
	f := &errandLaunchFixture{t: t, repo: repo, taskRepo: taskRepo, parent: "parent-1", taskID: "task-1", launchError: launchError}
	f.manager = &errandLaunchManager{mockAgentManager: &mockAgentManager{}, runtimes: map[string]*agentruntime.AgentExecution{}}
	f.manager.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		f.manager.captured.Lock()
		id := "execution-" + req.SessionID
		f.manager.runtimes[id] = &agentruntime.AgentExecution{ID: id, TaskID: req.TaskID, SessionID: req.SessionID, DeliveryIncarnationID: req.DeliveryIncarnationID}
		f.manager.launchedWith = append(f.manager.launchedWith, req)
		f.manager.captured.Unlock()
		return &executor.LaunchAgentResponse{AgentExecutionID: id}, nil
	}
	f.manager.getExecutionIDForSessionFunc = func(_ context.Context, sessionID string) (string, error) {
		if _, ok := f.manager.GetExecution("execution-" + sessionID); !ok {
			return "", agentruntime.ErrNoExecutionForSession
		}
		return "execution-" + sessionID, nil
	}
	f.manager.getPromptActivityForSessionFunc = func(sessionID string) (string, uint64, uint64, time.Time, error) {
		id, err := f.manager.getExecutionIDForSessionFunc(context.Background(), sessionID)
		return id, 0, 0, time.Time{}, err
	}
	f.manager.stopAgentWithReasonFunc = func(_ context.Context, id, _ string, _ bool) error {
		f.manager.captured.Lock()
		delete(f.manager.runtimes, id)
		f.manager.captured.Unlock()
		return nil
	}
	f.svc = createTestServiceWithScheduler(repo, stepGetter, taskRepo, f.manager)
	turnBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(turnBus.Close)
	f.svc.SetTurnService(taskservice.NewService(taskservice.Repos{
		Tasks: repo, TaskRepos: repo, Workspaces: repo, Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, Executors: repo, TaskEnvironments: repo,
	}, turnBus, testLogger(), taskservice.RepositoryDiscoveryConfig{}))
	f.svc.executor.SetOnTaskRuntimeStateReconcile(f.svc.reconcileTaskStateForRuntime)
	return f
}

func (f *errandLaunchFixture) request() errands.LaunchRequest {
	req := errands.LaunchRequest{
		ErrandID: "errand-1", TaskID: f.taskID, ParentSessionID: f.parent, Title: "Audit", Objective: "audit the handlers",
		AgentProfileID: "errand-profile", TurnID: "turn-1", SubmissionID: "prompt:errand:sub-1",
	}
	if _, err := f.repo.GetErrand(f.t.Context(), req.ErrandID); errors.Is(err, models.ErrErrandNotFound) {
		seedErrandLaunchReservation(f.t, f, req)
	} else {
		require.NoError(f.t, err)
	}
	return req
}

func TestOrdinaryStartStillDefersAtTheCeiling(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	f.svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	require.True(t, f.svc.sessionCeiling.admit(ctx, admissionRequest{
		taskID: "occupier", sessionID: "occupier-session", origin: launchOriginAutomatic, seam: "test-setup",
	}).admitted)

	_, deferred, err := f.svc.admitOrDeferSeam1(ctx, f.taskID, launchOriginAutomatic, map[string]interface{}{"prompt": "ordinary"})
	require.NoError(t, err)
	require.True(t, deferred, "ordinary automatic starts keep their deferred launch")
}

func TestErrandAdapterMapsCeilingRefusal(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	adapter := NewErrandAdapter(f.svc)
	adapter.SetOwnedLaunchRuntime(ownedErrandLaunchFixtureRuntime{}, func(_ context.Context, taskID, parentID string) (*agentruntime.WorkspaceInfo, error) {
		return &agentruntime.WorkspaceInfo{
			TaskID: taskID, SessionID: parentID, WorkspaceID: "ws1",
			ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: "/workspace",
		}, nil
	})

	f.svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	require.True(t, f.svc.sessionCeiling.admit(ctx, admissionRequest{
		taskID: "occupier", sessionID: "occupier-session", origin: launchOriginAutomatic, seam: "test-setup",
	}).admitted)
	result, err := adapter.LaunchErrand(ctx, f.request())
	require.NoError(t, err)
	require.Equal(t, errands.LaunchQueued, result.Disposition)
}

func seedErrandLaunchReservation(t *testing.T, f *errandLaunchFixture, req errands.LaunchRequest) *models.Errand {
	t.Helper()
	row := &models.Errand{ID: req.ErrandID, TaskID: req.TaskID, ParentSessionID: req.ParentSessionID,
		Title: req.Title, Objective: req.Objective, AgentProfileID: req.AgentProfileID,
		FirstTurnID: req.TurnID, SubmissionID: req.SubmissionID,
		Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone, AwaitingResult: true}
	require.NoError(t, f.repo.CreateErrand(t.Context(), row))
	return row
}
