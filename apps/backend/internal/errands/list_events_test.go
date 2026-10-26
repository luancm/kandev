package errands

import (
	"context"
	"encoding/json"
	"errors"
	"expvar"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestListProjectsEveryErrandInCreationOrder(t *testing.T) {
	h := newHarness(t)
	done := h.mustStart("done").Errand
	running := h.mustStart("running").Errand
	h.launcher.behave = func(int, LaunchRequest) error { return ErrCeilingRefused }
	queued := h.mustStart("queued").Errand
	h.store.setSessionState(done.ID, models.TaskSessionStateWaitingForInput)
	if _, _, err := h.service.update(context.Background(), done.ID, func(e *models.Errand) bool {
		e.LastOutcome = models.ErrandOutcomeDone
		return true
	}); err != nil {
		t.Fatalf("settle: %v", err)
	}
	h.store.pending[running.ID] = models.TaskPendingActionPermission

	snapshots, err := h.service.List(context.Background(), testTask)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []struct {
		id     string
		status models.ErrandStatus
	}{{done.ID, models.ErrandStatusDone}, {running.ID, models.ErrandStatusNeedsAttention}, {queued.ID, models.ErrandStatusQueued}}
	if len(snapshots) != len(want) {
		t.Fatalf("List returned %d errands, want %d", len(snapshots), len(want))
	}
	for i, w := range want {
		if snapshots[i].Errand.ID != w.id || snapshots[i].Status != w.status {
			t.Fatalf("snapshot %d = %s/%s, want %s/%s", i, snapshots[i].Errand.ID, snapshots[i].Status, w.id, w.status)
		}
	}
	if other, err := h.service.List(context.Background(), "task-2"); err != nil || len(other) != 0 {
		t.Fatalf("List of another task = %v, %v, want empty", other, err)
	}
}

func TestBusPublisherRoutesByWorkspaceAndSkipsUnresolvedWorkspaces(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stderr"})
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	received := make(chan map[string]interface{}, 2)
	if _, err := eventBus.Subscribe(events.ErrandUpdated, func(_ context.Context, event *bus.Event) error {
		raw, _ := json.Marshal(event.Data)
		var payload map[string]interface{}
		_ = json.Unmarshal(raw, &payload)
		received <- payload
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	publisher := NewBusPublisher(eventBus, func(_ context.Context, taskID string) (string, error) {
		if taskID == "orphan" {
			return "", errors.New("task missing")
		}
		return "workspace-1", nil
	}, log)

	errand := &models.Errand{ID: "errand-1", TaskID: "task-1", ParentSessionID: "parent-1"}
	publisher.PublishErrandUpdated(context.Background(), errand, models.ErrandStatusQueued)
	payload := <-received
	if payload["workspace_id"] != "workspace-1" || payload["errand_id"] != "errand-1" || payload["status"] != "queued" {
		t.Fatalf("payload = %v", payload)
	}
	if _, has := payload["can_send_result"]; !has {
		t.Fatal("payload lacks can_send_result")
	}

	publisher.PublishErrandUpdated(context.Background(), &models.Errand{ID: "e2", TaskID: "orphan"}, models.ErrandStatusQueued)
	select {
	case unexpected := <-received:
		t.Fatalf("an errand without a workspace was published: %v", unexpected)
	default:
	}
}

// @covers AC-TASKS-ERRANDS-013.1
func TestCountersUseOnlyClosedLabelSets(t *testing.T) {
	h := newHarness(t)
	h.mustStart("audit")
	_, _ = h.service.Cancel(context.Background(), CancelRequest{
		ErrandID: h.launcher.calls()[0].ErrandID, TaskID: testTask, Source: models.ErrandCancelSourceUser,
	})
	countSettled(models.ErrandStatusRunning) // outside the closed set: ignored

	allowed := map[string]map[string]bool{
		"errands_started_total": {"outcome=started": true, "outcome=queued": true, "outcome=limit_reached": true,
			"outcome=rejected": true, "outcome=failed": true},
		"errands_settled_total": {"status=done": true, "status=failed": true, "status=cancelled": true},
	}
	for name, labels := range allowed {
		counter, ok := expvar.Get(name).(*expvar.Map)
		if !ok {
			t.Fatalf("expvar %s is not published", name)
		}
		counter.Do(func(entry expvar.KeyValue) {
			if !labels[entry.Key] {
				t.Errorf("%s has label %q outside its closed set", name, entry.Key)
			}
		})
	}
	if settled := expvar.Get("errands_settled_total").(*expvar.Map).Get("status=running"); settled != nil {
		t.Fatal("an open-set status leaked into errands_settled_total")
	}
}

func TestWSErrorRendersTypedFailuresWithReasonAndDetails(t *testing.T) {
	err := newError(ReasonLimitReached, "full", map[string]interface{}{"limit": 5, "live": 5})
	msg, handled := WSError("id-1", "errand.start", err)
	if !handled {
		t.Fatal("typed error not handled")
	}
	var payload ws.ErrorPayload
	if jsonErr := json.Unmarshal(msg.Payload, &payload); jsonErr != nil {
		t.Fatalf("payload: %v", jsonErr)
	}
	if payload.Code != ws.ErrorCodeConflict || payload.Details["reason"] != "errand_limit_reached" || payload.Details["limit"] != float64(5) {
		t.Fatalf("payload = %+v", payload)
	}

	if msg, handled := WSError("id", "a", errors.New("boom")); handled || msg != nil {
		t.Fatal("an untyped error was rendered as a typed failure")
	}
	msg, handled = WSError("id", "a", errors.Join(ErrInvalidRequest, errors.New("x")))
	if !handled {
		t.Fatal("invalid request not handled")
	}
	_ = json.Unmarshal(msg.Payload, &payload)
	if payload.Code != ws.ErrorCodeValidation || strings.Contains(string(msg.Payload), "reason") {
		t.Fatalf("invalid request payload = %s, want VALIDATION_ERROR without a reason", msg.Payload)
	}
}

func TestErrorCodesFollowTheDesignTable(t *testing.T) {
	want := map[Reason]string{
		ReasonFeatureDisabled: ws.ErrorCodeNotFound, ReasonNotFound: ws.ErrorCodeNotFound,
		ReasonNotSupported: ws.ErrorCodeForbidden, ReasonNotPermitted: ws.ErrorCodeForbidden, ReasonSenderNotParent: ws.ErrorCodeForbidden,
		ReasonProfileNotFound: ws.ErrorCodeValidation, ReasonProfileAmbiguous: ws.ErrorCodeValidation,
		ReasonProfileIncompatible: ws.ErrorCodeValidation, ReasonLimitReached: ws.ErrorCodeConflict,
	}
	for reason, code := range want {
		if got := ErrorCode(reason); got != code {
			t.Fatalf("ErrorCode(%s) = %s, want %s", reason, got, code)
		}
	}
}
