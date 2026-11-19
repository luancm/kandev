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
