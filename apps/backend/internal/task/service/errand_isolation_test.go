package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// seedErrandParentAndErrand inserts an ordinary parent session and a newer
// errand of it on taskID.
func seedErrandParentAndErrand(
	t *testing.T, repo *sqliterepo.Repository, taskID string, parentPrimary bool, errandState models.TaskSessionState,
) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	must(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "parent", TaskID: taskID, State: models.TaskSessionStateWaitingForInput, IsPrimary: parentPrimary,
		StartedAt: now, UpdatedAt: now,
	}))
	must(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "errand", TaskID: taskID, Role: models.SessionRoleErrand, ParentSessionID: "parent",
		State: errandState, StartedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute),
	}))
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
