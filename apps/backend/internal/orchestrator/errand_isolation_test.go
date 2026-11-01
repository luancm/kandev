package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	repoerrors "github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type errandIsolationVariant string

const (
	variantOrdinary errandIsolationVariant = "ordinary"
	variantFollowUp errandIsolationVariant = "completion follow-up"
	variantErrand   errandIsolationVariant = "errand"
)

// seedErrandSession inserts an errand of parentID that started after it.
func seedErrandSession(
	t *testing.T, repo *sqliterepo.Repository, taskID, parentID, errandID string, state models.TaskSessionState,
) {
	t.Helper()
	startedAt := time.Now().UTC().Add(time.Minute)
	require.NoError(t, repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: errandID, TaskID: taskID, Role: models.SessionRoleErrand, ParentSessionID: parentID,
		State: state, StartedAt: startedAt, UpdatedAt: startedAt,
	}))
}

// seedOrdinarySession inserts an ordinary session started at offset from now.
func seedOrdinarySession(
	t *testing.T, repo *sqliterepo.Repository, taskID, sessionID string, state models.TaskSessionState, offset time.Duration,
) {
	t.Helper()
	startedAt := time.Now().UTC().Add(offset)
	require.NoError(t, repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: state, StartedAt: startedAt, UpdatedAt: startedAt,
	}))
}

// markSessionErrand turns an already seeded session into an errand. Only
// tests rewrite the role; production writes it once at insert.
func markSessionErrand(t *testing.T, repo *sqliterepo.Repository, sessionID, parentID string) {
	t.Helper()
	_, err := repo.DB().ExecContext(context.Background(),
		`UPDATE task_sessions SET role = ?, parent_session_id = ?, is_primary = 0 WHERE id = ?`,
		string(models.SessionRoleErrand), parentID, sessionID)
	require.NoError(t, err)
}

// applyIsolationVariant makes sessionID the requested kind of session. The
// errand variant also seeds its primary parent.
func applyIsolationVariant(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID string, variant errandIsolationVariant) {
	t.Helper()
	ctx := context.Background()
	switch variant {
	case variantFollowUp:
		require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyCompletionFollowUp, true))
	case variantErrand:
		seedOrdinarySession(t, repo, taskID, "parent-of-"+sessionID, models.TaskSessionStateWaitingForInput, -time.Minute)
		require.NoError(t, repo.SetSessionPrimary(ctx, "parent-of-"+sessionID))
		markSessionErrand(t, repo, sessionID, "parent-of-"+sessionID)
	}
}

func installMovingStep(t *testing.T, fixture *deliveryWorkflowOutcomeFixture, stepID, nextID string) {
	t.Helper()
	stepTime := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	fixture.installStep(t, &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: "wf1", Position: 0, UpdatedAt: stepTime,
		Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
			Type:   wfmodels.OnTurnCompleteMoveToStep,
			Config: map[string]interface{}{"step_id": nextID},
		}}},
	})
	fixture.installStep(t, &wfmodels.WorkflowStep{ID: nextID, WorkflowID: "wf1", Position: 1, UpdatedAt: stepTime})
}

func (f *deliveryWorkflowOutcomeFixture) agentEvent() watcher.AgentEventData {
	return watcher.AgentEventData{
		TaskID: f.taskID, SessionID: f.session.ID, TurnID: f.effect.TurnID,
		DeliveryStreamID: f.effect.StreamID, DeliveryIncarnationID: f.effect.IncarnationID,
		DeliveryHarnessGeneration: 1, DeliverySequence: 1, DeliverySubmissionID: f.effect.SubmissionID,
	}
}

func (f *deliveryWorkflowOutcomeFixture) taskStateWrites() []v1.TaskState {
	taskRepo := f.service.taskRepo.(*mockTaskRepo)
	taskRepo.mu.Lock()
	defer taskRepo.mu.Unlock()
	return append([]v1.TaskState(nil), taskRepo.stateHistory[f.taskID]...)
}

// @covers AC-TASKS-ERRANDS-005.1
func TestErrandAgentReadyRecordsNotRequiredWithoutWorkflowOrTaskState(t *testing.T) {
	cases := []struct {
		variant     errandIsolationVariant
		wantStep    string
		wantOutcome string
		wantWrites  []v1.TaskState
	}{
		{variant: variantOrdinary, wantStep: "ready-next-ordinary", wantOutcome: "applied"},
		{variant: variantFollowUp, wantStep: "ready-step-follow-up", wantOutcome: "not_required",
			wantWrites: []v1.TaskState{v1.TaskStateReview}},
		{variant: variantErrand, wantStep: "ready-step-errand", wantOutcome: "not_required"},
	}
	for _, tc := range cases {
		t.Run(string(tc.variant), func(t *testing.T) {
			suffix := map[errandIsolationVariant]string{
				variantOrdinary: "ordinary", variantFollowUp: "follow-up", variantErrand: "errand",
			}[tc.variant]
			fixture := newDeliveryWorkflowOutcomeFixture(t, "ready-"+suffix, "ready-step-"+suffix)
			installMovingStep(t, fixture, "ready-step-"+suffix, "ready-next-"+suffix)
			applyIsolationVariant(t, fixture.repo, fixture.taskID, fixture.session.ID, tc.variant)

			fixture.service.handleAgentReady(fixture.ctx, fixture.agentEvent())

			task, err := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
			require.NoError(t, err)
			require.Equal(t, tc.wantStep, task.WorkflowStepID)
			stored, err := fixture.repo.GetAgentDeliveryEffect(fixture.ctx, fixture.effect.EffectKey)
			require.NoError(t, err)
			require.Equal(t, tc.wantOutcome, stored.Outcome)
			if tc.variant == variantOrdinary {
				return
			}
			require.Equal(t, models.TaskSessionStateWaitingForInput, mustGetSession(t, fixture.repo, fixture.session.ID).State)
			require.Equal(t, tc.wantWrites, fixture.taskStateWrites())
		})
	}
}

// @covers AC-TASKS-ERRANDS-005.1
func TestErrandTurnCompleteViaEngineRecordsNotRequiredWithoutTransition(t *testing.T) {
	fixture := newDeliveryWorkflowOutcomeFixture(t, "engine-errand", "engine-step")
	installMovingStep(t, fixture, "engine-step", "engine-next")
	fixture.session.Role = models.SessionRoleErrand
	fixture.session.ParentSessionID = "parent-engine"

	transitioned := fixture.service.processOnTurnCompleteViaEngineWithCause(
		withWorkflowEffect(fixture.ctx, fixture.effect), fixture.taskID, fixture.session, turnCompletionCauseAgentTurn,
	)

	require.False(t, transitioned)
	task, err := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, "engine-step", task.WorkflowStepID)
	stored, err := fixture.repo.GetAgentDeliveryEffect(fixture.ctx, fixture.effect.EffectKey)
	require.NoError(t, err)
	require.Equal(t, "not_required", stored.Outcome)
	require.Empty(t, fixture.taskStateWrites())
}

// @covers AC-TASKS-ERRANDS-005.1
func TestErrandTurnStartSkipsWorkflowTurnStartActions(t *testing.T) {
	for _, variant := range []errandIsolationVariant{variantOrdinary, variantFollowUp, variantErrand} {
		t.Run(string(variant), func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			steps, ids := buildWorkflowFromJSON(t, developmentWorkflowJSON)
			seedSession(t, repo, "task", "session", ids["Backlog"])
			applyIsolationVariant(t, repo, "task", "session", variant)
			svc := createEngineService(t, repo, steps, &mockAgentManager{repoForExecutionLookup: repo})

			transitioned, err := svc.processOnTurnStartViaEngineResult(ctx, "task", mustGetSession(t, repo, "session"))

			require.NoError(t, err)
			require.Equal(t, variant == variantOrdinary, transitioned)
			wantStep := ids["Backlog"]
			if variant == variantOrdinary {
				wantStep = ids["In Progress"]
			}
			task, err := repo.GetTask(ctx, "task")
			require.NoError(t, err)
			require.Equal(t, wantStep, task.WorkflowStepID)
		})
	}
}

// @covers AC-TASKS-ERRANDS-005.1
func TestErrandAgentCompletedSettlesWithoutWorkflowOrTaskState(t *testing.T) {
	for _, variant := range []errandIsolationVariant{variantFollowUp, variantErrand} {
		t.Run(string(variant), func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			steps, ids := buildWorkflowFromJSON(t, developmentWorkflowJSON)
			seedSession(t, repo, "task", "session", ids["New Step"])
			seedExecutorRunning(t, repo, "session", "task", "exec-session")
			applyIsolationVariant(t, repo, "task", "session", variant)
			taskRepo := newMockTaskRepo()
			seedMockTaskState(taskRepo, "task", v1.TaskStateInProgress)
			agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
			svc := createEngineServiceWithScheduler(t, repo, steps, taskRepo, agentMgr)

			svc.handleAgentCompleted(ctx, watcher.AgentEventData{
				TaskID: "task", SessionID: "session", AgentExecutionID: "exec-session",
			})
			waitForStopCall(t, agentMgr)

			require.Equal(t, models.TaskSessionStateWaitingForInput, mustGetSession(t, repo, "session").State)
			task, err := repo.GetTask(ctx, "task")
			require.NoError(t, err)
			require.Equal(t, ids["New Step"], task.WorkflowStepID)
			taskRepo.mu.Lock()
			writes := append([]v1.TaskState(nil), taskRepo.stateHistory["task"]...)
			taskRepo.mu.Unlock()
			if variant == variantErrand {
				require.Empty(t, writes, "an errand completion must not write task state")
			} else {
				require.Equal(t, []v1.TaskState{v1.TaskStateReview}, writes)
			}
		})
	}
}

// @covers AC-TASKS-ERRANDS-005.1
func TestErrandRunningDoesNotHoldParentCompletionOutOfReview(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "parent", "")
	seedExecutorRunning(t, repo, "parent", "t1", "exec-parent")
	seedErrandSession(t, repo, "t1", "parent", "errand", models.TaskSessionStateRunning)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "parent", AgentExecutionID: "exec-parent",
	})
	waitForStopCall(t, agentMgr)

	require.Equal(t, v1.TaskStateReview, taskRepo.updatedStates["t1"])
}

// @covers AC-TASKS-ERRANDS-005.1
func TestErrandRecoverableFailureSkipsTaskStateAndWorkflowRecovery(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "parent", "step1")
	setSessionState(t, ctx, repo, "parent", models.TaskSessionStateWaitingForInput)
	seedErrandSession(t, repo, "t1", "parent", "errand", models.TaskSessionStateRunning)
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionAutoStartAgent}}},
	}
	decisions := &spyDecisionStore{}
	svc, _ := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
	taskRepo := svc.taskRepo.(*mockTaskRepo)
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)

	svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "errand", AgentExecutionID: "exec-errand", ErrorMessage: "agent crashed",
	})
	waitForFailureRecovery(t, svc)

	require.Equal(t, models.TaskSessionStateWaitingForInput, mustGetSession(t, repo, "errand").State)
	require.Empty(t, taskRepo.stateHistory["t1"], "an errand failure must not write task state")
	require.Zero(t, decisions.clearCalls, "an errand failure must not dispatch on_agent_error")
}

// @covers AC-TASKS-ERRANDS-005.1
func TestErrandNeverOwnsTaskRuntimeState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "parent", "")
	seedErrandSession(t, repo, "t1", "parent", "errand", models.TaskSessionStateRunning)
	errand := mustGetSession(t, repo, "errand")
	parent := mustGetSession(t, repo, "parent")

	require.False(t, runtimeSessionOwnsTaskState(errand, v1.TaskStateInProgress))
	require.True(t, runtimeSessionOwnsTaskState(parent, v1.TaskStateInProgress))
	failedErrand := *errand
	failedErrand.State = models.TaskSessionStateFailed
	require.False(t, runtimeSessionOwnsTaskState(&failedErrand, v1.TaskStateFailed))

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateReview)
	svc := createTestService(repo, newMockStepGetter(), taskRepo)
	svc.writeTaskInProgressForRuntime(ctx, "t1", "errand")
	require.Empty(t, taskRepo.stateHistory["t1"], "a running errand must not move the task to IN_PROGRESS")
	svc.writeTaskInProgressForRuntime(ctx, "t1", "parent")
	require.Equal(t, []v1.TaskState{v1.TaskStateInProgress}, taskRepo.stateHistory["t1"])

	setSessionState(t, ctx, repo, "errand", models.TaskSessionStateWaitingForInput)
	svc.writeTaskReviewState(ctx, "t1", "errand")
	svc.writeTaskReviewStateOnCancel(ctx, "t1", "errand")
	require.Equal(t, []v1.TaskState{v1.TaskStateInProgress}, taskRepo.stateHistory["t1"],
		"an errand completion or cancel must not move the task to REVIEW")
}

// @covers AC-TASKS-ERRANDS-005.1
func TestErrandWaitingSettleLeavesTaskState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "ordinary", "")
	seedOrdinarySession(t, repo, "t1", "settled", models.TaskSessionStateRunning, time.Second)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	svc := createTestService(repo, newMockStepGetter(), taskRepo)

	svc.setSessionWaitingForInputWithoutTaskState(ctx, "t1", "settled")
	require.Equal(t, models.TaskSessionStateWaitingForInput, mustGetSession(t, repo, "settled").State)
	require.Empty(t, taskRepo.stateHistory["t1"])

	setSessionState(t, ctx, repo, "settled", models.TaskSessionStateCompleted)
	svc.setSessionWaitingForInput(ctx, "t1", "ordinary")
	require.Equal(t, []v1.TaskState{v1.TaskStateReview}, taskRepo.stateHistory["t1"])
}

// @covers AC-TASKS-ERRANDS-012.3
func TestErrandIsolationDoesNotDependOnErrandsFlag(t *testing.T) {
	for _, flag := range []string{"false", "true"} {
		t.Run("features.errands="+flag, func(t *testing.T) {
			t.Setenv("KANDEV_FEATURES_ERRANDS", flag)
			ctx := context.Background()
			repo := setupTestRepo(t)
			steps, ids := buildWorkflowFromJSON(t, developmentWorkflowJSON)
			seedSession(t, repo, "task", "parent", ids["New Step"])
			setSessionState(t, ctx, repo, "parent", models.TaskSessionStateWaitingForInput)
			require.NoError(t, repo.SetSessionPrimary(ctx, "parent"))
			seedErrandSession(t, repo, "task", "parent", "errand", models.TaskSessionStateRunning)
			taskRepo := newMockTaskRepo()
			seedMockTaskState(taskRepo, "task", v1.TaskStateInProgress)
			svc := createEngineServiceWithTaskRepo(t, repo, steps, taskRepo, &mockAgentManager{repoForExecutionLookup: repo})

			svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "task", SessionID: "errand"})

			require.Equal(t, models.TaskSessionStateWaitingForInput, mustGetSession(t, repo, "errand").State)
			task, err := repo.GetTask(ctx, "task")
			require.NoError(t, err)
			require.Equal(t, ids["New Step"], task.WorkflowStepID)
			require.Empty(t, taskRepo.stateHistory["task"])
			require.True(t, mustGetSession(t, repo, "parent").IsPrimary)
		})
	}
}

type parentSettlementFaultStore struct {
	errands.Store
	mu           sync.Mutex
	failNext     bool
	failureCount int
}

func (s *parentSettlementFaultStore) ListErrandsByParent(
	ctx context.Context, parentSessionID string,
) ([]*models.Errand, error) {
	s.mu.Lock()
	if s.failNext {
		s.failNext = false
		s.failureCount++
		s.mu.Unlock()
		return nil, errors.New("injected parent errand read failure")
	}
	s.mu.Unlock()
	return s.Store.ListErrandsByParent(ctx, parentSessionID)
}

func (s *parentSettlementFaultStore) failNextParentRead() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNext = true
}

func (s *parentSettlementFaultStore) failures() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failureCount
}

type parentSettlementFailureFixture struct {
	repo         *sqliterepo.Repository
	service      *Service
	taskRepo     *mockTaskRepo
	agentManager *mockAgentManager
	errands      *errands.Service
	store        *parentSettlementFaultStore
	taskID       string
	parentID     string
	turnID       string
	errandID     string
	event        watcher.AgentEventData
}

func newParentSettlementFailureFixture(t *testing.T, suffix string) *parentSettlementFailureFixture {
	t.Helper()
	ctx := context.Background()
	taskID, parentID := "settlement-task-"+suffix, "settlement-parent-"+suffix
	submissionID := "settlement-submission-" + suffix
	turnID := "turn-" + submissionID
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, parentID, "step1")
	require.NoError(t, repo.SetSessionPrimary(ctx, parentID))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: turnID, TaskID: taskID, TaskSessionID: parentID, StartedAt: time.Now().UTC(),
	}))
	parent, err := repo.GetTaskSession(ctx, parentID)
	require.NoError(t, err)
	incarnationID := parent.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = parent.ID
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: parentID, IncarnationID: incarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "dispatch-" + suffix,
		PayloadHash: "payload-" + suffix, Payload: []byte("parent prompt"),
		State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatalf("prepare parent terminal submission: %v", err)
	}
	streamID := "settlement-stream-" + suffix
	terminal := persistedTerminalEvent(parentID, incarnationID, 1, submissionID, streamID, "complete")
	projectOrchestratorDeliveryEvent(t, repo, ctx, terminal)

	steps := newMockStepGetter()
	steps.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Position: 0,
		CancelTriggersTurnComplete: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
				Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "terminal"},
			}},
			OnAgentError: []wfmodels.GenericAction{{
				Type: wfmodels.GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "terminal"},
			}},
		},
	}
	steps.steps["terminal"] = &wfmodels.WorkflowStep{
		ID: "terminal", WorkflowID: "wf1", Position: 1, CompleteTaskOnEnter: true,
	}
	for _, step := range steps.steps {
		step.UpdatedAt = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
		_, insertErr := repo.DB().ExecContext(ctx, `
			INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`, step.ID, step.WorkflowID, step.ID, step.Position,
			step.UpdatedAt, step.UpdatedAt)
		require.NoError(t, insertErr)
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	manager := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true,
		currentPromptExecutionID: "exec-" + suffix}
	manager.currentPromptGeneration.Store(1)
	manager.currentPromptActivityEpoch.Store(1)
	seedExecutorRunning(t, repo, parentID, taskID, manager.currentPromptExecutionID)
	service := createTestServiceWithScheduler(repo, steps, taskRepo, &errandLaunchManager{mockAgentManager: manager})
	onEnterDone := make(chan struct{}, 1)
	service.onProcessOnEnterComplete = func() { onEnterDone <- struct{}{} }
	t.Cleanup(func() {
		task, readErr := repo.GetTask(context.Background(), taskID)
		if readErr == nil && task.WorkflowStepID == "terminal" {
			select {
			case <-onEnterDone:
			case <-time.After(5 * time.Second):
				t.Error("workflow on-enter did not finish before fixture cleanup")
			}
		}
	})
	service.SetWorkflowStepGetter(steps)
	service.SetTurnService(taskservice.NewService(taskservice.Repos{
		Tasks: repo, TaskRepos: repo, Workspaces: repo, Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, Executors: repo, TaskEnvironments: repo,
	}, nil, testLogger(), taskservice.RepositoryDiscoveryConfig{}))
	service.executor.SetOnTaskRuntimeStateReconcile(service.reconcileTaskStateForRuntime)
	store := &parentSettlementFaultStore{Store: repo}
	adapter := NewErrandAdapter(service)
	errService, err := errands.New(errands.Dependencies{
		Store: store, Launcher: adapter, Stopper: adapter, MCP: adapter,
		Profiles: errandProfileStub{}, Events: &errandEventRecorder{}, Logger: testLogger(),
	})
	require.NoError(t, err)
	service.SetErrandController(errService)
	started, err := errService.Start(ctx, errands.StartRequest{
		TaskID: taskID, ParentSessionID: parentID, Title: "read failure", Objective: "stay outstanding",
	})
	require.NoError(t, err)
	errRow, err := repo.GetErrand(ctx, started.Errand.ID)
	require.NoError(t, err)
	require.True(t, errRow.AwaitingResult, "the production errand row must still owe its parent an outcome: %+v", errRow)
	seedExecutorRunning(t, repo, started.Errand.ID, taskID, "mock-launch-"+started.Errand.ID)
	return &parentSettlementFailureFixture{
		repo: repo, service: service, taskRepo: taskRepo, agentManager: manager, errands: errService,
		store: store, taskID: taskID, parentID: parentID, turnID: turnID, errandID: started.Errand.ID,
		event: watcher.AgentEventData{
			TaskID: taskID, SessionID: parentID, TurnID: turnID, AgentExecutionID: manager.currentPromptExecutionID,
			DeliveryStreamID: streamID, DeliveryIncarnationID: incarnationID,
			DeliveryHarnessGeneration: 1, DeliverySequence: 1, DeliverySubmissionID: submissionID,
		},
	}
}

// @covers AC-TASKS-ERRANDS-006.1 AC-TASKS-ERRANDS-006.5 AC-TASKS-ERRANDS-007.5
func TestErrandParentSettlementReadFailureNeverMovesOrReviewsTask(t *testing.T) {
	cases := []struct {
		name string
		run  func(*parentSettlementFailureFixture) error
	}{
		{name: "agent.ready", run: func(f *parentSettlementFailureFixture) error {
			f.service.handleAgentReady(context.Background(), f.event)
			return nil
		}},
		{name: "agent.completed", run: func(f *parentSettlementFailureFixture) error {
			f.service.handleAgentCompleted(context.Background(), f.event)
			waitForStopCall(t, f.agentManager)
			return nil
		}},
		{name: "recoverable_failure", run: func(f *parentSettlementFailureFixture) error {
			runParentFixtureRecoverableFailure(f)
			return nil
		}},
		{name: "CancelAgent", run: func(f *parentSettlementFailureFixture) error {
			return f.service.CancelAgent(context.Background(), f.parentID)
		}},
		{name: "CancelTurn", run: func(f *parentSettlementFailureFixture) error {
			return f.service.CancelTurn(context.Background(), f.parentID, f.turnID)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newParentSettlementFailureFixture(t, tc.name)
			f.store.failNextParentRead()
			require.NoError(t, tc.run(f))

			task, err := f.repo.GetTask(context.Background(), f.taskID)
			require.NoError(t, err)
			require.Equal(t, "step1", task.WorkflowStepID, "unreadable errands must not apply the moving workflow")
			require.Equal(t, v1.TaskStateInProgress, task.State)
			require.Empty(t, f.taskRepo.stateHistory[f.taskID], "unreadable errands must not write task review state")
			require.Equal(t, models.TaskSessionStateWaitingForInput, mustGetSession(t, f.repo, f.parentID).State)
			require.Equal(t, 1, f.store.failures(), "the settlement read fault must be exercised at the Store boundary")
			stored, err := f.repo.GetAgentDeliveryEffect(context.Background(), workflowEffectKey(f.turnID))
			require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryEffectNotFound, "the exact completion must retain no workflow receipt")
			require.Nil(t, stored)
			errRow, err := f.repo.GetErrand(context.Background(), f.errandID)
			require.NoError(t, err)
			require.True(t, errRow.AwaitingResult, "the outstanding errand must remain unchanged")
			child, err := f.repo.GetExecutorRunningBySessionID(context.Background(), f.errandID)
			require.NoError(t, err)
			require.NotNil(t, child, "the live child must have a runtime to exercise stop isolation")
			f.agentManager.mu.Lock()
			stops := slices.Clone(f.agentManager.stopAgentWithReasonArgs)
			f.agentManager.mu.Unlock()
			for _, stopped := range stops {
				require.NotEqual(t, child.AgentExecutionID, stopped.ExecutionID, "parent settlement failure must not stop the errand")
			}
			session := mustGetSession(t, f.repo, f.parentID)
			_, hasSignal := models.LoadPendingStepSignal(session.Metadata)
			require.False(t, hasSignal, "read failure must not synthesize step completion")

			settleParentFixtureErrand(t, f)
			completeNextParentFixtureTurn(t, f)
			stored, err = f.repo.GetAgentDeliveryEffect(context.Background(), workflowEffectKey(f.turnID))
			require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryEffectNotFound, "a later completion must not resolve the unreadable original as successful")
			require.Nil(t, stored)
		})
	}
}

func settleParentFixtureErrand(t *testing.T, f *parentSettlementFailureFixture) {
	t.Helper()
	ctx := context.Background()
	row, err := f.repo.GetErrand(ctx, f.errandID)
	require.NoError(t, err)
	require.NoError(t, f.repo.CompleteTurn(ctx, row.FirstTurnID))
	setSessionState(t, ctx, f.repo, f.errandID, models.TaskSessionStateWaitingForInput)
	require.NoError(t, f.errands.OnErrandTurnSettled(ctx, errands.TurnSettlement{
		SessionID: row.ID, TurnID: row.FirstTurnID, SubmissionID: row.SubmissionID,
		Outcome: models.ErrandOutcomeFailed, FailureSummary: "finished child outcome",
	}))
	result, err := f.errands.Await(ctx, errands.AwaitRequest{
		ParentSessionID: f.parentID, ErrandIDs: []string{f.errandID}, Mode: "all", Token: "finish-" + row.ID,
	})
	require.NoError(t, err)
	require.Len(t, result.Errands, 1)
	outstanding, err := f.errands.ParentOutstanding(ctx, f.parentID)
	require.NoError(t, err)
	require.False(t, outstanding)
}

func completeNextParentFixtureTurn(t *testing.T, f *parentSettlementFailureFixture) {
	t.Helper()
	ctx := context.Background()
	nextTurnID := "next-" + f.turnID
	setSessionState(t, ctx, f.repo, f.parentID, models.TaskSessionStateRunning)
	require.NoError(t, f.repo.CreateTurn(ctx, &models.Turn{
		ID: nextTurnID, TaskID: f.taskID, TaskSessionID: f.parentID, StartedAt: time.Now().UTC(),
	}))
	f.service.handleAgentReady(ctx, watcher.AgentEventData{
		TaskID: f.taskID, SessionID: f.parentID, TurnID: nextTurnID,
	})
	task, err := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)
	require.Equal(t, "terminal", task.WorkflowStepID, "a later readable empty completion must still run the moving workflow")
	effect, err := f.repo.GetAgentDeliveryEffect(ctx, workflowEffectKey(nextTurnID))
	require.NoError(t, err)
	require.Equal(t, "applied", effect.Outcome, "workflow receipt must belong to the new turn")
}

// @covers AC-TASKS-ERRANDS-006.2 AC-TASKS-ERRANDS-012.2
func TestErrandParentSettlementReadableAndDisabledControls(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		for _, path := range []string{"agent.ready", "agent.completed", "recoverable_failure", "CancelAgent", "CancelTurn"} {
			t.Run(fmt.Sprintf("%s/disabled=%t", path, disabled), func(t *testing.T) {
				f := newParentSettlementFailureFixture(t, path)
				if disabled {
					f.errands.SetEnabled(func() bool { return false })
					f.store.failNextParentRead()
				} else {
					settleParentFixtureErrand(t, f)
				}
				switch path {
				case "agent.ready":
					f.service.handleAgentReady(context.Background(), f.event)
				case "agent.completed":
					f.service.handleAgentCompleted(context.Background(), f.event)
					waitForStopCall(t, f.agentManager)
				case "recoverable_failure":
					runParentFixtureRecoverableFailure(f)
				case "CancelAgent":
					require.NoError(t, f.service.CancelAgent(context.Background(), f.parentID))
				case "CancelTurn":
					require.NoError(t, f.service.CancelTurn(context.Background(), f.parentID, f.turnID))
					completeNextParentFixtureTurn(t, f)
				}
				task, err := f.repo.GetTask(context.Background(), f.taskID)
				require.NoError(t, err)
				require.Equal(t, "terminal", task.WorkflowStepID)
				require.Zero(t, f.store.failures(), "disabled settlement must not read the store")
			})
		}
	}
}

func runParentFixtureRecoverableFailure(f *parentSettlementFailureFixture) {
	ctx := context.Background()
	dispatch := f.service.handleRecoverableFailureLockedState(ctx, f.event, "recoverable_agent_failure")
	if dispatch != nil {
		dispatch(ctx)
	}
}
