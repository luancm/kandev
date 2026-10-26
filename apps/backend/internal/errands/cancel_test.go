package errands

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func cancelRequest(errand *models.Errand, source models.ErrandCancelSource) CancelRequest {
	return CancelRequest{ErrandID: errand.ID, TaskID: testTask, Source: source}
}

// @covers AC-TASKS-ERRANDS-007.1
func TestCancelMarksTheRowBeforeStoppingAndIsIdempotent(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand

	snapshot, err := h.service.Cancel(context.Background(), cancelRequest(errand, models.ErrandCancelSourceParent))
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if snapshot.Status != models.ErrandStatusCancelled || snapshot.Errand.CancelSource != models.ErrandCancelSourceParent {
		t.Fatalf("snapshot = %s/%s, want cancelled by the parent", snapshot.Status, snapshot.Errand.CancelSource)
	}
	if h.stopper.rowAtStop[errand.ID] != models.ErrandLifecycleCancelled || h.stopper.reasons[0] != StopReasonErrandCancelled {
		t.Fatalf("row was %q at stop with reason %q, want durable-first with errand_cancelled",
			h.stopper.rowAtStop[errand.ID], h.stopper.reasons[0])
	}
	if snapshot.Errand.ResultState != models.ErrandResultNone || snapshot.Errand.AwaitingResult {
		t.Fatalf("a parent cancel left result %q awaiting=%v, want no result", snapshot.Errand.ResultState, snapshot.Errand.AwaitingResult)
	}

	updates, events := h.store.updates, len(h.publisher.statuses())
	again, err := h.service.Cancel(context.Background(), cancelRequest(errand, models.ErrandCancelSourceUser))
	if err != nil || again.Status != models.ErrandStatusCancelled {
		t.Fatalf("repeat Cancel = %v/%v, want success", again, err)
	}
	if h.store.updates != updates || len(h.publisher.statuses()) != events {
		t.Fatal("a repeated cancel changed the row or published again")
	}
	if len(h.stopper.stops()) != 2 {
		t.Fatalf("stops = %d, want the stop retried on a repeat", len(h.stopper.stops()))
	}
}

// @covers AC-TASKS-ERRANDS-003.5
func TestUserCancelOfAnOutstandingErrandLeavesACancellationResult(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand
	snapshot, err := h.service.Cancel(context.Background(), cancelRequest(errand, models.ErrandCancelSourceUser))
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	row := snapshot.Errand
	if row.ResultKind != models.ErrandResultCancelled || row.ResultState != models.ErrandResultPending || row.AwaitingResult {
		t.Fatalf("row = %+v, want a pending cancelled result and no longer awaiting", row)
	}
	if got := h.deliverer.delivered(); len(got) != 1 || got[0] != testParent {
		t.Fatalf("deliveries = %v, want the notice delivered once", got)
	}
}

func TestUserCancelOfASettledErrandLeavesNoResult(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand
	if _, _, err := h.service.update(context.Background(), errand.ID, func(e *models.Errand) bool {
		e.AwaitingResult, e.LastOutcome, e.ResultState = false, models.ErrandOutcomeDone, models.ErrandResultDelivered
		return true
	}); err != nil {
		t.Fatalf("settle: %v", err)
	}
	snapshot, err := h.service.Cancel(context.Background(), cancelRequest(errand, models.ErrandCancelSourceUser))
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if snapshot.Errand.ResultState != models.ErrandResultDelivered || snapshot.Errand.ResultKind == models.ErrandResultCancelled {
		t.Fatalf("row = %+v, want the delivered result untouched", snapshot.Errand)
	}
}

// @covers AC-TASKS-ERRANDS-008.3
func TestCancellingAQueuedErrandNeedsNoRuntimeStop(t *testing.T) {
	h := newHarness(t)
	h.launcher.behave = func(int, LaunchRequest) error { return ErrCeilingRefused }
	errand := h.mustStart("audit").Errand
	snapshot, err := h.service.Cancel(context.Background(), cancelRequest(errand, models.ErrandCancelSourceUser))
	if err != nil || snapshot.Status != models.ErrandStatusCancelled {
		t.Fatalf("Cancel = %v/%v, want a cancelled queued errand", snapshot, err)
	}
	if snapshot.Errand.ResultKind != models.ErrandResultCancelled {
		t.Fatalf("result kind = %q, want a cancellation notice for the outstanding queued errand", snapshot.Errand.ResultKind)
	}
}

// Cancelling another task's errand, or another parent's errand through the
// parent-scoped path, must look like a missing errand.
func TestCancelIsScopedToTheCallerTaskAndParent(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand

	_, err := h.service.Cancel(context.Background(), CancelRequest{
		ErrandID: errand.ID, TaskID: "task-2", Source: models.ErrandCancelSourceUser,
	})
	assertReason(t, err, ReasonNotFound)
	_, err = h.service.Cancel(context.Background(), CancelRequest{
		ErrandID: errand.ID, TaskID: testTask, ParentSessionID: "someone-else", Source: models.ErrandCancelSourceParent,
	})
	assertReason(t, err, ReasonNotFound)
	_, err = h.service.Cancel(context.Background(), CancelRequest{
		ErrandID: "ghost", TaskID: testTask, Source: models.ErrandCancelSourceUser,
	})
	assertReason(t, err, ReasonNotFound)

	stored, _ := h.store.GetErrand(context.Background(), errand.ID)
	if stored.Lifecycle != models.ErrandLifecycleLaunched || len(h.stopper.stops()) != 0 {
		t.Fatal("a refused cancel changed the errand or stopped its runtime")
	}
}

func TestCancelledStartingErrandStopsTheSessionItJustLaunched(t *testing.T) {
	h := newHarness(t)
	h.launcher.onCall = func(request LaunchRequest) {
		if _, err := h.service.Cancel(context.Background(), CancelRequest{
			ErrandID: request.ErrandID, TaskID: testTask, Source: models.ErrandCancelSourceUser,
		}); err != nil {
			t.Errorf("cancel during launch: %v", err)
		}
	}
	result := h.mustStart("audit")

	if result.Status != models.ErrandStatusCancelled || result.Errand.Lifecycle != models.ErrandLifecycleCancelled {
		t.Fatalf("result = %s/%s, want the cancel to win over the launch", result.Status, result.Errand.Lifecycle)
	}
	if len(h.stopper.stops()) == 0 || h.stopper.stops()[len(h.stopper.stops())-1] != result.Errand.ID {
		t.Fatalf("stops = %v, want the just-launched session stopped", h.stopper.stops())
	}
}

// @covers AC-TASKS-ERRANDS-007.6
func TestCancelAllCancelsLiveErrandsAndDeliversOncePerParent(t *testing.T) {
	h := newHarness(t)
	first, second := h.mustStart("first").Errand, h.mustStart("second").Errand
	done := h.mustStart("done").Errand
	h.store.setSessionState(done.ID, models.TaskSessionStateWaitingForInput)
	if _, _, err := h.service.update(context.Background(), done.ID, func(e *models.Errand) bool {
		e.LastOutcome, e.AwaitingResult = models.ErrandOutcomeDone, false
		return true
	}); err != nil {
		t.Fatalf("settle: %v", err)
	}

	count, err := h.service.CancelAll(context.Background(), testTask)
	if err != nil || count != 2 {
		t.Fatalf("CancelAll = %d, %v, want 2 cancelled", count, err)
	}
	for _, id := range []string{first.ID, second.ID} {
		row, _ := h.store.GetErrand(context.Background(), id)
		if row.Lifecycle != models.ErrandLifecycleCancelled || row.CancelSource != models.ErrandCancelSourceUser {
			t.Fatalf("errand %s = %s/%s, want cancelled by the user", id, row.Lifecycle, row.CancelSource)
		}
	}
	if row, _ := h.store.GetErrand(context.Background(), done.ID); row.Lifecycle != models.ErrandLifecycleLaunched {
		t.Fatal("CancelAll cancelled an errand that was already settled")
	}
	if got := h.deliverer.delivered(); len(got) != 1 || got[0] != testParent {
		t.Fatalf("deliveries = %v, want one per parent", got)
	}

	count, err = h.service.CancelAll(context.Background(), testTask)
	if err != nil || count != 0 {
		t.Fatalf("second CancelAll = %d, %v, want a successful no-op", count, err)
	}
}

func TestCancelAllRefusesBlankTask(t *testing.T) {
	h := newHarness(t)
	if _, err := h.service.CancelAll(context.Background(), ""); err == nil {
		t.Fatal("CancelAll accepted a blank task")
	}
}

// @covers AC-TASKS-ERRANDS-007.2 AC-TASKS-ERRANDS-007.4
func TestCascadeCancelsLiveErrandsAndDiscardsUndeliveredResults(t *testing.T) {
	h := newHarness(t)
	live := h.mustStart("live").Errand
	settled := h.mustStart("settled").Errand
	h.store.setSessionState(settled.ID, models.TaskSessionStateWaitingForInput)
	if _, _, err := h.service.update(context.Background(), settled.ID, func(e *models.Errand) bool {
		e.LastOutcome, e.AwaitingResult = models.ErrandOutcomeDone, false
		e.ResultKind, e.ResultState = models.ErrandResultOutput, models.ErrandResultPending
		return true
	}); err != nil {
		t.Fatalf("settle: %v", err)
	}

	cancelled, err := h.service.CancelForParent(context.Background(), testParent)
	if err != nil || cancelled != 1 {
		t.Fatalf("CancelForParent = %d, %v, want 1", cancelled, err)
	}
	liveRow, _ := h.store.GetErrand(context.Background(), live.ID)
	if liveRow.Lifecycle != models.ErrandLifecycleCancelled || liveRow.CancelSource != models.ErrandCancelSourceCascade ||
		liveRow.ResultState != models.ErrandResultNone {
		t.Fatalf("live errand = %+v, want cascade-cancelled with no result", liveRow)
	}
	settledRow, _ := h.store.GetErrand(context.Background(), settled.ID)
	if settledRow.ResultState != models.ErrandResultDiscarded || settledRow.Lifecycle != models.ErrandLifecycleLaunched {
		t.Fatalf("settled errand = %+v, want its pending result discarded", settledRow)
	}
	if len(h.deliverer.delivered()) != 0 {
		t.Fatalf("a cascade delivered %v, want nothing delivered", h.deliverer.delivered())
	}
}

func TestCancelForTaskCancelsQueuedErrandsToo(t *testing.T) {
	h := newHarness(t)
	h.launcher.behave = func(int, LaunchRequest) error { return ErrCeilingRefused }
	queued := h.mustStart("queued").Errand
	cancelled, err := h.service.CancelForTask(context.Background(), testTask)
	if err != nil || cancelled != 1 {
		t.Fatalf("CancelForTask = %d, %v, want the queued errand cancelled", cancelled, err)
	}
	row, _ := h.store.GetErrand(context.Background(), queued.ID)
	if row.Lifecycle != models.ErrandLifecycleCancelled || row.CancelSource != models.ErrandCancelSourceCascade {
		t.Fatalf("queued errand = %s/%s, want cascade-cancelled", row.Lifecycle, row.CancelSource)
	}
}

func TestCancelOnSessionStopRecordsAUserCancelWithoutStoppingAgain(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand
	if err := h.service.CancelOnSessionStop(context.Background(), errand.ID); err != nil {
		t.Fatalf("CancelOnSessionStop: %v", err)
	}
	row, _ := h.store.GetErrand(context.Background(), errand.ID)
	if row.Lifecycle != models.ErrandLifecycleCancelled || row.CancelSource != models.ErrandCancelSourceUser {
		t.Fatalf("errand = %s/%s, want cancelled by the user", row.Lifecycle, row.CancelSource)
	}
	if len(h.stopper.stops()) != 0 {
		t.Fatal("the stop hook stopped the runtime that its caller is already stopping")
	}
	if err := h.service.CancelOnSessionStop(context.Background(), testParent); err != nil {
		t.Fatalf("an ordinary session must be ignored, got %v", err)
	}
}
