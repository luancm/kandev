package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

type recordingTaskErrandCanceller struct{ taskIDs []string }

func (r *recordingTaskErrandCanceller) CascadeTask(taskID string) bool {
	r.taskIDs = append(r.taskIDs, taskID)
	return true
}

func (*recordingTaskErrandCanceller) CancelForTask(context.Context, string) (int, error) {
	return 0, nil
}

// @covers AC-TASKS-ERRANDS-007.4
func TestArchiveTaskCancelsItsErrandsEvenWhenNoSessionIsActive(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Test", Priority: "medium",
	}))
	canceller := &recordingTaskErrandCanceller{}
	svc.SetTaskErrandCanceller(canceller)

	require.NoError(t, svc.ArchiveTask(ctx, "task-1"))
	require.Equal(t, []string{"task-1"}, canceller.taskIDs)
}

type taskDeletionErrandCanceller struct {
	recordingTaskErrandCanceller
	stop func(context.Context, string) (int, error)
}

func (c *taskDeletionErrandCanceller) CancelForTask(ctx context.Context, taskID string) (int, error) {
	return c.stop(ctx, taskID)
}

func TestDeleteTaskRetainsTaskWhenErrandStopIsUnconfirmed(t *testing.T) {
	for _, path := range []string{"task", "workspace", "cascade"} {
		t.Run(path, func(t *testing.T) {
			exerciseTaskDeletionWhenErrandStopIsUnconfirmed(t, path)
		})
	}
}

func exerciseTaskDeletionWhenErrandStopIsUnconfirmed(t *testing.T, path string) {
	t.Helper()
	svc, _, repo := createTestService(t)
	ctx := t.Context()
	const taskID = "task-delete-owned-errand"
	seedCleanupTaskAndSession(t, repo, taskID, "parent")
	remove := func() error { return svc.DeleteTask(ctx, taskID) }
	switch path {
	case "workspace":
		remove = func() error { return svc.DeleteWorkspace(ctx, "ws-"+taskID) }
	case "cascade":
		handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
		handoff.SetTaskResourceCleaner(svc)
		remove = func() error {
			_, err := handoff.DeleteTaskTree(ctx, taskID, false)
			return err
		}
	}
	stopErr := errors.New("errand stop is unconfirmed")
	calls := 0
	svc.SetTaskErrandCanceller(&taskDeletionErrandCanceller{stop: func(ctx context.Context, observedTaskID string) (int, error) {
		require.Equal(t, taskID, observedTaskID)
		var state models.TaskResourceCleanupState
		require.NoError(t, repo.DB().QueryRowContext(ctx, `
			SELECT state FROM task_resource_cleanup_jobs
			WHERE task_id = ? AND state = ?
		`, taskID, models.TaskResourceCleanupStatePrepared).Scan(&state))
		calls++
		if calls == 1 {
			return 0, stopErr
		}
		return 1, nil
	}})

	require.ErrorIs(t, remove(), stopErr)
	_, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	require.NoError(t, remove())
	require.Equal(t, 2, calls)
	_, err = repo.GetTask(ctx, taskID)
	require.Error(t, err)
}
