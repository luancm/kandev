package sqlite

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type liveCountRow struct {
	id           string
	taskID       string
	parentID     string
	lifecycle    models.ErrandLifecycle
	outcome      models.ErrandOutcome
	sessionState models.TaskSessionState
}

// exerciseErrandListingAndLiveCount proves list ordering and that the live
// count includes exactly the queued, running, and needs-attention errands.
func exerciseErrandListingAndLiveCount(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	taskA := newErrandTask(t, repo, "lists-a")
	taskB := newErrandTask(t, repo, "lists-b")
	for _, parent := range []struct{ id, taskID string }{
		{"parent-a", taskA}, {"parent-a2", taskA}, {"parent-b", taskB},
	} {
		createErrandScenarioSession(t, repo, &models.TaskSession{ID: parent.id, TaskID: parent.taskID})
	}
	rows := []liveCountRow{
		{"e-queued", taskA, "parent-a", models.ErrandLifecycleQueued, "", ""},
		{"e-running", taskA, "parent-a", models.ErrandLifecycleLaunched, "", models.TaskSessionStateRunning},
		{"e-follow-up", taskA, "parent-a2", models.ErrandLifecycleLaunched, models.ErrandOutcomeDone, models.TaskSessionStateStarting},
		{"e-unsettled", taskA, "parent-a2", models.ErrandLifecycleLaunched, "", models.TaskSessionStateWaitingForInput},
		{"e-done", taskA, "parent-a", models.ErrandLifecycleLaunched, models.ErrandOutcomeDone, models.TaskSessionStateWaitingForInput},
		{"e-session-failed", taskA, "parent-a", models.ErrandLifecycleLaunched, "", models.TaskSessionStateFailed},
		{"e-session-missing", taskA, "parent-a", models.ErrandLifecycleLaunched, "", ""},
		{"e-cancelled", taskA, "parent-a", models.ErrandLifecycleCancelled, "", models.TaskSessionStateCancelled},
		{"e-queued-b", taskB, "parent-b", models.ErrandLifecycleQueued, "", ""},
	}
	for index, row := range rows {
		if row.sessionState != "" {
			createErrandScenarioSession(t, repo, &models.TaskSession{
				ID: row.id, TaskID: row.taskID, State: row.sessionState,
				Role: models.SessionRoleErrand, ParentSessionID: row.parentID,
			})
		}
		errand := sampleErrand(row.id, row.taskID, row.parentID, errandScenarioBase.Add(time.Duration(index)*time.Minute))
		errand.Lifecycle, errand.LastOutcome = row.lifecycle, row.outcome
		mustCreateErrand(t, repo, errand)
	}

	for taskID, want := range map[string]int{taskA: 4, taskB: 1, "task-without-errands": 0} {
		if got, err := repo.CountLiveErrandsByTask(ctx, taskID); err != nil || got != want {
			t.Fatalf("CountLiveErrandsByTask(%s) = %d, %v; want %d", taskID, got, err, want)
		}
	}
	assertErrandList(t, "by task", func() ([]*models.Errand, error) { return repo.ListErrandsByTask(ctx, taskA) },
		"e-queued", "e-running", "e-follow-up", "e-unsettled", "e-done", "e-session-failed", "e-session-missing", "e-cancelled")
	assertErrandList(t, "by parent", func() ([]*models.Errand, error) { return repo.ListErrandsByParent(ctx, "parent-a2") },
		"e-follow-up", "e-unsettled")
	assertErrandList(t, "queued", func() ([]*models.Errand, error) { return repo.ListQueuedErrands(ctx) },
		"e-queued", "e-queued-b")
	assertErrandList(t, "empty task", func() ([]*models.Errand, error) { return repo.ListErrandsByTask(ctx, "task-without-errands") })
}

func assertErrandList(t *testing.T, name string, list func() ([]*models.Errand, error), want ...string) {
	t.Helper()
	errands, err := list()
	if err != nil {
		t.Fatalf("list %s: %v", name, err)
	}
	if got := errandIDs(errands); !reflect.DeepEqual(got, append([]string{}, want...)) {
		t.Fatalf("list %s = %v, want %v", name, got, want)
	}
}

// exerciseErrandDeletion proves that an errand row leaves with its session
// and with its task, and never with an unrelated row.
func exerciseErrandDeletion(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	taskID := newErrandTask(t, repo, "delete")
	createErrandScenarioSession(t, repo, &models.TaskSession{ID: "parent-delete", TaskID: taskID})
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: "errand-delete", TaskID: taskID, Role: models.SessionRoleErrand, ParentSessionID: "parent-delete",
	})
	launched := sampleErrand("errand-delete", taskID, "parent-delete", errandScenarioBase)
	launched.Lifecycle = models.ErrandLifecycleLaunched
	mustCreateErrand(t, repo, launched)
	mustCreateErrand(t, repo, sampleErrand("errand-queued-delete", taskID, "parent-delete", errandScenarioBase.Add(time.Minute)))

	session, err := repo.GetTaskSession(ctx, "errand-delete")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if err := repo.DeleteTaskSession(ctx, session); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}
	if _, err := repo.GetErrand(ctx, "errand-delete"); !errors.Is(err, models.ErrErrandNotFound) {
		t.Fatalf("errand row after session delete: %v, want ErrErrandNotFound", err)
	}
	if _, err := repo.GetErrand(ctx, "errand-queued-delete"); err != nil {
		t.Fatalf("unrelated queued errand removed by session delete: %v", err)
	}

	if err := repo.DeleteErrand(ctx, "errand-queued-delete"); err != nil {
		t.Fatalf("DeleteErrand: %v", err)
	}
	if _, err := repo.GetErrand(ctx, "errand-queued-delete"); !errors.Is(err, models.ErrErrandNotFound) {
		t.Fatalf("errand row after DeleteErrand: %v, want ErrErrandNotFound", err)
	}

	mustCreateErrand(t, repo, sampleErrand("errand-task-delete", taskID, "parent-delete", errandScenarioBase.Add(2*time.Minute)))
	if err := repo.DeleteTask(ctx, taskID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if _, err := repo.GetErrand(ctx, "errand-task-delete"); !errors.Is(err, models.ErrErrandNotFound) {
		t.Fatalf("errand row after task delete: %v, want ErrErrandNotFound", err)
	}
}
