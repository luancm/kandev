package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func errandEventPayload(workspaceID string) dto.ErrandUpdatedEvent {
	return dto.ErrandUpdatedEvent{
		ErrandDTO:   dto.FromErrand(&models.Errand{ID: "errand-1", TaskID: "task-a", ParentSessionID: "parent-1"}, models.ErrandStatusQueued),
		WorkspaceID: workspaceID,
	}
}

// @covers AC-TASKS-ERRANDS-010.9
func TestErrandUpdatedIsRoutedToTheOwningWorkspaceOnly(t *testing.T) {
	hub := newTestHub(t)
	hub.setAuthPolicy(AuthPolicy{
		Enforced: func() bool { return true },
		WorkspaceOwner: func(_ context.Context, workspaceID string) (string, error) {
			if workspaceID == "workspace-a" {
				return "user-a", nil
			}
			return "", errors.New("unknown workspace")
		},
	})
	owner := newTestClient("owner")
	owner.identity = authn.Identity{UserID: "user-a", Role: authn.RoleMember}
	foreign := newTestClient("foreign")
	foreign.identity = authn.Identity{UserID: "user-b", Role: authn.RoleMember}
	registerTestClient(hub, owner)
	registerTestClient(hub, foreign)
	broadcaster := &TaskEventBroadcaster{hub: hub, logger: testLogger()}

	event := bus.NewEvent(events.ErrandUpdated, "test", errandEventPayload("workspace-a"))
	require.NoError(t, broadcaster.broadcastEvent(context.Background(), event, ws.ActionErrandUpdated))
	require.True(t, clientReceived(owner), "the workspace owner receives errand.updated")
	require.False(t, clientReceived(foreign), "errand.updated crossed the workspace boundary")
}

func TestErrandUpdatedWithoutAWorkspaceIsDroppedNotBroadcastGlobally(t *testing.T) {
	hub := newTestHub(t)
	hub.setAuthPolicy(AuthPolicy{Enforced: func() bool { return true }})
	client := newTestClient("client")
	client.identity = authn.Identity{UserID: "user-a", Role: authn.RoleMember}
	registerTestClient(hub, client)
	broadcaster := &TaskEventBroadcaster{hub: hub, logger: testLogger()}

	event := bus.NewEvent(events.ErrandUpdated, "test", errandEventPayload(""))
	require.NoError(t, broadcaster.broadcastEvent(context.Background(), event, ws.ActionErrandUpdated))
	require.False(t, clientReceived(client), "an unattributed errand event must fail closed")
}

func TestErrandUpdatedReachesClientsThroughTheEventBusSubscription(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := NewHub(nil, log)
	RegisterTaskNotifications(ctx, eventBus, hub, log)

	require.NoError(t, eventBus.Publish(ctx, events.ErrandUpdated,
		bus.NewEvent(events.ErrandUpdated, "test", errandEventPayload("workspace-a"))))
	select {
	case message := <-hub.broadcast:
		require.Equal(t, ws.ActionErrandUpdated, message.Action)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(message.Payload, &payload))
		require.Equal(t, "errand-1", payload["errand_id"])
		require.Equal(t, "queued", payload["status"])
		require.Contains(t, payload, "can_send_result")
	default:
		t.Fatal("errand.updated was not broadcast")
	}
}

// @covers AC-TASKS-ERRANDS-007.1
func TestDispatchScopeCoversTheErrandRequests(t *testing.T) {
	for _, tc := range []struct {
		action  string
		payload string
		allowed bool
	}{
		{ws.ActionErrandList, `{"task_id":"task-a"}`, true},
		{ws.ActionErrandList, `{"task_id":"task-b"}`, false},
		{ws.ActionErrandCancel, `{"session_id":"sess-a"}`, true},
		{ws.ActionErrandCancel, `{"session_id":"sess-b"}`, false},
		{ws.ActionErrandCancelAll, `{"task_id":"task-a"}`, true},
		{ws.ActionErrandCancelAll, `{"task_id":"task-b"}`, false},
	} {
		t.Run(tc.action+tc.payload, func(t *testing.T) {
			f := newDispatchScopeFixture(t, realIdentity())
			f.dispatchAs(t, tc.action, tc.payload)
			require.Equal(t, tc.allowed, *f.handled, "the gateway backstop scopes errand requests by task_id and session_id")
		})
	}
}
