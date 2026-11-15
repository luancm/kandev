package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

type recordingTaskErrandCanceller struct{ taskIDs []string }

func (r *recordingTaskErrandCanceller) CancelErrandsForTask(_ context.Context, taskID string) {
	r.taskIDs = append(r.taskIDs, taskID)
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
