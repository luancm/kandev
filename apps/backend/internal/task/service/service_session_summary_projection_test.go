package service

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
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
	svc, _, repo := createTestService(t)
	createTaskWithoutRepositories(t, ctx, repo)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-summary-service", TaskID: "task-1", IsPrimary: true,
		State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	reader := &countingSessionSummaryRepository{Repository: repo}
	svc.sessions = reader

	batch, err := svc.BatchGetTaskSessionSummaryObservations(ctx, []string{"task-1"})
	if err != nil {
		t.Fatalf("batch session summary observations: %v", err)
	}
	if len(batch["task-1"]) != 1 || batch["task-1"][0].ID != "session-summary-service" {
		t.Fatalf("batch summary observations = %+v", batch)
	}
	listed, err := svc.ListTaskSessionSummaryObservations(ctx, "task-1")
	if err != nil {
		t.Fatalf("list session summary observations: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "session-summary-service" {
		t.Fatalf("listed summary observations = %+v", listed)
	}
	if reader.batchSummaryCalls != 1 || reader.listSummaryCalls != 1 || reader.fullBatchCalls != 0 || reader.fullListCalls != 0 {
		t.Fatalf("summary/full read counts = batch %d/list %d/full batch %d/full list %d",
			reader.batchSummaryCalls, reader.listSummaryCalls, reader.fullBatchCalls, reader.fullListCalls)
	}
}

func TestSessionSummaryFallbackProjectsContextMetadata(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	createTaskWithoutRepositories(t, ctx, repo)
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
	}
	svc.sessions = &contextMetadataFallbackRepository{
		SessionRepository: repo,
		sessionsByTask:    map[string][]*models.TaskSession{"task-1": sessions},
	}

	batch, err := svc.BatchGetTaskSessionSummaryObservations(ctx, []string{"task-1"})
	if err != nil {
		t.Fatalf("batch fallback summaries: %v", err)
	}
	listed, err := svc.ListTaskSessionSummaryObservations(ctx, "task-1")
	if err != nil {
		t.Fatalf("list fallback summaries: %v", err)
	}
	byID := make(map[string]*models.TaskSessionSummaryObservation, len(listed))
	for _, observation := range listed {
		byID[observation.ID] = observation
	}
	if len(batch["task-1"]) != len(sessions) || len(byID) != len(sessions) {
		t.Fatalf("fallback summary counts: batch=%d list=%d want=%d", len(batch["task-1"]), len(byID), len(sessions))
	}
	wantMeasured := map[string]interface{}{
		"size": 100, "used": 68, "remaining": 32, "efficiency": 68,
		"source": "acp", "timestamp": "2026-10-30T10:00:00Z",
	}
	for _, observation := range append(batch["task-1"], listed...) {
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
