package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func seedErrandParentAndErrand(
	t *testing.T, repo *sqliterepo.Repository, taskID string, parentPrimary bool, errandState models.TaskSessionState,
) {
	t.Helper()
	ctx := t.Context()
	now := time.Now().UTC()
	must(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "parent", TaskID: taskID, State: models.TaskSessionStateWaitingForInput, IsPrimary: parentPrimary,
		StartedAt: now, UpdatedAt: now,
	}))
	row := &models.Errand{
		ID: "errand", TaskID: taskID, ParentSessionID: "parent", Objective: "Inspect the workspace",
		FirstTurnID: "errand-turn", SubmissionID: "errand-submission", AgentProfileID: "errand-profile",
		Lifecycle: models.ErrandLifecycleLaunched, ResultState: models.ErrandResultNone,
		CreatedAt: now.Add(time.Minute),
	}
	switch errandState {
	case models.TaskSessionStateStarting:
		row.Lifecycle = models.ErrandLifecycleQueued
		row.AwaitingResult = true
	case models.TaskSessionStateRunning:
		row.AwaitingResult = true
	default:
		row.LastOutcome = models.ErrandOutcomeDone
		row.LatestSettledTurnID = row.FirstTurnID
	}
	must(t, repo.CreateErrand(ctx, row))
	if errandState == models.TaskSessionStateWaitingForInput {
		return
	}
	descriptor, err := repo.ReserveErrandExecutionGeneration(ctx, row.ID, 0)
	must(t, err)
	if errandState == models.TaskSessionStateStarting {
		return
	}
	bound, err := repo.BindErrandExecution(ctx, row.ID, descriptor.Generation, "errand-execution")
	must(t, err)
	require.True(t, bound)
	must(t, repo.CreateErrandTurn(ctx, row.ID, &models.Turn{ID: row.FirstTurnID, TaskID: taskID}))
	binding := &models.ErrandHarnessBinding{
		ErrandID: row.ID, Generation: descriptor.Generation, ExecutionID: "errand-execution",
		IncarnationID: "errand-incarnation", HarnessGeneration: 1, StreamID: "errand-stream", NativeSessionID: "native-errand",
	}
	must(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	payload := []byte("Inspect the workspace")
	_, err = repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{
		ErrandID: row.ID, Generation: binding.Generation, ExecutionID: binding.ExecutionID,
		IncarnationID: binding.IncarnationID, HarnessGeneration: binding.HarnessGeneration, StreamID: binding.StreamID,
		NativeSessionID: binding.NativeSessionID, PromptGeneration: 1, TurnID: row.FirstTurnID,
		Submission: &models.AgentDeliverySubmission{ID: row.SubmissionID, PayloadHash: journal.SubmissionHash(payload), Payload: payload},
	})
	must(t, err)
}

func promotedTaskMovedSessionID(t *testing.T, eventBus *MockEventBus, taskID string) (string, bool) {
	t.Helper()
	for _, event := range eventBus.GetPublishedEvents() {
		data, ok := event.Data.(map[string]interface{})
		if event.Type == events.TaskMoved && ok && data["task_id"] == taskID {
			sessionID, _ := data["session_id"].(string)
			return sessionID, true
		}
	}
	return "", false
}

// @covers AC-TASKS-ERRANDS-005.3
func TestService_FeederPromotionSkipsErrandAsCandidateSession(t *testing.T) {
	svc, eventBus, repo := setupFeederPromotionSessionTest(t)
	seedErrandParentAndErrand(t, repo, "task-promoted", false, models.TaskSessionStateWaitingForInput)
	eventBus.ClearEvents()

	if _, err := svc.MoveTask(context.Background(), "task-vacating", "wf-target", "step-target", 0); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	sessionID, moved := promotedTaskMovedSessionID(t, eventBus, "task-promoted")
	if !moved {
		t.Fatal("promoted task.moved event not published")
	}
	if sessionID != "parent" {
		t.Fatalf("promoted task session_id = %q, want the parent over the newer errand", sessionID)
	}
}

// @covers AC-TASKS-ERRANDS-005.4
func TestService_FeederPromotionBlocksOnRunningErrand(t *testing.T) {
	svc, eventBus, repo := setupFeederPromotionSessionTest(t)
	seedErrandParentAndErrand(t, repo, "task-promoted", true, models.TaskSessionStateRunning)
	eventBus.ClearEvents()

	if _, err := svc.MoveTask(context.Background(), "task-vacating", "wf-target", "step-target", 0); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	promoted, err := repo.GetTask(context.Background(), "task-promoted")
	if err != nil {
		t.Fatalf("GetTask(task-promoted): %v", err)
	}
	if promoted.WorkflowStepID != "step-feeder" {
		t.Fatalf("promoted task step = %q, want step-feeder while its errand runs", promoted.WorkflowStepID)
	}
}

// @covers AC-TASKS-ERRANDS-005.3
func TestResolvePrimaryOrActiveSessionSkipsErrand(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	createMoveTask(t, ctx, repo, "task-errand", "wf-source", "step-source", nil)
	seedErrandParentAndErrand(t, repo, "task-errand", false, models.TaskSessionStateRunning)

	session := svc.resolvePrimaryOrActiveSession(ctx, "task-errand")
	if session == nil || session.ID != "parent" {
		t.Fatalf("resolvePrimaryOrActiveSession = %v, want the parent over the newer running errand", session)
	}
	must(t, repo.UpdateTaskSessionState(ctx, "parent", models.TaskSessionStateCompleted, ""))
	if session := svc.resolvePrimaryOrActiveSession(ctx, "task-errand"); session != nil {
		t.Fatalf("resolvePrimaryOrActiveSession = %q, want none when only an errand is active", session.ID)
	}
}

// @covers AC-TASKS-ERRANDS-005.4
func TestValidateMoveSessionsBlocksOnRunningErrand(t *testing.T) {
	for _, errandState := range []models.TaskSessionState{models.TaskSessionStateStarting, models.TaskSessionStateRunning} {
		t.Run(string(errandState), func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			seedMoveWorkflows(t, ctx, repo)
			createMoveTask(t, ctx, repo, "task-errand", "wf-source", "step-source", nil)
			seedErrandParentAndErrand(t, repo, "task-errand", true, errandState)

			for _, allowPrimary := range []bool{false, true} {
				err := svc.validateMoveSessions(ctx, "task-errand", MoveTaskOptions{AllowActivePrimarySession: allowPrimary})
				if err == nil {
					t.Fatalf("validateMoveSessions(allowPrimary=%t) = nil, want a block while the errand is %s", allowPrimary, errandState)
				}
			}
		})
	}
	t.Run("idle errand", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		ctx := context.Background()
		seedMoveWorkflows(t, ctx, repo)
		createMoveTask(t, ctx, repo, "task-errand", "wf-source", "step-source", nil)
		seedErrandParentAndErrand(t, repo, "task-errand", true, models.TaskSessionStateWaitingForInput)
		if err := svc.validateMoveSessions(ctx, "task-errand", MoveTaskOptions{}); err != nil {
			t.Fatalf("validateMoveSessions = %v, want nil for an idle errand", err)
		}
	})
}
