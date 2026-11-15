package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const contractNativeConversation = "provider-contract-conversation"

type completedConversationEvents chan string

func (done completedConversationEvents) PublishErrandUpdated(_ context.Context, row *models.Errand, _ models.ErrandStatus) {
	if row.TeardownIntent != nil && row.TeardownIntent.State == models.ErrandTeardownIntentCompleted {
		done <- row.LatestSettledTurnID
	}
}

// Each owner has fresh services, subscriptions and runtime inventory. Only the
// SQLite repository is supplied by the scenario, including after a pool reopen.
type completedConversationOwner struct {
	repo      *sqliterepo.Repository
	svc       *Service
	errands   *errands.Service
	manager   *errandLaunchManager
	attempts  map[string]string
	bus       *bus.MemoryEventBus
	prompts   chan promptCall
	queueDone chan struct{}
	completed completedConversationEvents
	stopOnce  sync.Once
}

func newCompletedConversationOwner(t *testing.T, repo *sqliterepo.Repository) *completedConversationOwner {
	t.Helper()
	database := sqlx.NewDb(repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(database, database)
	require.NoError(t, err)
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	queue.SetAutoMergeEnabled(false)
	eventBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(eventBus.Close)
	task, err := repo.GetTask(t.Context(), "task-1")
	require.NoError(t, err)
	tasks := newMockTaskRepo()
	tasks.tasks[task.ID] = task.ToAPI()
	manager := &errandLaunchManager{mockAgentManager: &mockAgentManager{}, runtimes: map[string]*agentruntime.AgentExecution{}}
	f := &completedConversationOwner{repo: repo, manager: manager, bus: eventBus,
		attempts: make(map[string]string), prompts: make(chan promptCall, 8), queueDone: make(chan struct{}, 8),
		completed: make(completedConversationEvents, 16)}
	f.svc = NewService(DefaultServiceConfig(), eventBus, manager, tasks, repo, nil, nil, queue, testLogger())
	withTestExecutorActor(f.svc.executor)
	steps := newMockStepGetter()
	steps.steps["step-1"] = &wfmodels.WorkflowStep{ID: "step-1", WorkflowID: "wf1", Name: "Work", AgentProfileID: "step-profile"}
	f.svc.SetWorkflowStepGetter(steps)
	turns := taskservice.NewService(taskservice.Repos{Tasks: repo, TaskRepos: repo, Workspaces: repo, Workflows: repo,
		Messages: repo, Turns: repo, Sessions: repo, Executors: repo, TaskEnvironments: repo,
	}, eventBus, testLogger(), taskservice.RepositoryDiscoveryConfig{})
	f.svc.SetTurnService(turns)
	f.svc.SetTransientRetryMessageService(turns)
	f.svc.SetMessageCreator(&repositoryBackedMessageCreator{mockMessageCreator: &mockMessageCreator{}, repo: repo})
	f.svc.onQueuedMessageExecutionComplete = func() { f.queueDone <- struct{}{} }
	f.installRuntime()
	adapter := NewErrandAdapter(f.svc)
	f.errands, err = errands.New(errands.Dependencies{Store: repo, Launcher: adapter, Stopper: adapter, MCP: adapter,
		Profiles: errandProfileStub{}, Events: f.completed, Logger: testLogger()})
	require.NoError(t, err)
	f.errands.SetTurnReader(settlementTranscript{repo})
	f.errands.SetDeliverer(NewErrandParentDeliverer(f.svc))
	f.svc.SetErrandController(f.errands)
	f.errands.StartWorkers(t.Context())
	t.Cleanup(func() { f.stop(t) })
	require.NoError(t, f.svc.StartEventWatcher(t.Context()))
	return f
}

func (f *completedConversationOwner) stop(t *testing.T) {
	t.Helper()
	f.stopOnce.Do(func() {
		require.NoError(t, f.svc.messageQueue.SetAutoRun(context.Background(), "parent-1", false))
		require.Eventually(t, func() bool { return !f.svc.isQueuedDispatchInFlight("parent-1") },
			5*time.Second, 5*time.Millisecond, "drain parent queue before releasing its database")
		f.errands.StopWorkers()
		require.NoError(t, f.svc.StopEventWatcher())
		f.svc.cancelResumeAttempts()
		f.svc.cancelAllTransientRetries()
		f.svc.stopBackgroundWorkSettleWorkers()
		f.svc.stopSendNowWorkers()
		f.bus.Close()
	})
}

func (f *completedConversationOwner) installRuntime() {
	m := f.manager
	m.getExecutionIDForSessionFunc = func(_ context.Context, sessionID string) (string, error) {
		m.captured.Lock()
		defer m.captured.Unlock()
		for id, execution := range m.runtimes {
			if execution.SessionID == sessionID {
				return id, nil
			}
		}
		return "", agentruntime.ErrNoExecutionForSession
	}
	m.isAgentRunningFn = func(ctx context.Context, sessionID string) bool {
		id, err := m.GetExecutionIDForSession(ctx, sessionID)
		if err != nil {
			return false
		}
		execution, ok := m.GetExecution(id)
		return ok && execution.Status == v1.AgentStatusRunning
	}
	m.isAgentReadyFn = m.isAgentRunningFn
	m.rowLivenessFn = func(row *models.ExecutorRunning) models.ProcessLiveness {
		if _, live := m.GetExecution(row.AgentExecutionID); live {
			return models.ProcessLivenessAlive
		}
		return models.ProcessLivenessDead
	}
	m.getACPSessionIDForSessionFunc = func(sessionID string) (string, bool) {
		if _, err := m.GetExecutionIDForSession(context.Background(), sessionID); err != nil {
			return "", false
		}
		session, err := f.repo.GetTaskSession(context.Background(), sessionID)
		if err != nil {
			return "", false
		}
		return session.DownstreamACPSessionID, session.DownstreamACPSessionID != ""
	}
	m.getPromptActivityForSessionFunc = func(sessionID string) (string, uint64, uint64, time.Time, error) {
		id, err := m.GetExecutionIDForSession(context.Background(), sessionID)
		return id, 0, 0, time.Time{}, err
	}
	m.stopAgentWithReasonFunc = func(_ context.Context, id, _ string, _ bool) error {
		m.captured.Lock()
		delete(m.runtimes, id)
		m.captured.Unlock()
		return nil
	}
	m.launchAgentFunc = f.launch
	m.startAgentProcessFunc = f.boot
	m.promptAgentFunc = func(_ context.Context, executionID, prompt string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
		f.prompts <- promptCall{ExecutionID: executionID, Prompt: prompt}
		return &executor.PromptResult{}, nil
	}
}

func (f *completedConversationOwner) launch(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	id := req.CandidateExecutionID
	if id == "" {
		id = "restored-" + req.SessionID
	}
	if req.OnExecutionAllocated != nil {
		if err := req.OnExecutionAllocated(ctx, id); err != nil {
			return nil, err
		}
	}
	f.manager.captured.Lock()
	f.manager.runtimes[id] = &agentruntime.AgentExecution{ID: id, TaskID: req.TaskID, SessionID: req.SessionID,
		DeliveryIncarnationID: req.DeliveryIncarnationID, Status: v1.AgentStatusStarting}
	f.manager.launchedWith = append(f.manager.launchedWith, req)
	f.attempts[id] = executor.ResumeAttemptIDFromContext(ctx)
	f.manager.captured.Unlock()
	if err := f.repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: id, TaskID: req.TaskID, SessionID: req.SessionID,
		AgentExecutionID: id, ResumeToken: req.ACPSessionID, Resumable: true, Status: "starting"}); err != nil {
		return nil, err
	}
	return &executor.LaunchAgentResponse{AgentExecutionID: id}, nil
}

func (f *completedConversationOwner) boot(ctx context.Context, executionID string) error {
	f.manager.captured.Lock()
	stored, ok := f.manager.runtimes[executionID]
	if !ok {
		f.manager.captured.Unlock()
		return fmt.Errorf("missing allocated execution %s", executionID)
	}
	execution := &agentruntime.AgentExecution{ID: stored.ID, TaskID: stored.TaskID, SessionID: stored.SessionID,
		DeliveryIncarnationID: stored.DeliveryIncarnationID, Status: v1.AgentStatusRunning}
	f.manager.runtimes[executionID] = execution
	f.manager.captured.Unlock()
	attemptID := executor.ResumeAttemptIDFromContext(ctx)
	if attemptID == "" {
		return fmt.Errorf("missing resume attempt for %s", executionID)
	}
	data := watcher.AgentEventData{TaskID: execution.TaskID, SessionID: execution.SessionID,
		AgentExecutionID: executionID, AttemptID: attemptID}
	return f.bus.Publish(ctx, events.AgentBootReady, bus.NewEvent(events.AgentBootReady, "contract-provider", data))
}

func (f *completedConversationOwner) retainExecution(t *testing.T, sessionID, executionID string) {
	t.Helper()
	session, err := f.repo.GetTaskSession(t.Context(), sessionID)
	require.NoError(t, err)
	require.NoError(t, f.repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: sessionID,
		TaskID: session.TaskID, SessionID: sessionID, AgentExecutionID: executionID,
		ResumeToken: session.DownstreamACPSessionID, Resumable: true, Status: "ready"}))
	f.manager.captured.Lock()
	f.manager.runtimes[executionID] = &agentruntime.AgentExecution{ID: executionID, TaskID: session.TaskID,
		SessionID: sessionID, DeliveryIncarnationID: session.QueueIncarnationID, Status: v1.AgentStatusRunning}
	f.manager.captured.Unlock()
}

func (f *completedConversationOwner) publishReady(t *testing.T, sessionID, executionID, turnID string) {
	t.Helper()
	f.manager.captured.Lock()
	attemptID := f.attempts[executionID]
	f.manager.captured.Unlock()
	require.NoError(t, f.bus.Publish(t.Context(), events.AgentReady, bus.NewEvent(events.AgentReady, "contract-provider",
		watcher.AgentEventData{TaskID: "task-1", SessionID: sessionID, AgentExecutionID: executionID,
			TurnID: turnID, AttemptID: attemptID})))
}

func (f *completedConversationOwner) receivePrompt(t *testing.T) promptCall {
	t.Helper()
	select {
	case prompt := <-f.prompts:
		return prompt
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not receive a prompt")
		return promptCall{}
	}
}

func (f *completedConversationOwner) waitQueue(t *testing.T) {
	t.Helper()
	select {
	case <-f.queueDone:
	case <-time.After(5 * time.Second):
		t.Fatal("parent notification worker did not finish")
	}
}
