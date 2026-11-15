package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

func TestBatchErrandsSummariesByTaskIDsSQLite(t *testing.T) {
	exerciseBatchErrandsSummaries(t, newRepoForSessionTests(t))
}

func TestPostgresBatchErrandsSummariesByTaskIDs(t *testing.T) {
	exerciseBatchErrandsSummaries(t, newPostgresErrandRepo(t))
}

func TestBatchErrandsSummariesByTaskIDsFailureDoesNotReturnPartialZero(t *testing.T) {
	repo := newRepoForSessionTests(t)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := repo.BatchErrandsSummariesByTaskIDs(canceled, []string{"task-canceled-summary"})
	if !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("canceled batch result = %#v, err = %v; want nil result and context.Canceled", got, err)
	}

	closedRepo := newRepoForSessionTests(t)
	if err := closedRepo.ro.Close(); err != nil {
		t.Fatalf("close database reader: %v", err)
	}
	got, err = closedRepo.BatchErrandsSummariesByTaskIDs(context.Background(), []string{"task-closed-summary"})
	if err == nil || got != nil {
		t.Fatalf("closed-reader batch result = %#v, err = %v; want nil result and an error", got, err)
	}

	failingRepo := newRepoForSessionTests(t)
	activeTaskID := newErrandTask(t, failingRepo, "batch-partial-failure")
	createErrandScenarioSession(t, failingRepo, &models.TaskSession{
		ID: "batch-parent-partial-failure", TaskID: activeTaskID, State: models.TaskSessionStateRunning,
	})
	createErrandSummaryRow(t, failingRepo, activeTaskID, errandSummaryRow{
		id: "batch-child-partial-failure", parentID: "batch-parent-partial-failure",
		lifecycle: models.ErrandLifecycleQueued, resultState: models.ErrandResultNone,
	}, 0)

	var selects atomic.Int32
	gate := newErrandSQLGate("SELECT", false)
	gate.queryFault = func(query string) error {
		if strings.HasPrefix(strings.TrimSpace(query), "SELECT") && selects.Add(1) == 2 {
			return errors.New("second summary query failed")
		}
		return nil
	}
	failingRepo = independentErrandRepository(t, failingRepo, gate)
	taskIDs := make([]string, sqliteMaxHostParams)
	taskIDs[0] = activeTaskID
	for index := 1; index < len(taskIDs); index++ {
		taskIDs[index] = fmt.Sprintf("batch-partial-%03d", index)
	}
	got, err = failingRepo.BatchErrandsSummariesByTaskIDs(context.Background(), taskIDs)
	if err == nil || got != nil || selects.Load() != 2 {
		t.Fatalf("partially failed batch = %#v, err = %v, SELECT statements = %d; want nil result, error, 2 SELECTs", got, err, selects.Load())
	}
}

func TestBatchErrandsSummariesByTaskIDsChunkQueryCountSQLite(t *testing.T) {
	repo := newRepoForSessionTests(t)

	// Count driver queries, not SQLite authorizer callbacks for nested SELECTs.
	var queryCount atomic.Int32
	gate := newErrandSQLGate("SELECT", false)
	gate.queryFault = func(query string) error {
		if strings.HasPrefix(strings.TrimSpace(query), "SELECT") {
			queryCount.Add(1)
		}
		return nil
	}
	repo = independentErrandRepository(t, repo, gate)
	empty, err := repo.BatchErrandsSummariesByTaskIDs(context.Background(), nil)
	if err != nil || len(empty) != 0 || queryCount.Load() != 0 {
		t.Fatalf("empty batch = %#v, err = %v, queries = %d; want empty map, no error, no query", empty, err, queryCount.Load())
	}

	ids := make([]string, sqliteMaxHostParams)
	for index := range ids {
		ids[index] = fmt.Sprintf("batch-query-%03d", index)
	}
	ids = append(ids, ids[0], "")
	got, err := repo.BatchErrandsSummariesByTaskIDs(context.Background(), ids)
	if err != nil {
		t.Fatalf("chunked batch: %v", err)
	}
	if len(got) != sqliteMaxHostParams || queryCount.Load() != 2 {
		t.Fatalf("chunked batch returned %d IDs using %d queries; want %d IDs using 2 queries", len(got), queryCount.Load(), sqliteMaxHostParams)
	}

	queryCount.Store(0)
	ids = ids[:sqliteMaxHostParams-2]
	ids = append(ids, ids[0], "")
	got, err = repo.BatchErrandsSummariesByTaskIDs(context.Background(), ids)
	if err != nil {
		t.Fatalf("deduplicated batch: %v", err)
	}
	if len(got) != sqliteMaxHostParams-2 || queryCount.Load() != 1 {
		t.Fatalf("deduplicated batch returned %d IDs using %d queries; want %d IDs using 1 query", len(got), queryCount.Load(), sqliteMaxHostParams-2)
	}
	for id, summary := range got {
		if summary != (statussummary.ErrandsSummary{}) {
			t.Fatalf("summary[%q] = %+v, want explicit zero", id, summary)
		}
	}
}
