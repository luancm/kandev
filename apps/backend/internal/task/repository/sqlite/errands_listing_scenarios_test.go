package sqlite

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type liveCountRow struct {
	id        string
	taskID    string
	parentID  string
	lifecycle models.ErrandLifecycle
	outcome   models.ErrandOutcome
	allocated bool
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
		{"e-queued", taskA, "parent-a", models.ErrandLifecycleQueued, "", false},
		{"e-running", taskA, "parent-a", models.ErrandLifecycleLaunched, "", true},
		{"e-allocated", taskA, "parent-a2", models.ErrandLifecycleLaunched, "", true},
		{"e-unsettled", taskA, "parent-a2", models.ErrandLifecycleLaunched, "", true},
		{"e-done", taskA, "parent-a", models.ErrandLifecycleLaunched, models.ErrandOutcomeDone, false},
		{"e-failed", taskA, "parent-a", models.ErrandLifecycleLaunched, models.ErrandOutcomeFailed, false},
		{"e-unallocated", taskA, "parent-a", models.ErrandLifecycleLaunched, "", false},
		{"e-cancelled", taskA, "parent-a", models.ErrandLifecycleCancelled, "", false},
		{"e-queued-b", taskB, "parent-b", models.ErrandLifecycleQueued, "", false},
	}
	for index, row := range rows {
		errand := sampleErrand(row.id, row.taskID, row.parentID, errandScenarioBase.Add(time.Duration(index)*time.Minute))
		errand.Lifecycle, errand.LastOutcome = row.lifecycle, row.outcome
		mustCreateErrand(t, repo, errand)
		if row.allocated {
			if _, err := repo.ReserveErrandExecutionGeneration(ctx, errand.ID, 0); err != nil {
				t.Fatalf("reserve Errand execution %s: %v", errand.ID, err)
			}
		}
	}

	for taskID, want := range map[string]int{taskA: 4, taskB: 1, "task-without-errands": 0} {
		if got, err := repo.CountLiveErrandsByTask(ctx, taskID); err != nil || got != want {
			t.Fatalf("CountLiveErrandsByTask(%s) = %d, %v; want %d", taskID, got, err, want)
		}
	}
	assertErrandList(t, "by task", func() ([]*models.Errand, error) { return repo.ListErrandsByTask(ctx, taskA) },
		"e-queued", "e-running", "e-allocated", "e-unsettled", "e-done", "e-failed", "e-unallocated", "e-cancelled")
	assertErrandList(t, "by parent", func() ([]*models.Errand, error) { return repo.ListErrandsByParent(ctx, "parent-a2") },
		"e-allocated", "e-unsettled")
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

func exerciseErrandDeletion(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	taskID := newErrandTask(t, repo, "delete")
	otherTaskID := newErrandTask(t, repo, "ordinary-delete")
	const errandID, parentID = "errand-delete", "parent-delete"
	createErrandScenarioSession(t, repo, &models.TaskSession{ID: parentID, TaskID: taskID})
	ordinary := &models.TaskSession{ID: errandID, TaskID: otherTaskID}
	createErrandScenarioSession(t, repo, ordinary)
	row := sampleErrand(errandID, taskID, parentID, errandScenarioBase)
	row.Lifecycle = models.ErrandLifecycleLaunched
	mustCreateErrand(t, repo, row)
	const queuedID = "errand-queued-delete"
	mustCreateErrand(t, repo, sampleErrand(queuedID, taskID, parentID, errandScenarioBase.Add(time.Minute)))
	turn := &models.Turn{ID: row.FirstTurnID}
	require.NoError(t, repo.CreateErrandTurn(ctx, row.ID, turn))
	message := &models.Message{ID: "owned-output", TurnID: turn.ID, AuthorType: models.MessageAuthorAgent, Content: "output"}
	require.NoError(t, repo.CreateErrandMessage(ctx, row.ID, message))

	require.NoError(t, repo.DeleteTaskSession(ctx, ordinary))
	_, err := repo.GetTaskSession(ctx, ordinary.ID)
	require.ErrorIs(t, err, models.ErrTaskSessionNotFound)
	got, err := repo.GetErrand(ctx, row.ID)
	require.NoError(t, err, "ordinary session deletion removed its independent Errand")
	assertErrandFields(t, got, row)
	_, err = repo.GetErrandMessage(ctx, row.ID, message.ID)
	require.NoError(t, err)
	_, err = repo.GetTaskSession(ctx, parentID)
	require.NoError(t, err)

	require.NoError(t, repo.DeleteErrand(ctx, row.ID))
	_, err = repo.GetErrand(ctx, row.ID)
	require.ErrorIs(t, err, models.ErrErrandNotFound)
	_, err = repo.GetErrandTurn(ctx, row.ID, turn.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = repo.GetErrandMessage(ctx, row.ID, message.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = repo.GetErrand(ctx, queuedID)
	require.NoError(t, err)

	require.NoError(t, repo.DeleteTask(ctx, taskID))
	_, err = repo.GetErrand(ctx, queuedID)
	require.ErrorIs(t, err, models.ErrErrandNotFound)
}
