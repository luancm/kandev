package sqlite

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

func exerciseBatchErrandsSummaries(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	activeTaskID := newErrandTask(t, repo, "batch-active")
	quietTaskID := newErrandTask(t, repo, "batch-quiet")
	unrequestedTaskID := newErrandTask(t, repo, "batch-unrequested")

	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: "batch-parent-idle", TaskID: activeTaskID, State: models.TaskSessionStateWaitingForInput,
	})
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: "batch-parent-running", TaskID: activeTaskID, State: models.TaskSessionStateRunning,
	})
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: "batch-parent-unrequested", TaskID: unrequestedTaskID, State: models.TaskSessionStateRunning,
	})

	rows := []errandSummaryRow{
		{
			id: "batch-child-running", parentID: "batch-parent-running",
			lifecycle: models.ErrandLifecycleLaunched, resultState: models.ErrandResultNone,
			sessionState: models.TaskSessionStateRunning,
		},
		{
			id: "batch-child-queued", parentID: "batch-parent-running",
			lifecycle: models.ErrandLifecycleQueued, resultState: models.ErrandResultNone,
		},
		{
			id: "batch-child-failed", parentID: "batch-parent-running",
			lifecycle: models.ErrandLifecycleLaunched, outcome: models.ErrandOutcomeFailed,
			resultState: models.ErrandResultNone, sessionState: models.TaskSessionStateFailed,
		},
		{
			id: "batch-child-missing", parentID: "batch-parent-running",
			lifecycle: models.ErrandLifecycleLaunched, resultState: models.ErrandResultNone,
		},
		{
			id: "batch-child-pending-result", parentID: "batch-parent-idle",
			lifecycle: models.ErrandLifecycleLaunched, outcome: models.ErrandOutcomeDone,
			awaitingResult: true, resultState: models.ErrandResultPending,
			sessionState: models.TaskSessionStateWaitingForInput,
		},
	}
	for index, row := range rows {
		createErrandSummaryRow(t, repo, activeTaskID, row, index)
	}
	createErrandSummaryRow(t, repo, unrequestedTaskID, errandSummaryRow{
		id: "batch-child-unrequested", parentID: "batch-parent-unrequested",
		lifecycle: models.ErrandLifecycleQueued, resultState: models.ErrandResultNone,
	}, len(rows))

	want := map[string]statussummary.ErrandsSummary{
		activeTaskID: {Live: 2, Waiting: true},
		quietTaskID:  {},
	}
	assertBatchErrandsSummaryMap(t, repo, []string{activeTaskID, "", quietTaskID, activeTaskID}, want)

	for taskID, expected := range want {
		got, err := repo.ErrandsSummaryByTask(ctx, taskID)
		if err != nil {
			t.Fatalf("ErrandsSummaryByTask(%s): %v", taskID, err)
		}
		if got != expected {
			t.Fatalf("ErrandsSummaryByTask(%s) = %+v, want %+v", taskID, got, expected)
		}
	}

	settleErrandSummaryRow(t, repo, "batch-child-pending-result", false, models.ErrandResultDelivered)
	want[activeTaskID] = statussummary.ErrandsSummary{Live: 2}
	assertBatchErrandsSummaryMap(t, repo, []string{activeTaskID, quietTaskID}, want)
	got, err := repo.ErrandsSummaryByTask(ctx, activeTaskID)
	if err != nil {
		t.Fatalf("ErrandsSummaryByTask(%s) after result release: %v", activeTaskID, err)
	}
	if got != want[activeTaskID] {
		t.Fatalf("ErrandsSummaryByTask(%s) after result release = %+v, want %+v", activeTaskID, got, want[activeTaskID])
	}

	// The portable chunk boundary reserves one host parameter for parent state.
	ids := make([]string, sqliteMaxHostParams)
	for index := range ids {
		ids[index] = fmt.Sprintf("batch-empty-%03d", index)
	}
	ids = append(ids, ids[0], "")
	chunked, err := repo.BatchErrandsSummariesByTaskIDs(ctx, ids)
	if err != nil {
		t.Fatalf("BatchErrandsSummariesByTaskIDs at chunk boundary: %v", err)
	}
	if len(chunked) != sqliteMaxHostParams {
		t.Fatalf("chunked result length = %d, want %d unique nonempty IDs", len(chunked), sqliteMaxHostParams)
	}
	for _, id := range ids[:sqliteMaxHostParams] {
		if summary, ok := chunked[id]; !ok || summary != (statussummary.ErrandsSummary{}) {
			t.Fatalf("chunked result[%q] = %+v, present = %t, want explicit zero", id, summary, ok)
		}
	}
}

func assertBatchErrandsSummaryMap(
	t *testing.T,
	repo *Repository,
	taskIDs []string,
	want map[string]statussummary.ErrandsSummary,
) {
	t.Helper()
	got, err := repo.BatchErrandsSummariesByTaskIDs(context.Background(), taskIDs)
	if err != nil {
		t.Fatalf("BatchErrandsSummariesByTaskIDs(%v): %v", taskIDs, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BatchErrandsSummariesByTaskIDs(%v) = %#v, want %#v", taskIDs, got, want)
	}
}
