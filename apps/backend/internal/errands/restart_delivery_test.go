package errands

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-ERRANDS-004.8
func TestRestartDeliversAResultThatWasStillPending(t *testing.T) {
	ctx := context.Background()
	h, errandID := settleFirstTurn(t, "the answer")

	row, err := h.store.GetErrand(ctx, errandID)
	if err != nil {
		t.Fatalf("GetErrand: %v", err)
	}
	if row.ResultState != models.ErrandResultPending {
		t.Fatalf("result state = %s before restart, want pending", row.ResultState)
	}

	// The settle that produced the result already asked for a notification.
	// A restart must ask again, because nothing shows the result was delivered.
	h.deliverer.mu.Lock()
	h.deliverer.parents = nil
	h.deliverer.mu.Unlock()

	if err := h.service.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	h.deliverer.mu.Lock()
	parents := append([]string(nil), h.deliverer.parents...)
	h.deliverer.mu.Unlock()
	if len(parents) != 1 || parents[0] != testParent {
		t.Fatalf("restart delivered to parents %v, want exactly [%s]", parents, testParent)
	}
}
