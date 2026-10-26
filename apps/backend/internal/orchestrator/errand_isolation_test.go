package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
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
