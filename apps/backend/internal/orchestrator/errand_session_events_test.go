package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func lastSessionStatePayload(t *testing.T, bus *mockEventBus, sessionID string) map[string]interface{} {
	t.Helper()
	var found map[string]interface{}
	for _, evt := range bus.published() {
		data, ok := evt.Event.Data.(map[string]interface{})
		if evt.Subject == events.TaskSessionStateChanged && ok && data["session_id"] == sessionID {
			found = data
		}
	}
	require.NotNil(t, found, "no session.state_changed event for %s", sessionID)
	return found
}

// @covers AC-TASKS-ERRANDS-010.2
func TestSessionStateEventsCarryErrandRoleAndParentOnEveryBuilder(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "parent-1", "")
	seedErrandSession(t, repo, "task-1", "parent-1", "errand-1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	bus := &mockEventBus{}
	svc.eventBus = bus

	errand, err := repo.GetTaskSession(ctx, "errand-1")
	require.NoError(t, err)
	now := time.Now().UTC()

	builders := map[string]func(){
		"created": func() { svc.publishSessionCreatedEvent(ctx, "task-1", "errand-1", "") },
		"waiting": func() { svc.publishSessionWaitingEvent(ctx, "task-1", "errand-1", "", errand) },
		"state": func() {
			svc.publishTaskSessionStateChanged(ctx, "task-1", "errand-1",
				models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, "", &now, errand)
		},
	}
	for name, publish := range builders {
		t.Run(name, func(t *testing.T) {
			bus.events = nil
			publish()
			data := lastSessionStatePayload(t, bus, "errand-1")
			require.Equal(t, "errand", data["role"])
			require.Equal(t, "parent-1", data["parent_session_id"])
		})
	}

	t.Run("ordinary sessions keep their payload unchanged", func(t *testing.T) {
		parent, err := repo.GetTaskSession(ctx, "parent-1")
		require.NoError(t, err)
		for name, publish := range map[string]func(){
			"created": func() { svc.publishSessionCreatedEvent(ctx, "task-1", "parent-1", "") },
			"waiting": func() { svc.publishSessionWaitingEvent(ctx, "task-1", "parent-1", "", parent) },
			"state": func() {
				svc.publishTaskSessionStateChanged(ctx, "task-1", "parent-1",
					models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, "", &now, parent)
			},
		} {
			bus.events = nil
			publish()
			data := lastSessionStatePayload(t, bus, "parent-1")
			require.NotContains(t, data, "role", name)
			require.NotContains(t, data, "parent_session_id", name)
		}
	})
}
