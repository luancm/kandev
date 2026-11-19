package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestCreatedOrdinarySessionStillUsesWorkflowProfileAndPrompt(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	now := time.Now().UTC()
	session := &models.TaskSession{
		ID: "ordinary-created", TaskID: f.taskID, State: models.TaskSessionStateCreated,
		AgentProfileID: "ordinary-profile", StartedAt: now, UpdatedAt: now,
	}
	require.NoError(t, f.repo.CreateTaskSession(ctx, session))
	f.svc.workflowStepGetter.(*mockStepGetter).steps["step-1"].Prompt = "WORKFLOW-PREFIX-SENTINEL {{task_prompt}}"
	_, err := f.svc.startCreatedSession(
		ctx, f.taskID, session.ID, "caller-profile", "ordinary request",
		true, false, true, nil, nil, "", startCreatedSessionOptions{},
	)
	require.NoError(t, err)
	require.Len(t, f.manager.launchedWith, 1)
	require.Equal(t, "step-profile", f.manager.launchedWith[0].AgentProfileID)
	require.Contains(t, f.manager.launchedWith[0].TaskDescription, "WORKFLOW-PREFIX-SENTINEL")
	require.Contains(t, f.manager.launchedWith[0].TaskDescription, "ordinary request")
	f.taskRepo.mu.Lock()
	require.Equal(t, []v1.TaskState{v1.TaskStateScheduling}, f.taskRepo.stateHistory[f.taskID])
	f.taskRepo.mu.Unlock()
}

func TestScheduleTaskForSessionRejectsIneligibleTask(t *testing.T) {
	f := newErrandLaunchFixture(t)
	f.taskRepo.mu.Lock()
	f.taskRepo.tasks[f.taskID].State = v1.TaskStateFailed
	f.taskRepo.mu.Unlock()

	err := f.svc.scheduleTaskForSession(t.Context(), f.taskID, f.parent)
	require.ErrorContains(t, err, "cannot schedule session")

	f.taskRepo.mu.Lock()
	defer f.taskRepo.mu.Unlock()
	require.Empty(t, f.taskRepo.stateHistory[f.taskID])
}
