package errands

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestAwaitOutcomeMetadata(t *testing.T) {
	t.Run("ready any", func(t *testing.T) {
		h, id := settleFirstTurn(t, "answer")
		result, err := h.service.Await(context.Background(), AwaitRequest{ParentSessionID: testParent, ErrandIDs: []string{id}, Mode: awaitModeAny})
		if err != nil || result.EndReason != "any_ready" || result.DurationMS < 0 {
			t.Fatalf("await = %+v, %v", result, err)
		}
	})
	t.Run("immediate timeout", func(t *testing.T) {
		h := newHarness(t)
		row := h.mustStart("pending").Errand
		result, err := h.service.Await(context.Background(), AwaitRequest{ParentSessionID: testParent, ErrandIDs: []string{row.ID}, Mode: awaitModeAll})
		if err != nil || result.EndReason != "timeout" || result.DurationMS < 0 {
			t.Fatalf("await = %+v, %v", result, err)
		}
	})
	t.Run("parent ended", func(t *testing.T) {
		h := newHarness(t)
		row := h.mustStart("pending").Errand
		req := AwaitRequest{ParentSessionID: testParent, ErrandIDs: []string{row.ID}, Mode: awaitModeAll, Token: "ended"}
		result, err := h.service.finishAwait(context.Background(), req, []*models.Errand{row}, false, true)
		if err != nil || result.EndReason != "parent_ended" || !result.Ended {
			t.Fatalf("await = %+v, %v", result, err)
		}
	})
}
