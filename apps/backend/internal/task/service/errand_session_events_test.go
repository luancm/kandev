package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func sessionStateEventFor(t *testing.T, published func() []map[string]interface{}, sessionID string) map[string]interface{} {
	t.Helper()
	for _, data := range published() {
		if data["session_id"] == sessionID {
			return data
		}
	}
	t.Fatalf("no session.state_changed event for %s", sessionID)
	return nil
}

// @covers AC-TASKS-ERRANDS-010.2
func TestSessionCancelledAndRecoveredEventsCarryErrandRoleAndParent(t *testing.T) {
	svc, eventBus, _ := createTestService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	errand := &models.TaskSession{
		ID: "errand-1", TaskID: "task-1", State: models.TaskSessionStateCancelled, UpdatedAt: now,
		Role: models.SessionRoleErrand, ParentSessionID: "parent-1",
	}
	ordinary := &models.TaskSession{ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCancelled, UpdatedAt: now}

	svc.publishSessionsCancelled(ctx, "task-1", nil, []*models.TaskSession{errand, ordinary}, "task archived")
	if err := svc.publishSessionRecovered(ctx, "task-1", models.TaskSessionStateRunning, &models.TaskSession{
		ID: "errand-2", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput, UpdatedAt: now,
		Role: models.SessionRoleErrand, ParentSessionID: "parent-1",
	}); err != nil {
		t.Fatalf("publishSessionRecovered: %v", err)
	}

	published := func() []map[string]interface{} {
		var payloads []map[string]interface{}
		for _, evt := range eventBus.GetPublishedEvents() {
			if data, ok := evt.Data.(map[string]interface{}); ok && evt.Type == events.TaskSessionStateChanged {
				payloads = append(payloads, data)
			}
		}
		return payloads
	}
	for _, id := range []string{"errand-1", "errand-2"} {
		data := sessionStateEventFor(t, published, id)
		if data["role"] != "errand" || data["parent_session_id"] != "parent-1" {
			t.Fatalf("%s event role/parent = %v/%v, want errand/parent-1", id, data["role"], data["parent_session_id"])
		}
	}
	ordinaryData := sessionStateEventFor(t, published, "session-1")
	if _, has := ordinaryData["role"]; has {
		t.Fatalf("an ordinary session event gained a role: %v", ordinaryData)
	}
	if _, has := ordinaryData["parent_session_id"]; has {
		t.Fatalf("an ordinary session event gained a parent: %v", ordinaryData)
	}
}
