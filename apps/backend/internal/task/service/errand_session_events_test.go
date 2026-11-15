package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type errandWaitingActivityProvider struct{}

func (errandWaitingActivityProvider) ForegroundActivity(string) v1.ForegroundActivity { return "" }

func (errandWaitingActivityProvider) ParentErrandState(sessionID string) (bool, bool) {
	hasErrands := sessionID == "parent-1" || sessionID == "parent-2"
	return hasErrands, hasErrands
}

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
	svc.SetForegroundActivityProvider(errandWaitingActivityProvider{})
	now := time.Now().UTC()
	errand := &models.TaskSession{
		ID: "errand-1", TaskID: "task-1", State: models.TaskSessionStateCancelled, UpdatedAt: now,
		Role: models.SessionRoleErrand, ParentSessionID: "parent-1",
	}
	ordinary := &models.TaskSession{ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCancelled, UpdatedAt: now}

	parentCancelled := &models.TaskSession{ID: "parent-1", TaskID: "task-1", State: models.TaskSessionStateCancelled, UpdatedAt: now}
	svc.publishSessionsCancelled(ctx, "task-1", nil, []*models.TaskSession{errand, ordinary, parentCancelled}, "task archived")
	if err := svc.publishSessionRecovered(ctx, "task-1", models.TaskSessionStateRunning, &models.TaskSession{
		ID: "errand-2", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput, UpdatedAt: now,
		Role: models.SessionRoleErrand, ParentSessionID: "parent-1",
	}); err != nil {
		t.Fatalf("publishSessionRecovered: %v", err)
	}
	if err := svc.publishSessionRecovered(ctx, "task-1", models.TaskSessionStateRunning, &models.TaskSession{
		ID: "parent-2", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("publishSessionRecovered for parent: %v", err)
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
	if got := sessionStateEventFor(t, published, "parent-1")["waiting_on_errands"]; got != false {
		t.Fatalf("cancelled parent waiting_on_errands = %v, want explicit false", got)
	}
	if got := sessionStateEventFor(t, published, "parent-2")["waiting_on_errands"]; got != true {
		t.Fatalf("recovered parent waiting_on_errands = %v, want true", got)
	}
	ordinaryData := sessionStateEventFor(t, published, "session-1")
	if _, has := ordinaryData["role"]; has {
		t.Fatalf("an ordinary session event gained a role: %v", ordinaryData)
	}
	if _, has := ordinaryData["parent_session_id"]; has {
		t.Fatalf("an ordinary session event gained a parent: %v", ordinaryData)
	}
}

// @covers AC-TASKS-ERRANDS-006.3
func TestParentLifecycleEventsExplicitlyClearWaitingOnErrandsWithoutRows(t *testing.T) {
	svc, eventBus, _ := createTestService(t)
	svc.SetForegroundActivityProvider(errandWaitingActivityProvider{})
	ctx := context.Background()
	now := time.Now().UTC()

	svc.publishSessionsCancelled(ctx, "task-1", nil, []*models.TaskSession{{
		ID: "parent-cancelled", TaskID: "task-1", State: models.TaskSessionStateCancelled, UpdatedAt: now,
	}}, "task archived")
	if err := svc.publishSessionRecovered(ctx, "task-1", models.TaskSessionStateRunning, &models.TaskSession{
		ID: "parent-recovered", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("publishSessionRecovered: %v", err)
	}

	for _, sessionID := range []string{"parent-cancelled", "parent-recovered"} {
		var payloads []map[string]interface{}
		for _, event := range eventBus.GetPublishedEvents() {
			if data, ok := event.Data.(map[string]interface{}); ok && event.Type == events.TaskSessionStateChanged {
				payloads = append(payloads, data)
			}
		}
		data := sessionStateEventFor(t, func() []map[string]interface{} { return payloads }, sessionID)
		if got, exists := data["waiting_on_errands"]; !exists || got != false {
			t.Errorf("%s waiting_on_errands = %v (present=%v), want explicit false", sessionID, got, exists)
		}
	}
}
