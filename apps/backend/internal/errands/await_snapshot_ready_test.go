package errands

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type resumedClaimStore struct {
	Store
	memory  *memoryStore
	id      string
	changed chan struct{}
	once    bool
}

func (s *resumedClaimStore) ClaimResults(ctx context.Context, task, parent string, route models.ErrandDeliveryRoute, token string,
	candidates []models.ErrandClaimCandidate, at time.Time, inventory []models.ErrandClaimCandidate) ([]*models.Errand, error) {
	if !s.once {
		s.once = true
		row, err := s.GetErrand(ctx, s.id)
		if err != nil {
			return nil, err
		}
		row.ResultState, row.ResultText, row.LastOutcome = models.ErrandResultNone, "", ""
		row.AwaitingResult = true
		if err := s.UpdateErrand(ctx, row); err != nil {
			return nil, err
		}
		s.memory.setSessionState(s.id, models.TaskSessionStateRunning)
		close(s.changed)
	}
	return s.Store.ClaimResults(ctx, task, parent, route, token, candidates, at, inventory)
}

func TestAwaitStaleSnapshotRechecksReadiness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, id := settleFirstTurn(t, "obsolete result")
		store := &resumedClaimStore{Store: h.store, memory: h.store, id: id, changed: make(chan struct{})}
		h.service.store = store
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan AwaitResult, 1)
		errors := make(chan error, 1)
		go func() {
			result, err := h.service.Await(ctx, AwaitRequest{ParentSessionID: testParent, ErrandIDs: []string{id}, Mode: "all", MaxWaitSeconds: 600, Token: "resumed-await"})
			done <- result
			errors <- err
		}()
		<-store.changed
		synctest.Wait()
		select {
		case result := <-done:
			t.Fatalf("stale readiness ended a live await: %+v err=%v", result, <-errors)
		default:
		}
		row, err := h.store.GetErrand(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		row.ResultState, row.ResultText, row.LastOutcome = models.ErrandResultPending, "successor result", models.ErrandOutcomeDone
		row.AwaitingResult = false
		if err := h.store.UpdateErrand(ctx, row); err != nil {
			t.Fatal(err)
		}
		h.store.setSessionState(id, models.TaskSessionStateWaitingForInput)
		h.service.wakeAwaits(testParent)
		result := <-done
		if err := <-errors; err != nil || len(result.Errands) != 1 || result.Errands[0].Result != "successor result" {
			t.Fatalf("reprojected await: %+v err=%v", result, err)
		}
	})
}
