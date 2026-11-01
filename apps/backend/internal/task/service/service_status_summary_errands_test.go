package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/statussummary"
	"github.com/stretchr/testify/require"
	"testing"
)

// @covers AC-TASKS-ERRANDS-006.3 AC-TASKS-ERRANDS-010.5 AC-TASKS-ERRANDS-012.2
func TestTaskStatusSummaryErrandsReadRepair(t *testing.T) {
	for _, stale := range []bool{false, true} {
		name := "missing_summary"
		if stale {
			name = "stale_summary"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			svc, bus, repo := createTestService(t)
			createTaskWithoutRepositories(t, ctx, repo)
			task, err := repo.GetTask(ctx, "task-1")
			require.NoError(t, err)
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "parent", TaskID: task.ID, State: models.TaskSessionStateWaitingForInput}))
			require.NoError(t, repo.CreateErrand(ctx, &models.Errand{ID: "child", TaskID: task.ID, ParentSessionID: "parent", Title: "Child", Objective: "Work", AgentProfileID: "profile", FirstTurnID: "turn", SubmissionID: "submission", Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone, AwaitingResult: true}))
			svc.statusSummaries = repo
			svc.SetTaskStatusSummaryErrandsReader(repo.BatchErrandsSummariesByTaskIDs)
			var before map[string]*statussummary.TaskStatusSummary
			if stale {
				value := statussummary.TaskStatusSummary{Revision: 1}
				accepted, writeErr := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{TaskID: task.ID, WorkspaceID: task.WorkspaceID, Summary: value})
				require.NoError(t, writeErr)
				require.True(t, accepted)
				before = map[string]*statussummary.TaskStatusSummary{task.ID: &value}
			}
			bus.ClearEvents()
			got, err := svc.ReconcileTaskStatusSummaries(ctx, []*models.Task{task}, nil, nil, before)
			require.NoError(t, err)
			require.Equal(t, &statussummary.ErrandsSummary{Live: 1, Waiting: true}, got[task.ID].Errands)
			stored, err := repo.LoadTaskStatusSummaries(ctx, []string{task.ID})
			require.NoError(t, err)
			require.Equal(t, got[task.ID], stored[task.ID])
			require.Len(t, bus.GetPublishedEvents(), 1)
		})
	}
}

type errandsSummaryCASRepository struct {
	repository.TaskStatusSummaryRepository
	beforeReject func()
	rejected     bool
}

func (r *errandsSummaryCASRepository) CompareAndUpdateTaskStatusSummary(ctx context.Context, row *statussummary.StoredTaskStatusSummary) (bool, error) {
	if !r.rejected {
		r.rejected = true
		r.beforeReject()
		competing := *row
		competing.Summary.Revision++
		competing.Summary.Errands = nil
		competing.Summary.QueuedPromptCount = 7
		if _, err := r.TaskStatusSummaryRepository.CompareAndUpdateTaskStatusSummary(ctx, &competing); err != nil {
			return false, err
		}
		return false, nil
	}
	return r.TaskStatusSummaryRepository.CompareAndUpdateTaskStatusSummary(ctx, row)
}

func newErrandsSummaryRepairFixture(t *testing.T) (*Service, *MockEventBus, *sqliterepo.Repository, *models.Task) {
	t.Helper()
	ctx := context.Background()
	svc, bus, repo := createTestService(t)
	createTaskWithoutRepositories(t, ctx, repo)
	task, err := repo.GetTask(ctx, "task-1")
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "parent", TaskID: task.ID, State: models.TaskSessionStateWaitingForInput}))
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{ID: "child", TaskID: task.ID, ParentSessionID: "parent", Title: "Child", Objective: "Work", AgentProfileID: "profile", FirstTurnID: "turn", SubmissionID: "submission", Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone, AwaitingResult: true}))
	svc.statusSummaries = repo
	svc.SetTaskStatusSummaryErrandsReader(repo.BatchErrandsSummariesByTaskIDs)
	return svc, bus, repo, task
}

// @covers AC-TASKS-ERRANDS-010.5 AC-TASKS-ERRANDS-012.2
func TestTaskStatusSummaryErrandsRepairCASReloadsObservation(t *testing.T) {
	for _, missing := range []bool{false, true} {
		name := "existing"
		if missing {
			name = "missing"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			svc, bus, repo, task := newErrandsSummaryRepairFixture(t)
			var before map[string]*statussummary.TaskStatusSummary
			if !missing {
				value := statussummary.TaskStatusSummary{Revision: 1}
				accepted, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{TaskID: task.ID, WorkspaceID: task.WorkspaceID, Summary: value})
				require.NoError(t, err)
				require.True(t, accepted)
				before = map[string]*statussummary.TaskStatusSummary{task.ID: &value}
			}
			reads := 0
			svc.SetTaskStatusSummaryErrandsReader(func(ctx context.Context, ids []string) (map[string]statussummary.ErrandsSummary, error) {
				reads++
				return repo.BatchErrandsSummariesByTaskIDs(ctx, ids)
			})
			svc.statusSummaries = &errandsSummaryCASRepository{TaskStatusSummaryRepository: repo, beforeReject: func() {
				require.NoError(t, repo.UpdateTaskSessionState(ctx, "parent", models.TaskSessionStateRunning, ""))
			}}
			bus.ClearEvents()
			got, err := svc.ReconcileTaskStatusSummaries(ctx, []*models.Task{task}, nil, nil, before)
			require.NoError(t, err)
			require.Equal(t, &statussummary.ErrandsSummary{Live: 1}, got[task.ID].Errands)
			require.Equal(t, 7, got[task.ID].QueuedPromptCount)
			require.Equal(t, 2, reads)
			stored, err := repo.LoadTaskStatusSummaries(ctx, []string{task.ID})
			require.NoError(t, err)
			require.Equal(t, got[task.ID], stored[task.ID])
			require.Len(t, bus.GetPublishedEvents(), 1)
		})
	}
}

func TestTaskStatusSummaryErrandsRepairRetryReadFailureKeepsWinner(t *testing.T) {
	for _, missing := range []bool{false, true} {
		name := "existing"
		if missing {
			name = "missing"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			svc, bus, repo, task := newErrandsSummaryRepairFixture(t)
			var before map[string]*statussummary.TaskStatusSummary
			if !missing {
				value := statussummary.TaskStatusSummary{Revision: 1}
				accepted, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{TaskID: task.ID, WorkspaceID: task.WorkspaceID, Summary: value})
				require.NoError(t, err)
				require.True(t, accepted)
				before = map[string]*statussummary.TaskStatusSummary{task.ID: &value}
			}
			reads := 0
			svc.SetTaskStatusSummaryErrandsReader(func(ctx context.Context, ids []string) (map[string]statussummary.ErrandsSummary, error) {
				reads++
				if reads > 1 {
					return nil, errors.New("errands unavailable after CAS")
				}
				return repo.BatchErrandsSummariesByTaskIDs(ctx, ids)
			})
			svc.statusSummaries = &errandsSummaryCASRepository{TaskStatusSummaryRepository: repo, beforeReject: func() {
				require.NoError(t, repo.UpdateTaskSessionState(ctx, "parent", models.TaskSessionStateRunning, ""))
			}}
			bus.ClearEvents()
			got, err := svc.ReconcileTaskStatusSummaries(ctx, []*models.Task{task}, nil, nil, before)
			require.ErrorContains(t, err, "errands unavailable after CAS")
			require.Equal(t, 2, reads)
			require.NotNil(t, got[task.ID])
			require.Equal(t, 7, got[task.ID].QueuedPromptCount)
			require.Nil(t, got[task.ID].Errands)
			stored, err := repo.LoadTaskStatusSummaries(ctx, []string{task.ID})
			require.NoError(t, err)
			require.Equal(t, got[task.ID], stored[task.ID])
			require.Empty(t, bus.GetPublishedEvents())

			svc.SetTaskStatusSummaryErrandsReader(repo.BatchErrandsSummariesByTaskIDs)
			got, err = svc.ReconcileTaskStatusSummaries(ctx, []*models.Task{task}, nil, nil, got)
			require.NoError(t, err)
			require.Equal(t, &statussummary.ErrandsSummary{Live: 1}, got[task.ID].Errands)
			require.Equal(t, 7, got[task.ID].QueuedPromptCount)
			require.Len(t, bus.GetPublishedEvents(), 1)
		})
	}
}

// @covers AC-TASKS-ERRANDS-010.5 AC-TASKS-ERRANDS-012.2
func TestTaskStatusSummaryErrandsRepairReadFailureKeepsLastValid(t *testing.T) {
	for _, failureKind := range []string{"error", "omitted", "negative"} {
		t.Run(failureKind, func(t *testing.T) {
			ctx := context.Background()
			svc, bus, repo, task := newErrandsSummaryRepairFixture(t)
			current, err := svc.ReconcileTaskStatusSummaries(ctx, []*models.Task{task}, nil, nil, nil)
			require.NoError(t, err)
			last := *current[task.ID]
			require.NoError(t, repo.UpdateTaskSessionState(ctx, "parent", models.TaskSessionStateRunning, ""))
			svc.SetTaskStatusSummaryErrandsReader(func(context.Context, []string) (map[string]statussummary.ErrandsSummary, error) {
				switch failureKind {
				case "omitted":
					return map[string]statussummary.ErrandsSummary{}, nil
				case "negative":
					return map[string]statussummary.ErrandsSummary{task.ID: {Live: -1}}, nil
				default:
					return nil, errors.New("errands unavailable")
				}
			})
			bus.ClearEvents()
			got, err := svc.ReconcileTaskStatusSummaries(ctx, []*models.Task{task}, nil, nil, current)
			require.Error(t, err)
			require.Equal(t, last, *got[task.ID])
			stored, err := repo.LoadTaskStatusSummaries(ctx, []string{task.ID})
			require.NoError(t, err)
			require.Equal(t, last, *stored[task.ID])
			require.Empty(t, bus.GetPublishedEvents())
			// A different task without a stored projection must not get a fabricated zero.
			missing := &models.Task{ID: "missing", WorkspaceID: task.WorkspaceID}
			require.NoError(t, repo.CreateTask(ctx, missing))
			got, err = svc.ReconcileTaskStatusSummaries(ctx, []*models.Task{missing}, nil, nil, nil)
			require.Error(t, err)
			require.Nil(t, got[missing.ID])
			stored, err = repo.LoadTaskStatusSummaries(ctx, []string{missing.ID})
			require.NoError(t, err)
			require.Nil(t, stored[missing.ID])
			require.Empty(t, bus.GetPublishedEvents())
			svc.SetTaskStatusSummaryErrandsReader(repo.BatchErrandsSummariesByTaskIDs)
			got, err = svc.ReconcileTaskStatusSummaries(ctx, []*models.Task{task}, nil, nil, current)
			require.NoError(t, err)
			require.Equal(t, &statussummary.ErrandsSummary{Live: 1}, got[task.ID].Errands)
			require.Greater(t, got[task.ID].Revision, last.Revision)
		})
	}
}

func TestTaskStatusSummaryErrandsRepairNoopAndBatching(t *testing.T) {
	ctx := context.Background()
	svc, bus, repo := createTestService(t)
	createTaskWithoutRepositories(t, ctx, repo)
	svc.statusSummaries = repo
	tasks := make([]*models.Task, 130)
	for index := range tasks {
		task := &models.Task{ID: fmt.Sprintf("batch-errands-%03d", index), WorkspaceID: "ws-1"}
		require.NoError(t, repo.CreateTask(ctx, task))
		tasks[index] = task
	}
	reads := 0
	svc.SetTaskStatusSummaryErrandsReader(func(ctx context.Context, ids []string) (map[string]statussummary.ErrandsSummary, error) {
		reads++
		require.Len(t, ids, 130)
		return repo.BatchErrandsSummariesByTaskIDs(ctx, ids)
	})
	got, err := svc.ReconcileTaskStatusSummaries(ctx, tasks, nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 1, reads)
	require.Len(t, got, 130)
	revisions := make(map[string]uint64, len(got))
	for id, row := range got {
		require.Nil(t, row.Errands)
		revisions[id] = row.Revision
	}
	bus.ClearEvents()
	got, err = svc.ReconcileTaskStatusSummaries(ctx, tasks, nil, nil, got)
	require.NoError(t, err)
	require.Equal(t, 2, reads)
	require.Empty(t, bus.GetPublishedEvents())
	for id, row := range got {
		require.Equal(t, revisions[id], row.Revision)
	}
}

func TestTaskStatusSummaryErrandsRepairAuthoritativeTransitions(t *testing.T) {
	ctx := context.Background()
	svc, bus, repo, task := newErrandsSummaryRepairFixture(t)
	reconcile := func(previous map[string]*statussummary.TaskStatusSummary, want *statussummary.ErrandsSummary) map[string]*statussummary.TaskStatusSummary {
		t.Helper()
		bus.ClearEvents()
		got, err := svc.ReconcileTaskStatusSummaries(ctx, []*models.Task{task}, nil, nil, previous)
		require.NoError(t, err)
		require.Equal(t, want, got[task.ID].Errands)
		stored, err := repo.LoadTaskStatusSummaries(ctx, []string{task.ID})
		require.NoError(t, err)
		require.Equal(t, got[task.ID], stored[task.ID])
		require.Len(t, bus.GetPublishedEvents(), 1)
		return got
	}
	got := reconcile(nil, &statussummary.ErrandsSummary{Live: 1, Waiting: true})
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "child", TaskID: task.ID, Role: models.SessionRoleErrand, ParentSessionID: "parent", State: models.TaskSessionStateFailed}))
	errand, err := repo.GetErrand(ctx, "child")
	require.NoError(t, err)
	errand.Lifecycle = models.ErrandLifecycleLaunched
	errand.LastOutcome = models.ErrandOutcomeFailed
	errand.ResultState = models.ErrandResultPending
	require.NoError(t, repo.UpdateErrand(ctx, errand))
	got = reconcile(got, &statussummary.ErrandsSummary{Waiting: true})
	// A failed sibling leaves the live sibling count intact.
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{ID: "sibling", TaskID: task.ID, ParentSessionID: "parent", Title: "Sibling", Objective: "Work", AgentProfileID: "profile", FirstTurnID: "sibling-turn", SubmissionID: "sibling-submission", Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone, AwaitingResult: true}))
	got = reconcile(got, &statussummary.ErrandsSummary{Live: 1, Waiting: true})
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "parent", models.TaskSessionStateRunning, ""))
	got = reconcile(got, &statussummary.ErrandsSummary{Live: 1})
	sibling, err := repo.GetErrand(ctx, "sibling")
	require.NoError(t, err)
	sibling.Lifecycle = models.ErrandLifecycleCancelled
	sibling.AwaitingResult = false
	require.NoError(t, repo.UpdateErrand(ctx, sibling))
	got = reconcile(got, nil)
	// Without a configured reader, unrelated repair must preserve the old value.
	stale := *got[task.ID]
	stale.Revision++
	stale.Errands = &statussummary.ErrandsSummary{Live: 3}
	stale.PendingAction = "permission"
	accepted, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{TaskID: task.ID, WorkspaceID: task.WorkspaceID, Summary: stale})
	require.NoError(t, err)
	require.True(t, accepted)
	svc.SetTaskStatusSummaryErrandsReader(nil)
	reconcile(map[string]*statussummary.TaskStatusSummary{task.ID: &stale}, &statussummary.ErrandsSummary{Live: 3})
}
