package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-TASKS-ERRANDS-001.1 AC-TASKS-ERRANDS-005.2
func TestPrepareSessionErrandRoleParentAndNeverPrimary(t *testing.T) {
	repo := newMockRepository()
	seedPreflightTaskRepository(repo, "task-1", "repo-1", &models.Repository{ID: "repo-1", SourceType: sourceTypeLocal})
	exec := newPreflightTestExecutor(t, repo)
	identity := models.ReservedLaunchIdentity{TaskID: "task-1", SessionID: "errand-1", TurnID: "turn-1", SubmissionID: "sub-1"}
	ctx := WithErrandSession(models.WithReservedLaunchIdentity(context.Background(), identity), "parent-1")

	sessionID, err := exec.PrepareSession(ctx, &v1.Task{ID: "task-1", WorkspaceID: "workspace-1"}, "agent-profile", "", "", "")
	if err != nil {
		t.Fatalf("PrepareSession: %v", err)
	}
	if sessionID != "errand-1" || len(repo.createTaskSessionCalls) != 1 {
		t.Fatalf("session %q created %d rows, want the reserved session once", sessionID, len(repo.createTaskSessionCalls))
	}
	created := repo.createTaskSessionCalls[0]
	if created.Role != models.SessionRoleErrand || created.ParentSessionID != "parent-1" {
		t.Fatalf("role/parent = %q/%q, want errand/parent-1", created.Role, created.ParentSessionID)
	}
	if created.IsPrimary || len(repo.setSessionPrimaryCalls) != 0 {
		t.Fatalf("errand primary=%v, primary writes=%d, want never primary", created.IsPrimary, len(repo.setSessionPrimaryCalls))
	}
}

func TestPrepareSessionWithoutErrandContextKeepsOrdinaryRole(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	if _, err := exec.PrepareSession(context.Background(), &v1.Task{ID: "task-1", WorkspaceID: "workspace-1"}, "profile", "", "", ""); err != nil {
		t.Fatalf("PrepareSession: %v", err)
	}
	created := repo.createTaskSessionCalls[0]
	if created.Role != "" || created.ParentSessionID != "" || !created.IsPrimary {
		t.Fatalf("ordinary session = role %q parent %q primary %v", created.Role, created.ParentSessionID, created.IsPrimary)
	}
}

func TestErrandSessionContextRequiresAParent(t *testing.T) {
	if _, ok := errandParentFromContext(WithErrandSession(context.Background(), "")); ok {
		t.Fatal("an errand context without a parent was accepted")
	}
	if parent, ok := errandParentFromContext(WithErrandSession(context.Background(), "parent-1")); !ok || parent != "parent-1" {
		t.Fatalf("parent = %q, %v", parent, ok)
	}
}

// A launch failure of an errand session settles the session and never writes
// the task's state.
func TestHandleLaunchFailureLeavesTaskStateAloneForErrandSessions(t *testing.T) {
	for _, tc := range []struct {
		name      string
		role      models.SessionRole
		wantTouch int
	}{
		{"ordinary session fails the task", "", 1},
		{"errand session leaves the task alone", models.SessionRoleErrand, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepository()
			repo.tasks["task-1"] = &models.Task{ID: "task-1", State: v1.TaskStateInProgress}
			repo.sessions["session-1"] = &models.TaskSession{
				ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateStarting,
				Role: tc.role, ParentSessionID: parentFor(tc.role),
			}
			exec := newTestExecutor(t, &mockAgentManager{}, repo)

			if err := exec.handleLaunchFailure(context.Background(), "task-1", "session-1", "", "", errors.New("launch failed")); err == nil {
				t.Fatal("handleLaunchFailure returned no error")
			}
			if got := len(repo.updateTaskStateIfNotArchivedCalls); got != tc.wantTouch {
				t.Fatalf("task state writes = %d, want %d", got, tc.wantTouch)
			}
			if repo.sessions["session-1"].State != models.TaskSessionStateFailed {
				t.Fatalf("session state = %s, want FAILED", repo.sessions["session-1"].State)
			}
		})
	}
}

func TestRuntimeTaskStateFallbackSkipsErrandSessions(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["task-1"] = &models.Task{ID: "task-1", State: v1.TaskStateReview}
	repo.sessions["errand-1"] = &models.TaskSession{
		ID: "errand-1", TaskID: "task-1", State: models.TaskSessionStateFailed,
		Role: models.SessionRoleErrand, ParentSessionID: "parent-1",
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	// No reconcile callback: the repository fallback must still refuse.
	if err := exec.writeTaskFailedForRuntime(context.Background(), "task-1", "errand-1"); err != nil {
		t.Fatalf("writeTaskFailedForRuntime: %v", err)
	}
	repo.sessions["errand-1"].State = models.TaskSessionStateRunning
	if err := exec.writeTaskInProgressForRuntime(context.Background(), "task-1", "errand-1"); err != nil {
		t.Fatalf("writeTaskInProgressForRuntime: %v", err)
	}
	if len(repo.updateTaskStateIfNotArchivedCalls) != 0 || repo.tasks["task-1"].State != v1.TaskStateReview {
		t.Fatalf("an errand session wrote task state: %+v", repo.updateTaskStateIfNotArchivedCalls)
	}
}

func parentFor(role models.SessionRole) string {
	if role == models.SessionRoleErrand {
		return "parent-1"
	}
	return ""
}
