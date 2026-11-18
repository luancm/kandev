package websocket

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events/bus"
)

func TestErrandTranscriptChangeReachesOnlyTheNamedTask(t *testing.T) {
	hub := newTestHub(t)
	owner := newTestClient("viewer")
	foreign := newTestClient("other")
	hub.SubscribeToTask(owner, "task-1")
	hub.SubscribeToTask(foreign, "task-2")
	eventBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(eventBus.Close)
	broadcaster := RegisterTaskNotifications(t.Context(), eventBus, hub, testLogger())
	t.Cleanup(broadcaster.Close)

	const action = "errand.transcript_changed"
	payload := map[string]any{"task_id": "task-1", "errand_id": "errand-1"}
	require.NoError(t, eventBus.Publish(t.Context(), action, bus.NewEvent(action, "test", payload)))
	require.True(t, clientReceived(owner), "the selected task should receive a transcript invalidation")
	require.False(t, clientReceived(foreign), "the invalidation must stay within its task")
}
