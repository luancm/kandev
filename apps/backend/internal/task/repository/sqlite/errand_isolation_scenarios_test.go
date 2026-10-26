package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func newErrandIsolationTask(t *testing.T, repo *Repository, suffix string) (taskID, parentID, errandID string) {
	t.Helper()
	taskID = newErrandTask(t, repo, suffix)
	parentID, errandID = "parent-"+suffix, "errand-"+suffix
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: parentID, TaskID: taskID, IsPrimary: true,
		State: models.TaskSessionStateWaitingForInput, StartedAt: errandScenarioBase,
	})
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: errandID, TaskID: taskID, Role: models.SessionRoleErrand, ParentSessionID: parentID,
		State: models.TaskSessionStateRunning, StartedAt: errandScenarioBase.Add(time.Minute),
	})
	return taskID, parentID, errandID
}

func requirePrimarySession(t *testing.T, repo *Repository, taskID, want string) {
	t.Helper()
	primary, err := repo.GetPrimarySessionByTaskID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetPrimarySessionByTaskID: %v", err)
	}
	if primary.ID != want {
		t.Fatalf("primary session = %q, want %q", primary.ID, want)
	}
}

// exerciseErrandNeverPrimary proves that an errand row is never inserted or
// promoted as primary, while ordinary sessions keep their promotion behavior.
func exerciseErrandNeverPrimary(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	taskID := newErrandTask(t, repo, "primary")
	errand := &models.TaskSession{
		ID: "errand-primary", TaskID: taskID, Role: models.SessionRoleErrand, ParentSessionID: "parent-primary",
		IsPrimary: true, State: models.TaskSessionStateRunning, StartedAt: errandScenarioBase,
	}
	createErrandScenarioSession(t, repo, errand)
	if errand.IsPrimary {
		t.Fatal("CreateTaskSession left IsPrimary=true on an errand model")
	}
	stored, err := repo.GetTaskSession(ctx, errand.ID)
	if err != nil {
		t.Fatalf("GetTaskSession errand: %v", err)
	}
	if stored.IsPrimary {
		t.Fatal("an errand inserted with IsPrimary=true was stored as primary")
	}

	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: "parent-primary", TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
		StartedAt: errandScenarioBase.Add(-time.Minute),
	})
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: "sibling-primary", TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
		StartedAt: errandScenarioBase.Add(time.Minute),
	})
	if err := repo.SetSessionPrimary(ctx, "parent-primary"); err != nil {
		t.Fatalf("SetSessionPrimary ordinary session: %v", err)
	}
	if err := repo.SetSessionPrimary(ctx, errand.ID); !errors.Is(err, models.ErrErrandSessionPrimary) {
		t.Fatalf("SetSessionPrimary errand = %v, want ErrErrandSessionPrimary", err)
	}
	requirePrimarySession(t, repo, taskID, "parent-primary")
	if promoted, err := repo.SetSessionPrimaryIfNonterminal(ctx, errand.ID); promoted || !errors.Is(err, models.ErrErrandSessionPrimary) {
		t.Fatalf("SetSessionPrimaryIfNonterminal errand = (%v, %v), want (false, ErrErrandSessionPrimary)", promoted, err)
	}
	requirePrimarySession(t, repo, taskID, "parent-primary")

	if err := repo.SetSessionPrimary(ctx, "sibling-primary"); err != nil {
		t.Fatalf("SetSessionPrimary ordinary sibling: %v", err)
	}
	requirePrimarySession(t, repo, taskID, "sibling-primary")
}

// exerciseActiveSessionLookupSkipsErrands proves that the task's active
// session lookup, the fallback target of many task-level paths, never
// returns an errand while ordinary sessions keep the newest-first order.
func exerciseActiveSessionLookupSkipsErrands(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	taskID, parentID, errandID := newErrandIsolationTask(t, repo, "active")
	active, err := repo.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("GetActiveTaskSessionByTaskID: %v", err)
	}
	if active.ID != parentID {
		t.Fatalf("active session = %q, want parent %q over newer errand", active.ID, parentID)
	}

	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: "sibling-active", TaskID: taskID, State: models.TaskSessionStateRunning,
		StartedAt: errandScenarioBase.Add(30 * time.Second),
	})
	active, err = repo.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil || active.ID != "sibling-active" {
		t.Fatalf("active session = (%v, %v), want the newest ordinary session sibling-active", active, err)
	}

	for _, id := range []string{parentID, "sibling-active"} {
		if err := repo.UpdateTaskSessionState(ctx, id, models.TaskSessionStateCompleted, ""); err != nil {
			t.Fatalf("complete %s: %v", id, err)
		}
	}
	if _, err := repo.GetActiveTaskSessionByTaskID(ctx, taskID); !errors.Is(err, models.ErrTaskSessionNotFound) {
		t.Fatalf("lookup with only a live errand = %v, want ErrTaskSessionNotFound", err)
	}
	sessions, err := repo.ListActiveTaskSessionsByTaskID(ctx, taskID)
	if err != nil || len(sessions) != 1 || sessions[0].ID != errandID {
		t.Fatalf("ListActiveTaskSessionsByTaskID = (%v, %v), want only the errand so task stops still reach it", sessions, err)
	}
}

// exerciseErrandNeverClaimsTaskTitle proves that the generated-title handoff
// is never claimed by an errand, and an ordinary session still claims it.
func exerciseErrandNeverClaimsTaskTitle(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	taskID, parentID, errandID := newErrandIsolationTask(t, repo, "title")
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET metadata = ? WHERE id = ?`),
		`{"agent_title_pending":true}`, taskID); err != nil {
		t.Fatalf("seed pending title marker: %v", err)
	}
	owned, newlyClaimed, err := repo.ClaimTaskTitleSession(ctx, taskID, errandID)
	if err != nil || owned || newlyClaimed {
		t.Fatalf("errand title claim = (%v, %v, %v), want (false, false, nil)", owned, newlyClaimed, err)
	}
	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if owner := models.AgentTitleOwnerSessionID(task.Metadata); owner != "" {
		t.Fatalf("title owner after errand claim = %q, want none", owner)
	}
	owned, newlyClaimed, err = repo.ClaimTaskTitleSession(ctx, taskID, parentID)
	if err != nil || !owned || !newlyClaimed {
		t.Fatalf("ordinary title claim = (%v, %v, %v), want (true, true, nil)", owned, newlyClaimed, err)
	}
}
