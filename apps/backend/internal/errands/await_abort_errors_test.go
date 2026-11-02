package errands

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type awaitLoadFailureStore struct{ Store }

func (s *awaitLoadFailureStore) GetErrand(context.Context, string) (*models.Errand, error) {
	return nil, errors.New("await inventory read failed")
}

func TestAwaitErrorBeforeClaimCommitsAbortTombstone(t *testing.T) {
	h, id := pendingResultHarness(t, "answer")
	h.service.store = &awaitLoadFailureStore{Store: h.store}
	_, err := h.service.Await(context.Background(), AwaitRequest{
		ParentSessionID: testParent, ErrandIDs: []string{id}, Mode: awaitModeAll, Token: "load-error",
	})
	if err == nil {
		t.Fatal("await read failure returned success")
	}
	row, err := h.store.GetErrand(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.store.ClaimResults(context.Background(), testTask, testParent, models.ErrandDeliveryAwait, "load-error",
		[]models.ErrandClaimCandidate{{ID: id, Version: row.Version}}, time.Now().UTC(), serviceClaimInventory(t, h.store, testParent))
	if !errors.Is(err, models.ErrErrandVersionConflict) {
		t.Fatalf("known error failed to fence delayed claim: %v", err)
	}
}
