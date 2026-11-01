package sqlite

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
	"github.com/mattn/go-sqlite3"
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
	failingRepo.ro.SetMaxOpenConns(1)
	failingRepo.ro.SetMaxIdleConns(1)
	var selects atomic.Int32
	connection, err := failingRepo.ro.Connx(context.Background())
	if err != nil {
		t.Fatalf("reserve failing SQLite reader connection: %v", err)
	}
	if err := connection.Raw(func(driverConnection any) error {
		sqliteConnection, ok := driverConnection.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("reader connection = %T, want SQLiteConn", driverConnection)
		}
		sqliteConnection.RegisterAuthorizer(func(operation int, _, _, _ string) int {
			if operation == sqlite3.SQLITE_SELECT && selects.Add(1) == 2 {
				return sqlite3.SQLITE_DENY
			}
			return sqlite3.SQLITE_OK
		})
		return nil
	}); err != nil {
		_ = connection.Close()
		t.Fatalf("enable failing SQLite authorizer: %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("release failing SQLite reader connection: %v", err)
	}
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
	repo.ro.SetMaxOpenConns(1)
	repo.ro.SetMaxIdleConns(1)

	var queryCount atomic.Int32
	connection, err := repo.ro.Connx(context.Background())
	if err != nil {
		t.Fatalf("reserve SQLite reader connection: %v", err)
	}
	if err := connection.Raw(func(driverConnection any) error {
		sqliteConnection, ok := driverConnection.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("reader connection = %T, want SQLiteConn", driverConnection)
		}
		sqliteConnection.RegisterAuthorizer(func(operation int, _, _, _ string) int {
			if operation == sqlite3.SQLITE_SELECT {
				queryCount.Add(1)
			}
			return sqlite3.SQLITE_OK
		})
		return nil
	}); err != nil {
		_ = connection.Close()
		t.Fatalf("enable SQLite statement trace: %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("release traced SQLite reader connection: %v", err)
	}

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
