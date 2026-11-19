package service_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	internaldb "github.com/kandev/kandev/internal/db"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type countingSessionSummaryRepository struct {
	*sqliterepo.Repository
	batchSummaryCalls int
	listSummaryCalls  int
	fullBatchCalls    int
	fullListCalls     int
}

type contextMetadataFallbackRepository struct {
	repository.SessionRepository
	sessionsByTask map[string][]*models.TaskSession
}

func newSessionSummaryTestService(
	t *testing.T,
	wrapSessions func(*sqliterepo.Repository) repository.SessionRepository,
) (*taskservice.Service, *sqliterepo.Repository) {
	t.Helper()
	database, err := internaldb.OpenSQLite(filepath.Join(t.TempDir(), "session-summary.db"))
	if err != nil {
		t.Fatalf("open session summary test database: %v", err)
	}
	sqlxDB := sqlx.NewDb(database, "sqlite3")
	t.Cleanup(func() {
		if err := sqlxDB.Close(); err != nil {
			t.Errorf("close session summary test database: %v", err)
		}
	})
	repo, err := sqliterepo.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("create session summary test repository: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("create session summary test logger: %v", err)
	}
	svc := taskservice.NewService(taskservice.Repos{
		Tasks: repo, Sessions: wrapSessions(repo),
	}, nil, log, taskservice.RepositoryDiscoveryConfig{})
	return svc, repo
}

func (r *contextMetadataFallbackRepository) BatchGetSessionsByTaskIDs(
	_ context.Context,
	taskIDs []string,
) (map[string][]*models.TaskSession, error) {
	result := make(map[string][]*models.TaskSession, len(taskIDs))
	for _, taskID := range taskIDs {
		result[taskID] = r.sessionsByTask[taskID]
	}
	return result, nil
}

func (r *contextMetadataFallbackRepository) ListTaskSessions(
	_ context.Context,
	taskID string,
) ([]*models.TaskSession, error) {
	return r.sessionsByTask[taskID], nil
}

func (r *countingSessionSummaryRepository) BatchGetTaskSessionSummaryObservations(
	ctx context.Context,
	taskIDs []string,
) (map[string][]*models.TaskSessionSummaryObservation, error) {
	r.batchSummaryCalls++
	return r.Repository.BatchGetTaskSessionSummaryObservations(ctx, taskIDs)
}

func (r *countingSessionSummaryRepository) ListTaskSessionSummaryObservations(
	ctx context.Context,
	taskID string,
) ([]*models.TaskSessionSummaryObservation, error) {
	r.listSummaryCalls++
	return r.Repository.ListTaskSessionSummaryObservations(ctx, taskID)
}

func (r *countingSessionSummaryRepository) BatchGetSessionsByTaskIDs(
	ctx context.Context,
	taskIDs []string,
) (map[string][]*models.TaskSession, error) {
	r.fullBatchCalls++
	return r.Repository.BatchGetSessionsByTaskIDs(ctx, taskIDs)
}

func (r *countingSessionSummaryRepository) ListTaskSessions(
	ctx context.Context,
	taskID string,
) ([]*models.TaskSession, error) {
	r.fullListCalls++
	return r.Repository.ListTaskSessions(ctx, taskID)
}

func TestSessionSummaryServiceUsesNarrowReads(t *testing.T) {
	ctx := context.Background()
	var reader *countingSessionSummaryRepository
	svc, repo := newSessionSummaryTestService(t, func(repo *sqliterepo.Repository) repository.SessionRepository {
		reader = &countingSessionSummaryRepository{Repository: repo}
		return reader
	})
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-1", Title: "Session summary"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-summary-service", TaskID: "task-1", IsPrimary: true,
		State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := repo.CreateErrand(ctx, &models.Errand{
		ID: "errand-summary-service", TaskID: "task-1", ParentSessionID: "session-summary-service",
		Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone,
	}); err != nil {
		t.Fatalf("create errand: %v", err)
	}
	batch, err := svc.BatchGetTaskSessionSummaryObservations(ctx, []string{"task-1"})
	if err != nil {
		t.Fatalf("batch session summary observations: %v", err)
	}
	if len(batch["task-1"]) != 1 {
		t.Fatalf("batch summary observations = %+v", batch)
	}
	listed, err := svc.ListTaskSessionSummaryObservations(ctx, "task-1")
	if err != nil {
		t.Fatalf("list session summary observations: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed summary observations = %+v", listed)
	}
	ordinary, err := svc.ListOrdinaryTaskSessionSummaryObservations(ctx, "task-1")
	if err != nil {
		t.Fatalf("list ordinary session summary observations: %v", err)
	}
	if len(ordinary) != 1 || ordinary[0].ID != "session-summary-service" {
		t.Fatalf("ordinary session summary observations = %+v", ordinary)
	}
	if reader.batchSummaryCalls != 1 || reader.listSummaryCalls != 2 || reader.fullBatchCalls != 0 || reader.fullListCalls != 0 {
		t.Fatalf("summary/full read counts = batch %d/list %d/full batch %d/full list %d",
			reader.batchSummaryCalls, reader.listSummaryCalls, reader.fullBatchCalls, reader.fullListCalls)
	}
}

func newFallbackSessionSummaryFixture(t *testing.T) (*taskservice.Service, []*models.TaskSession) {
	t.Helper()
	var fallback *contextMetadataFallbackRepository
	svc, _ := newSessionSummaryTestService(t, func(repo *sqliterepo.Repository) repository.SessionRepository {
		fallback = &contextMetadataFallbackRepository{SessionRepository: repo}
		return fallback
	})
	sessions := []*models.TaskSession{
		{
			ID: "fallback-summary-measured", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput,
			Metadata: map[string]interface{}{
				models.SessionMetaKeyContextWindow: map[string]interface{}{
					"size": 100, "used": 68, "remaining": 32, "efficiency": 68,
					"source": "acp", "timestamp": "2026-10-30T10:00:00Z",
					"provider_catalogue": map[string]interface{}{"extra": true},
				},
				models.SessionMetaKeyContextCompactionCount: 1,
				"unrelated": "excluded",
			},
		},
		{
			ID: "fallback-summary-clear", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput,
			Metadata: map[string]interface{}{models.SessionMetaKeyContextWindow: nil},
		},
		{
			ID: "fallback-summary-missing", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput,
			Metadata: map[string]interface{}{models.SessionMetaKeyContextCompactionCount: "invalid"},
		},
		{
			ID: "fallback-summary-invalid", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput,
			Metadata: map[string]interface{}{models.SessionMetaKeyContextWindow: "invalid"},
		},
		{
			ID: "fallback-summary-peer", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput,
		},
	}
	fallback.sessionsByTask = map[string][]*models.TaskSession{"task-1": sessions}
	return svc, sessions
}

func TestSessionSummaryFallbackPreservesContextMetadata(t *testing.T) {
	var nilObservation *models.TaskSessionSummaryObservation
	if session := nilObservation.ToTaskSession(); session != nil {
		t.Fatalf("nil fallback summary converted to a session: %+v", session)
	}
	svc, sessions := newFallbackSessionSummaryFixture(t)
	batch, err := svc.BatchGetTaskSessionSummaryObservations(t.Context(), []string{"task-1"})
	if err != nil {
		t.Fatalf("batch fallback summaries: %v", err)
	}
	list, err := svc.ListTaskSessionSummaryObservations(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("list fallback summaries: %v", err)
	}
	if len(batch["task-1"]) != len(sessions) || len(list) != len(sessions) {
		t.Fatalf("fallback summary counts: batch=%d list=%d want=%d", len(batch["task-1"]), len(list), len(sessions))
	}
	assertFallbackContextMetadata(t, batch["task-1"])
	assertFallbackContextMetadata(t, list)
}

func assertFallbackContextMetadata(t *testing.T, observations []*models.TaskSessionSummaryObservation) {
	t.Helper()
	wantMeasured := map[string]interface{}{
		"size": 100, "used": 68, "remaining": 32, "efficiency": 68,
		"source": "acp", "timestamp": "2026-10-30T10:00:00Z",
	}
	for _, observation := range observations {
		if observation.ID == "fallback-summary-measured" {
			if !reflect.DeepEqual(observation.Metadata[models.SessionMetaKeyContextWindow], wantMeasured) {
				t.Errorf("measured fallback projection = %#v, want %#v", observation.Metadata[models.SessionMetaKeyContextWindow], wantMeasured)
			}
			if observation.Metadata[models.SessionMetaKeyContextCompactionCount] != 1 {
				t.Errorf("fallback compaction count = %#v, want 1", observation.Metadata[models.SessionMetaKeyContextCompactionCount])
			}
			if _, exists := observation.Metadata["unrelated"]; exists {
				t.Error("fallback summary included unrelated metadata")
			}
			continue
		}
		if observation.ID == "fallback-summary-clear" || observation.ID == "fallback-summary-invalid" {
			if value, exists := observation.Metadata[models.SessionMetaKeyContextWindow]; !exists || value != nil {
				t.Errorf("%s context clear = %#v, exists=%t; want explicit null", observation.ID, value, exists)
			}
			continue
		}
		if _, exists := observation.Metadata[models.SessionMetaKeyContextWindow]; exists {
			t.Errorf("missing context was projected for %s: %#v", observation.ID, observation.Metadata[models.SessionMetaKeyContextWindow])
		}
		if _, exists := observation.Metadata[models.SessionMetaKeyContextCompactionCount]; exists {
			t.Errorf("invalid compaction count was projected for %s: %#v", observation.ID, observation.Metadata[models.SessionMetaKeyContextCompactionCount])
		}
	}
}
