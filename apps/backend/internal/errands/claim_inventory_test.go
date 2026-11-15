package errands

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	"strings"
	"testing"
	"time"
)

func serviceClaimInventory(t *testing.T, store Store, parent string) []models.ErrandClaimCandidate {
	t.Helper()
	rows, err := store.ListErrandsByParent(t.Context(), parent)
	if err != nil {
		t.Fatal(err)
	}
	return models.ErrandInventory(rows)
}

type insertingClaimStore struct {
	Store
	calls int
}

func (s *insertingClaimStore) ClaimResults(ctx context.Context, task, parent string, route models.ErrandDeliveryRoute, token string,
	candidates []models.ErrandClaimCandidate, at time.Time, inventory []models.ErrandClaimCandidate) ([]*models.Errand, error) {
	s.calls++
	if s.calls == 1 {
		err := s.CreateErrand(ctx, &models.Errand{ID: "phantom", TaskID: task, ParentSessionID: parent,
			Title: "New sibling", Objective: "Inspect sibling", AgentProfileID: "profile-1", FirstTurnID: "phantom-turn", SubmissionID: "phantom-submission",
			Lifecycle: models.ErrandLifecycleCancelled, CancelSource: models.ErrandCancelSourceUser, ResultState: models.ErrandResultPending,
			ResultKind: models.ErrandResultCancelled, ResultText: "new sibling output", CreatedAt: at})
		if err != nil {
			return nil, err
		}
	}
	return s.Store.ClaimResults(ctx, task, parent, route, token, candidates, at, inventory)
}

func TestComposeParentTurnReprojectsNewInventoryMember(t *testing.T) {
	h, _ := pendingResultHarness(t, "original output")
	store := &insertingClaimStore{Store: h.store}
	h.service.store = store
	result, err := h.service.ComposeParentTurn(t.Context(), testParent, models.ErrandDeliveryTurnContext)
	if err != nil || store.calls != 2 || !strings.Contains(result.Block, "new sibling output") || !strings.Contains(result.Block, "original output") {
		t.Fatalf("phantom inventory was omitted: result=%+v calls=%d err=%v", result, store.calls, err)
	}
}
