package errands

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-ERRANDS-008.2
func TestDrainQueuedLaunchesInCreationOrderWithTheStoredIdentityAndSnapshot(t *testing.T) {
	h := newHarness(t)
	h.launcher.behave = func(int, LaunchRequest) error { return ErrCeilingRefused }
	first, second := h.mustStart("first").Errand, h.mustStart("second").Errand
	h.mcp.ids = []string{"changed-later"}

	h.launcher.behave = nil
	h.service.DrainQueued(context.Background())

	calls := h.launcher.calls()[2:]
	if len(calls) != 2 || calls[0].ErrandID != first.ID || calls[1].ErrandID != second.ID {
		t.Fatalf("drain launched %+v, want first then second", calls)
	}
	for index, call := range calls {
		identity := h.launcher.identity[index+2]
		if identity.SessionID != call.ErrandID || identity.TurnID != call.TurnID || identity.SubmissionID != call.SubmissionID {
			t.Fatalf("drain identity %+v does not match the errand's reserved identity", identity)
		}
		if len(call.MCPServerIDs) != 2 || call.MCPServerIDs[0] != "mcp-1" {
			t.Fatalf("drain MCP = %v, want the stored snapshot", call.MCPServerIDs)
		}
	}
	for _, id := range []string{first.ID, second.ID} {
		if row, _ := h.store.GetErrand(context.Background(), id); row.Lifecycle != models.ErrandLifecycleLaunched {
			t.Fatalf("errand %s lifecycle = %s, want launched", id, row.Lifecycle)
		}
	}
}

func TestDrainQueuedStopsATasksWalkAtItsFirstRefusalButServesOtherTasks(t *testing.T) {
	h := newHarness(t)
	h.store.addSession(&models.TaskSession{ID: "parent-2", TaskID: "task-2", State: models.TaskSessionStateWaitingForInput})
	h.launcher.behave = func(int, LaunchRequest) error { return ErrCeilingRefused }
	h.mustStart("first")
	h.mustStart("second")
	other, err := h.service.Start(context.Background(), StartRequest{
		TaskID: "task-2", ParentSessionID: "parent-2", Title: "other", Objective: "other",
	})
	if err != nil {
		t.Fatalf("Start other task: %v", err)
	}

	h.launcher.behave = func(_ int, request LaunchRequest) error {
		if request.TaskID == testTask {
			return ErrCeilingRefused
		}
		return nil
	}
	before := len(h.launcher.calls())
	h.service.DrainQueued(context.Background())

	calls := h.launcher.calls()[before:]
	perTask := map[string]int{}
	for _, call := range calls {
		perTask[call.TaskID]++
	}
	if perTask[testTask] != 1 || perTask["task-2"] != 1 {
		t.Fatalf("drain attempts per task = %v, want one refused attempt for task-1 and one launch for task-2", perTask)
	}
	if row, _ := h.store.GetErrand(context.Background(), other.Errand.ID); row.Lifecycle != models.ErrandLifecycleLaunched {
		t.Fatalf("other task's errand = %s, want launched", row.Lifecycle)
	}
}

// @covers AC-TASKS-ERRANDS-008.3
func TestDrainQueuedFailsAnErrandWhoseLaunchFailsAndContinues(t *testing.T) {
	h := newHarness(t)
	h.launcher.behave = func(int, LaunchRequest) error { return ErrCeilingRefused }
	broken, healthy := h.mustStart("broken").Errand, h.mustStart("healthy").Errand

	h.launcher.behave = func(_ int, request LaunchRequest) error {
		if request.ErrandID == broken.ID {
			return errors.New("profile no longer exists")
		}
		return nil
	}
	h.service.DrainQueued(context.Background())

	brokenRow, _ := h.store.GetErrand(context.Background(), broken.ID)
	status, err := h.service.Project(context.Background(), brokenRow)
	if err != nil || status != models.ErrandStatusFailed {
		t.Fatalf("broken errand status = %s (%v), want failed", status, err)
	}
	if brokenRow.ResultKind != models.ErrandResultFailure || brokenRow.ResultState != models.ErrandResultPending {
		t.Fatalf("broken errand result = %s/%s, want a pending failure result", brokenRow.ResultKind, brokenRow.ResultState)
	}
	if healthyRow, _ := h.store.GetErrand(context.Background(), healthy.ID); healthyRow.Lifecycle != models.ErrandLifecycleLaunched {
		t.Fatalf("healthy errand = %s, want launched after the earlier failure", healthyRow.Lifecycle)
	}
}

func TestDrainQueuedSkipsAnErrandCancelledWhileQueued(t *testing.T) {
	h := newHarness(t)
	h.launcher.behave = func(int, LaunchRequest) error { return ErrCeilingRefused }
	errand := h.mustStart("audit").Errand
	if _, err := h.service.Cancel(context.Background(), cancelRequest(errand, models.ErrandCancelSourceUser)); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	before := len(h.launcher.calls())
	h.launcher.behave = nil
	h.service.DrainQueued(context.Background())
	if len(h.launcher.calls()) != before {
		t.Fatal("drain launched a cancelled errand")
	}
}

func TestDrainQueuedLeavesAnErrandHeldByAnInFlightStart(t *testing.T) {
	h := newHarness(t)
	h.launcher.behave = func(int, LaunchRequest) error { return ErrCeilingRefused }
	errand := h.mustStart("audit").Errand
	before := len(h.launcher.calls())

	h.launcher.behave = nil
	if !h.service.launching.acquire(errand.ID) {
		t.Fatal("could not hold the errand")
	}
	h.service.DrainQueued(context.Background())
	h.service.launching.release(errand.ID)
	if len(h.launcher.calls()) != before {
		t.Fatal("drain launched an errand that another launch holds")
	}
}
