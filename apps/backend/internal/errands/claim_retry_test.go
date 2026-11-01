package errands

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type replacingClaimStore struct {
	Store
	calls      int
	ids        []string
	always     bool
	tokens     []string
	afterClaim func()
}

func (s *replacingClaimStore) ClaimResults(ctx context.Context, task, parent string, route models.ErrandDeliveryRoute, token string,
	candidates []models.ErrandClaimCandidate, at time.Time, inventory []models.ErrandClaimCandidate) ([]*models.Errand, error) {
	s.calls++
	s.tokens = append(s.tokens, token)
	if s.calls == 1 || s.always {
		for _, id := range s.ids {
			row, err := s.GetErrand(ctx, id)
			if err != nil {
				return nil, err
			}
			row.ResultText = "authoritative replacement"
			if err := s.UpdateErrand(ctx, row); err != nil {
				return nil, err
			}
		}
	}
	rows, err := s.Store.ClaimResults(ctx, task, parent, route, token, candidates, at, inventory)
	if s.afterClaim != nil {
		s.afterClaim()
	}
	return rows, err
}

func TestAwaitCancellationAfterClaimCannotReturnSuccess(t *testing.T) {
	h, id := settleFirstTurn(t, "must not escape after cancellation")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	h.service.store = &replacingClaimStore{Store: h.store, afterClaim: cancel}
	result, err := h.service.Await(ctx, AwaitRequest{ParentSessionID: testParent, ErrandIDs: []string{id}, Mode: "all", Token: "cancel-after-claim"})
	if !errors.Is(err, context.Canceled) || len(result.Errands) != 0 {
		t.Fatalf("cancelled await returned success: result=%+v err=%v", result, err)
	}
	row, err := h.store.GetErrand(t.Context(), id)
	if err != nil || row.ResultState != models.ErrandResultPending {
		t.Fatalf("cancelled claim was stranded: %+v err=%v", row, err)
	}
}

func TestClaimSnapshotRetryIsBounded(t *testing.T) {
	for _, await := range []bool{false, true} {
		t.Run(map[bool]string{false: "compose", true: "await"}[await], func(t *testing.T) {
			h, id := settleFirstTurn(t, "original")
			store := &replacingClaimStore{Store: h.store, ids: []string{id}, always: true}
			h.service.store = store
			var err error
			var token, text string
			if await {
				result, callErr := h.service.Await(t.Context(), AwaitRequest{ParentSessionID: testParent, ErrandIDs: []string{id}, Mode: "all", Token: "bounded-await"})
				err, token = callErr, result.Token
				for _, item := range result.Errands {
					text += item.Result
				}
			} else {
				result, callErr := h.service.ComposeParentTurn(t.Context(), testParent, models.ErrandDeliveryTurnContext)
				err, token, text = callErr, result.Token, result.Block
			}
			if !errors.Is(err, models.ErrErrandVersionConflict) || store.calls != 3 || text != "" || token == "" {
				t.Fatalf("unbounded/false-success retry: token=%q text=%q calls=%d err=%v", token, text, store.calls, err)
			}
			for _, used := range store.tokens {
				if used != token {
					t.Fatal("retry generated a new claim token")
				}
			}
		})
	}
}

func TestComposeParentTurnReprojectsStaleClaim(t *testing.T) {
	h, id := settleFirstTurn(t, "stale output")
	store := &replacingClaimStore{Store: h.store, ids: []string{id}}
	h.service.store = store
	result, err := h.service.ComposeParentTurn(context.Background(), testParent, models.ErrandDeliveryTurnContext)
	if err != nil || !strings.Contains(result.Block, "authoritative replacement") || strings.Contains(result.Block, "stale output") {
		t.Fatalf("stale snapshot was not reprojected: result=%+v err=%v calls=%d", result, err, store.calls)
	}
}

func TestAwaitReprojectsStaleClaim(t *testing.T) {
	h, id := settleFirstTurn(t, "stale output")
	store := &replacingClaimStore{Store: h.store, ids: []string{id}}
	h.service.store = store
	result, err := h.service.Await(context.Background(), AwaitRequest{
		ParentSessionID: testParent, ErrandIDs: []string{id}, Mode: "all", Token: "stale-await",
	})
	if err != nil || len(result.Errands) != 1 || result.Errands[0].Result != "authoritative replacement" {
		t.Fatalf("stale await was not reprojected: result=%+v err=%v calls=%d", result, err, store.calls)
	}
}
