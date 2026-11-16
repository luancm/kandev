package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type errandSessionStateObserver struct {
	recordingErrandController
	observe func(context.Context, string) error
}

func (c *errandSessionStateObserver) OnSessionStateChanged(ctx context.Context, id string) error {
	return c.observe(ctx, id)
}

func TestErrandWaiterSeesPersistedIdleStateWithoutEventBus(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-state", "session-state", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = nil
	calls := 0
	svc.SetErrandController(&errandSessionStateObserver{observe: func(ctx context.Context, id string) error {
		session, err := repo.GetTaskSession(ctx, id)
		require.NoError(t, err)
		require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
		calls++
		return nil
	}})
	_, changed := svc.updateTaskSessionStateWithHook(ctx, "task-state", "session-state",
		models.TaskSessionStateWaitingForInput, "", false, nil)
	require.True(t, changed)
	require.Equal(t, 1, calls)
	_, changed = svc.updateTaskSessionStateWithHook(ctx, "task-state", "session-state",
		models.TaskSessionStateWaitingForInput, "", false, nil)
	require.False(t, changed)
	require.Equal(t, 1, calls, "unchanged state is not a new wake")
}
