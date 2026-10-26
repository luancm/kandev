package errands

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func eventually(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestCascadeParentRunsOffTheCallerAndStopsRuntimes(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand
	if h.service.CascadeParent(testParent) {
		t.Fatal("CascadeParent queued work before the worker started")
	}

	h.service.StartWorkers(context.Background())
	h.service.StartWorkers(context.Background())
	t.Cleanup(h.service.StopWorkers)
	if !h.service.CascadeParent(testParent) {
		t.Fatal("CascadeParent refused work while the worker runs")
	}
	eventually(t, "the cascade to cancel the errand", func() bool {
		row, _ := h.store.GetErrand(context.Background(), errand.ID)
		return row.Lifecycle == models.ErrandLifecycleCancelled
	})
	eventually(t, "the cascade to stop the runtime", func() bool { return len(h.stopper.stops()) == 1 })
	row, _ := h.store.GetErrand(context.Background(), errand.ID)
	if row.CancelSource != models.ErrandCancelSourceCascade {
		t.Fatalf("cancel source = %s, want cascade", row.CancelSource)
	}
}

func TestCascadeWorkerRestartsAfterStop(t *testing.T) {
	h := newHarness(t)
	h.service.StartWorkers(context.Background())
	h.service.StopWorkers()
	h.service.StopWorkers()
	if h.service.CascadeParent(testParent) {
		t.Fatal("CascadeParent queued work on a stopped worker")
	}

	h.service.StartWorkers(context.Background())
	t.Cleanup(h.service.StopWorkers)
	errand := h.mustStart("audit").Errand
	if !h.service.CascadeParent(testParent) {
		t.Fatal("CascadeParent refused work after a restart")
	}
	eventually(t, "the restarted worker to cancel", func() bool {
		row, _ := h.store.GetErrand(context.Background(), errand.ID)
		return row.Lifecycle == models.ErrandLifecycleCancelled
	})
}

func TestCascadeParentDeduplicatesWaitingParents(t *testing.T) {
	var worker cascadeWorker
	release := make(chan struct{})
	ran := make(chan string, 8)
	worker.start(context.Background(), func(_ context.Context, job cascadeJob) {
		ran <- job.id
		<-release
	})
	t.Cleanup(worker.stop)

	worker.enqueue(cascadeJob{id: "parent-a"})
	if got := <-ran; got != "parent-a" {
		t.Fatalf("first run = %s", got)
	}
	worker.enqueue(cascadeJob{id: "parent-b"})
	worker.enqueue(cascadeJob{id: "parent-b"})
	worker.enqueue(cascadeJob{taskWide: true, id: "parent-b"})
	close(release)
	if got := <-ran; got != "parent-b" {
		t.Fatalf("second run = %s", got)
	}
	if got := <-ran; got != "parent-b" {
		t.Fatalf("task-wide job with the same ID must be its own job, got %s", got)
	}
	select {
	case extra := <-ran:
		t.Fatalf("job ran twice: %s", extra)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCascadeTaskCancelsQueuedErrandsOffTheCaller(t *testing.T) {
	h := newHarness(t)
	h.launcher.behave = func(int, LaunchRequest) error { return ErrCeilingRefused }
	queued := h.mustStart("queued").Errand
	h.service.StartWorkers(context.Background())
	t.Cleanup(h.service.StopWorkers)

	if !h.service.CascadeTask(testTask) {
		t.Fatal("CascadeTask refused work while the worker runs")
	}
	eventually(t, "the task cascade to cancel the queued errand", func() bool {
		row, _ := h.store.GetErrand(context.Background(), queued.ID)
		return row.Lifecycle == models.ErrandLifecycleCancelled
	})
}
