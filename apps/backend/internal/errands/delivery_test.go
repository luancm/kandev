package errands

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliverySnapshotItemsUseSnakeCaseWireFields(t *testing.T) {
	encoded, err := json.Marshal(DeliveryItem{
		ErrandID: "errand-1", Title: "Review API", Status: models.ErrandStatusDone, Preview: "Reviewed the API.",
	})
	if err != nil {
		t.Fatalf("marshal delivery item: %v", err)
	}
	const want = `{"errand_id":"errand-1","title":"Review API","status":"done","preview":"Reviewed the API."}`
	if string(encoded) != want {
		t.Fatalf("delivery item JSON = %s, want %s", encoded, want)
	}
}

// fakeTurns serves transcript evidence keyed by turn ID.
type fakeTurns struct {
	views map[string]TurnView
}

func (f *fakeTurns) ReadTurn(_ context.Context, _, turnID string) (TurnView, error) {
	return f.views[turnID], nil
}

func (f *fakeTurns) LatestTurn(context.Context, string) (string, TurnView, error) {
	return "", TurnView{}, nil
}

type cancellationTurns struct {
	views    map[string]TurnView
	latestID string
	latest   TurnView
}

func (f *cancellationTurns) ReadTurn(_ context.Context, _, turnID string) (TurnView, error) {
	return f.views[turnID], nil
}

func (f *cancellationTurns) LatestTurn(context.Context, string) (string, TurnView, error) {
	return f.latestID, f.latest, nil
}

// settleFirstTurn starts an errand whose first turn is parent-initiated and
// settles it with text. It returns the harness and the errand ID.
func settleFirstTurn(t *testing.T, text string) (*harness, string) {
	t.Helper()
	h := newHarness(t)
	errand := h.mustStart("audit").Errand
	h.service.SetTurnReader(&fakeTurns{views: map[string]TurnView{
		errand.FirstTurnID: {LastAgentText: text, EarliestUserSender: testParent},
	}})
	h.store.setSessionState(errand.ID, models.TaskSessionStateWaitingForInput)
	err := h.service.OnErrandTurnSettled(context.Background(), TurnSettlement{
		SessionID: errand.ID, TurnID: errand.FirstTurnID, Outcome: models.ErrandOutcomeDone,
	})
	if err != nil {
		t.Fatalf("OnErrandTurnSettled: %v", err)
	}
	return h, errand.ID
}

// @covers AC-TASKS-ERRANDS-003.1
func TestParentInitiatedTurnCapturesItsFinalMessageAsOneResult(t *testing.T) {
	h, id := settleFirstTurn(t, "the answer")
	row, err := h.store.GetErrand(context.Background(), id)
	if err != nil {
		t.Fatalf("GetErrand: %v", err)
	}
	if row.ResultState != models.ErrandResultPending || row.ResultText != "the answer" || row.ResultTurnID == "" {
		t.Fatalf("result = %q/%s turn %q, want one pending result", row.ResultText, row.ResultState, row.ResultTurnID)
	}
	if row.AwaitingResult {
		t.Fatal("a settled parent-initiated turn left the errand awaiting its result")
	}
	if got := h.deliverer.delivered(); len(got) != 1 {
		t.Fatalf("deliveries = %v, want one notification request", got)
	}
}

// @covers AC-TASKS-ERRANDS-003.2
func TestOversizedResultIsBoundedAtACharacterBoundary(t *testing.T) {
	long := strings.Repeat("é", resultByteLimit)
	bounded, truncated := boundResult(long)
	if !truncated || len(bounded) > resultByteLimit || !utf8.ValidString(bounded) {
		t.Fatalf("truncated=%v len=%d valid=%v, want a truncated valid result within 16 KiB",
			truncated, len(bounded), utf8.ValidString(bounded))
	}
	if !strings.Contains(bounded, "get_task_conversation_kandev") {
		t.Fatal("a truncated result does not say where the full transcript is")
	}
	if short, cut := boundResult("short"); cut || short != "short" {
		t.Fatalf("short result = %q truncated=%v, want it unchanged", short, cut)
	}
}

// @covers AC-TASKS-ERRANDS-003.3
func TestUserTurnDoesNotProduceAResult(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand
	h.service.SetTurnReader(&fakeTurns{views: map[string]TurnView{
		"user-turn": {LastAgentText: "chatter", EarliestUserSender: "someone-else"},
	}})
	err := h.service.OnErrandTurnSettled(context.Background(), TurnSettlement{
		SessionID: errand.ID, TurnID: "user-turn", Outcome: models.ErrandOutcomeDone,
	})
	if err != nil {
		t.Fatalf("OnErrandTurnSettled: %v", err)
	}
	row, _ := h.store.GetErrand(context.Background(), errand.ID)
	if row.ResultState == models.ErrandResultPending || row.ResultText != "" {
		t.Fatalf("a user turn produced result %q/%s", row.ResultText, row.ResultState)
	}
	if row.LatestSettledTurnID != "user-turn" {
		t.Fatalf("latest settled turn = %q, want the user turn recorded", row.LatestSettledTurnID)
	}
}

func TestCancelledParentTurnProducesInterruptedFailureAndClearsAwaiting(t *testing.T) {
	h := newHarness(t)
	err := h.mustStart("audit").Errand
	h.service.SetTurnReader(&cancellationTurns{
		views:    map[string]TurnView{err.FirstTurnID: {EarliestUserSender: testParent}},
		latestID: err.FirstTurnID,
		latest:   TurnView{EarliestUserSender: testParent},
	})

	if err := h.service.OnErrandTurnCancelled(context.Background(), err.ID, err.FirstTurnID, err.SubmissionID); err != nil {
		t.Fatalf("OnErrandTurnCancelled: %v", err)
	}
	row, getErr := h.store.GetErrand(context.Background(), err.ID)
	if getErr != nil {
		t.Fatalf("GetErrand: %v", getErr)
	}
	if row.LastOutcome != models.ErrandOutcomeFailed || row.ResultKind != models.ErrandResultInterrupted ||
		row.ResultState != models.ErrandResultPending || row.ResultTurnID != err.FirstTurnID || row.AwaitingResult {
		t.Fatalf("row = outcome %q kind %q state %q turn %q awaiting %v, want an interrupted pending failure with no outstanding bit",
			row.LastOutcome, row.ResultKind, row.ResultState, row.ResultTurnID, row.AwaitingResult)
	}
}

func TestCancelledUserTurnPreservesPriorOutcomeWithoutAResult(t *testing.T) {
	h := newHarness(t)
	err := h.mustStart("audit").Errand
	_, _, updateErr := h.service.update(context.Background(), err.ID, func(row *models.Errand) bool {
		row.LastOutcome = models.ErrandOutcomeDone
		row.LatestSettledTurnID = "previous-turn"
		return true
	})
	if updateErr != nil {
		t.Fatalf("seed prior outcome: %v", updateErr)
	}
	h.service.SetTurnReader(&cancellationTurns{
		views:    map[string]TurnView{"user-turn": {EarliestUserSender: "user-session"}},
		latestID: "user-turn",
		latest:   TurnView{EarliestUserSender: "user-session"},
	})

	if err := h.service.OnErrandTurnCancelled(context.Background(), err.ID, "user-turn", "user-submission"); err != nil {
		t.Fatalf("OnErrandTurnCancelled: %v", err)
	}
	row, getErr := h.store.GetErrand(context.Background(), err.ID)
	if getErr != nil {
		t.Fatalf("GetErrand: %v", getErr)
	}
	if row.LastOutcome != models.ErrandOutcomeDone || row.ResultState == models.ErrandResultPending || row.ResultText != "" ||
		row.LatestSettledTurnID != "user-turn" {
		t.Fatalf("row = outcome %q state %q text %q latest %q, want the previous outcome and no implicit result",
			row.LastOutcome, row.ResultState, row.ResultText, row.LatestSettledTurnID)
	}
	if got := h.deliverer.delivered(); len(got) != 0 {
		t.Fatalf("cancelled user turn triggered result delivery: %v", got)
	}
}

func TestCancelledErrandTurnIgnoresSuccessorAndWrongInitialSubmission(t *testing.T) {
	h := newHarness(t)
	err := h.mustStart("audit").Errand
	h.service.SetTurnReader(&cancellationTurns{
		views:    map[string]TurnView{err.FirstTurnID: {EarliestUserSender: testParent}},
		latestID: "successor-turn",
		latest:   TurnView{EarliestUserSender: testParent},
	})

	if err := h.service.OnErrandTurnCancelled(context.Background(), err.ID, err.FirstTurnID, err.SubmissionID); err != nil {
		t.Fatalf("stale OnErrandTurnCancelled: %v", err)
	}
	row, getErr := h.store.GetErrand(context.Background(), err.ID)
	if getErr != nil {
		t.Fatalf("GetErrand: %v", getErr)
	}
	if row.AwaitingResult == false || row.ResultState == models.ErrandResultPending || row.LastOutcome != "" {
		t.Fatalf("stale cancellation changed successor state: awaiting %v, result %q, outcome %q",
			row.AwaitingResult, row.ResultState, row.LastOutcome)
	}

	h.service.SetTurnReader(&cancellationTurns{
		views:    map[string]TurnView{err.FirstTurnID: {EarliestUserSender: testParent}},
		latestID: err.FirstTurnID,
		latest:   TurnView{EarliestUserSender: testParent},
	})
	if err := h.service.OnErrandTurnCancelled(context.Background(), err.ID, err.FirstTurnID, "wrong-submission"); err != nil {
		t.Fatalf("wrong-submission OnErrandTurnCancelled: %v", err)
	}
	row, getErr = h.store.GetErrand(context.Background(), err.ID)
	if getErr != nil {
		t.Fatalf("GetErrand after wrong submission: %v", getErr)
	}
	if !row.AwaitingResult || row.ResultState == models.ErrandResultPending || row.LastOutcome != "" {
		t.Fatalf("wrong submission changed errand: awaiting %v, result %q, outcome %q",
			row.AwaitingResult, row.ResultState, row.LastOutcome)
	}
}

// @covers AC-TASKS-ERRANDS-003.4
func TestLaterParentTurnSupersedesAnUndeliveredResult(t *testing.T) {
	h, id := settleFirstTurn(t, "first answer")
	a := models.ErrandParentAdmission{TaskID: testTask, ParentSessionID: testParent, ErrandID: id,
		SenderSessionID: testParent, TurnID: "successor-turn", SubmissionID: "successor-submission"}
	if _, err := h.service.ReserveParentAdmission(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err := h.service.DecideParentAdmission(context.Background(), a, models.ErrandAdmissionAccepted); err != nil {
		t.Fatal(err)
	}
	row, _ := h.store.GetErrand(context.Background(), id)
	if row.ResultState != models.ErrandResultSuperseded {
		t.Fatalf("result state = %s, want superseded by the new parent turn", row.ResultState)
	}
}

// @covers AC-TASKS-ERRANDS-004.9
func TestAwaitClaimsTheResultOnceAndAnotherAwaitSeesOnlyTheRoute(t *testing.T) {
	h, id := settleFirstTurn(t, "the answer")
	ctx := context.Background()
	first, err := h.service.Await(ctx, AwaitRequest{
		ParentSessionID: testParent, ErrandIDs: []string{id}, Mode: awaitModeAll, Token: "token-a",
	})
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	if first.EndReason != "all_ready" || first.DurationMS < 0 {
		t.Fatalf("await metadata = %+v", first)
	}
	if len(first.Errands) != 1 || first.Errands[0].Result != "the answer" {
		t.Fatalf("first await = %+v, want the result", first.Errands)
	}
	second, err := h.service.Await(ctx, AwaitRequest{
		ParentSessionID: testParent, ErrandIDs: []string{id}, Mode: awaitModeAll, Token: "token-b",
	})
	if err != nil {
		t.Fatalf("second Await: %v", err)
	}
	if second.Errands[0].Result != "" || second.Errands[0].DeliveredVia != string(models.ErrandDeliveryAwait) {
		t.Fatalf("second await = %+v, want no text and delivered via await", second.Errands[0])
	}
}

// @covers AC-TASKS-ERRANDS-004.9
func TestReleasedAwaitMakesTheResultDeliverableAgain(t *testing.T) {
	h, id := settleFirstTurn(t, "the answer")
	ctx := context.Background()
	if _, err := h.service.Await(ctx, AwaitRequest{
		ParentSessionID: testParent, ErrandIDs: []string{id}, Mode: awaitModeAll, Token: "token-a",
	}); err != nil {
		t.Fatalf("Await: %v", err)
	}
	if err := h.service.ReleaseAwait(ctx, "token-a"); err != nil {
		t.Fatalf("ReleaseAwait: %v", err)
	}
	row, _ := h.store.GetErrand(ctx, id)
	if row.ResultState != models.ErrandResultPending || row.DeliveryToken != "" {
		t.Fatalf("after release result = %s token %q, want pending with no token", row.ResultState, row.DeliveryToken)
	}
}

// @covers AC-TASKS-ERRANDS-004.9
func TestStatusBlockClaimsResultsAndReleaseReturnsThem(t *testing.T) {
	h, id := settleFirstTurn(t, "the answer")
	ctx := context.Background()
	composition, err := h.service.ComposeParentTurn(ctx, testParent, models.ErrandDeliveryTurnContext)
	if err != nil {
		t.Fatalf("ComposeParentTurn: %v", err)
	}
	if composition.Token == "" || !strings.Contains(composition.Block, "the answer") || !strings.Contains(composition.Block, id) {
		t.Fatalf("composition = %+v, want a block carrying the result", composition)
	}
	row, _ := h.store.GetErrand(ctx, id)
	if row.ResultState != models.ErrandResultDelivered || row.DeliveryToken != composition.Token {
		t.Fatalf("after compose result = %s token %q, want delivered under the token", row.ResultState, row.DeliveryToken)
	}
	if err := h.service.ReleaseTurnClaim(ctx, composition.Token); err != nil {
		t.Fatalf("ReleaseTurnClaim: %v", err)
	}
	row, _ = h.store.GetErrand(ctx, id)
	if row.ResultState != models.ErrandResultPending {
		t.Fatalf("after release result = %s, want pending", row.ResultState)
	}
}

type pausingListStore struct {
	Store
	mu     sync.Mutex
	calls  int
	listed chan struct{}
	resume chan struct{}
}

func (s *pausingListStore) ListErrands(ctx context.Context) ([]*models.Errand, error) {
	rows, err := s.Store.ListErrands(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.calls++
	pause := s.calls == 1
	s.mu.Unlock()
	if pause {
		close(s.listed)
		<-s.resume
	}
	return rows, nil
}

type replaceClaimOnConflictStore struct {
	Store
	once             sync.Once
	replacementToken string
	replacementRoute models.ErrandDeliveryRoute
	replacementAt    time.Time
}

func (s *replaceClaimOnConflictStore) ListErrandResultClaims(ctx context.Context, taskID, parentID, token string) ([]models.ErrandResultClaim, error) {
	claims, err := s.Store.ListErrandResultClaims(ctx, taskID, parentID, token)
	if err != nil || len(claims) == 0 {
		return claims, err
	}
	var replaceErr error
	s.once.Do(func() {
		current, err := s.GetErrand(ctx, claims[0].ErrandID)
		if err != nil {
			replaceErr = err
			return
		}
		current.ResultState = models.ErrandResultDelivered
		current.DeliveryToken = s.replacementToken
		current.DeliveredRoute = s.replacementRoute
		current.DeliveredAt = &s.replacementAt
		replaceErr = s.UpdateErrand(ctx, current)
		if replaceErr != nil {
			return
		}
		memory := s.Store.(*memoryStore)
		memory.mu.Lock()
		defer memory.mu.Unlock()
		memory.claims[current.DeliveryToken] = []models.ErrandResultClaim{{ErrandID: current.ID, ResultRevision: current.ResultRevision,
			ParentAdmissionGeneration: current.ParentAdmissionGeneration, Token: current.DeliveryToken, Route: current.DeliveredRoute}}
	})
	return claims, replaceErr
}

func releaseDelivery(h *harness, route models.ErrandDeliveryRoute, token string) error {
	if route == models.ErrandDeliveryAwait {
		return h.service.ReleaseAwait(context.Background(), token)
	}
	return h.service.ReleaseTurnClaim(context.Background(), token)
}

func assertClaimRetained(t *testing.T, got, want *models.Errand) {
	t.Helper()
	if got.ResultState != want.ResultState || got.DeliveryToken != want.DeliveryToken ||
		got.DeliveredRoute != want.DeliveredRoute || got.ResultText != want.ResultText ||
		got.ResultKind != want.ResultKind || got.ResultTurnID != want.ResultTurnID ||
		got.Version != want.Version || !sameDeliveryTime(got.DeliveredAt, want.DeliveredAt) {
		t.Fatalf("replacement claim changed: got state=%q token=%q route=%q result=%q version=%d delivered_at=%v; want state=%q token=%q route=%q result=%q version=%d delivered_at=%v",
			got.ResultState, got.DeliveryToken, got.DeliveredRoute, got.ResultText, got.Version, got.DeliveredAt,
			want.ResultState, want.DeliveryToken, want.DeliveredRoute, want.ResultText, want.Version, want.DeliveredAt)
	}
}

func sameDeliveryTime(got, want *time.Time) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return got.Equal(*want)
}

func oppositeDeliveryRoute(route models.ErrandDeliveryRoute) models.ErrandDeliveryRoute {
	if route == models.ErrandDeliveryAwait {
		return models.ErrandDeliveryTurnContext
	}
	return models.ErrandDeliveryAwait
}

// @covers AC-TASKS-ERRANDS-004.4
func TestReleaseClaimDoesNotReleaseReplacementToken(t *testing.T) {
	for _, route := range []models.ErrandDeliveryRoute{models.ErrandDeliveryAwait, models.ErrandDeliveryTurnContext} {
		t.Run(string(route), func(t *testing.T) {
			h, id := settleFirstTurn(t, "the answer")
			ctx := context.Background()
			var tokenA string
			if route == models.ErrandDeliveryAwait {
				tokenA = "token-a"
			}
			tokenA = claimDeliveryForErrand(t, h, id, route, tokenA)

			store := &pausingListStore{
				Store: h.store, listed: make(chan struct{}), resume: make(chan struct{}),
			}
			h.service.store = store
			resumeOnce := sync.Once{}
			resume := func() { resumeOnce.Do(func() { close(store.resume) }) }
			defer resume()
			releaseDone := make(chan error, 1)
			go func() { releaseDone <- releaseDelivery(h, route, tokenA) }()
			select {
			case <-store.listed:
			case <-time.After(5 * time.Second):
				t.Fatal("release A did not pause after reading its claim")
			}

			if err := releaseDelivery(h, route, tokenA); err != nil {
				t.Fatalf("release A: %v", err)
			}
			releasedA, err := h.store.GetErrand(ctx, id)
			if err != nil {
				t.Fatalf("GetErrand after releasing A: %v", err)
			}
			if releasedA.ResultState != models.ErrandResultPending || releasedA.DeliveryToken != "" {
				t.Fatalf("release A left state=%q token=%q, want pending without a token", releasedA.ResultState, releasedA.DeliveryToken)
			}

			waiter := h.service.awaits.register(testParent, "observe-release")
			defer h.service.awaits.remove(waiter)
			tokenB := "token-b"
			if route == models.ErrandDeliveryTurnContext {
				tokenB = ""
			}
			tokenB = claimDeliveryForErrand(t, h, id, route, tokenB)
			before, err := h.store.GetErrand(ctx, id)
			if err != nil {
				t.Fatalf("GetErrand after claiming B: %v", err)
			}
			beforeDeliveries := len(h.deliverer.delivered())
			beforeEvents := len(h.publisher.statuses())

			resume()
			select {
			case err := <-releaseDone:
				if err != nil {
					t.Fatalf("stale release A: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("stale release A did not finish after the barrier opened")
			}
			after, err := h.store.GetErrand(ctx, id)
			if err != nil {
				t.Fatalf("GetErrand after stale release A: %v", err)
			}
			assertClaimRetained(t, after, before)
			select {
			case <-waiter.wake:
				t.Fatal("stale release A woke parent waiters")
			default:
			}
			if got := len(h.deliverer.delivered()); got != beforeDeliveries {
				t.Fatalf("stale release A queued %d new deliveries, want none", got-beforeDeliveries)
			}
			if got := len(h.publisher.statuses()); got != beforeEvents {
				t.Fatalf("stale release A published %d new events, want none", got-beforeEvents)
			}

			if err := releaseDelivery(h, route, tokenB); err != nil {
				t.Fatalf("release B positive control: %v", err)
			}
			releasedB, err := h.store.GetErrand(ctx, id)
			if err != nil {
				t.Fatalf("GetErrand after releasing B: %v", err)
			}
			if releasedB.ResultState != models.ErrandResultPending || releasedB.DeliveryToken != "" {
				t.Fatalf("release B left state=%q token=%q, want pending without a token", releasedB.ResultState, releasedB.DeliveryToken)
			}
			select {
			case <-waiter.wake:
			default:
				t.Fatal("matching release B must wake parent waiters")
			}
			if got := len(h.publisher.statuses()); got != beforeEvents+1 {
				t.Fatalf("matching release B published %d new events, want one", got-beforeEvents)
			}
			if got := len(h.deliverer.delivered()); got != beforeDeliveries+1 {
				t.Fatalf("matching release B queued %d new deliveries, want one", got-beforeDeliveries)
			}
		})
	}
}

// @covers AC-TASKS-ERRANDS-004.4
func TestReleaseClaimChecksRouteAuthority(t *testing.T) {
	for _, claimRoute := range []models.ErrandDeliveryRoute{models.ErrandDeliveryAwait, models.ErrandDeliveryTurnContext, models.ErrandDeliveryNotification} {
		t.Run(string(claimRoute), func(t *testing.T) {
			h, id := settleFirstTurn(t, "the answer")
			token := "token-a"
			if claimRoute != models.ErrandDeliveryAwait {
				token = ""
			}
			token = claimDeliveryForErrand(t, h, id, claimRoute, token)
			wrongReleaseRoute := oppositeDeliveryRoute(claimRoute)
			if err := releaseDelivery(h, wrongReleaseRoute, token); err != nil {
				t.Fatalf("wrong-route release: %v", err)
			}
			row, err := h.store.GetErrand(context.Background(), id)
			if err != nil {
				t.Fatalf("GetErrand: %v", err)
			}
			if row.ResultState != models.ErrandResultDelivered || row.DeliveryToken != token || row.DeliveredRoute != claimRoute {
				t.Fatalf("wrong-route release changed claim: state=%q token=%q route=%q", row.ResultState, row.DeliveryToken, row.DeliveredRoute)
			}
			if err := releaseDelivery(h, claimRoute, token); err != nil {
				t.Fatalf("matching-route release positive control: %v", err)
			}
			row, err = h.store.GetErrand(context.Background(), id)
			if err != nil {
				t.Fatalf("GetErrand after positive control: %v", err)
			}
			if row.ResultState != models.ErrandResultPending || row.DeliveryToken != "" {
				t.Fatalf("matching-route release left state=%q token=%q", row.ResultState, row.DeliveryToken)
			}
		})
	}
}

// @covers AC-TASKS-ERRANDS-004.4
func TestReleaseClaimRechecksExactIdentityAtStorageBoundary(t *testing.T) {
	for _, route := range []models.ErrandDeliveryRoute{models.ErrandDeliveryAwait, models.ErrandDeliveryTurnContext} {
		for _, replacement := range []string{"token-and-route", "route-only"} {
			t.Run(string(route)+"/"+replacement, func(t *testing.T) {
				h, id := settleFirstTurn(t, "the answer")
				tokenA := "token-a"
				if route == models.ErrandDeliveryTurnContext {
					tokenA = ""
				}
				tokenA = claimDeliveryForErrand(t, h, id, route, tokenA)
				replacementToken := "token-b"
				if replacement == "route-only" {
					replacementToken = tokenA
				}
				replacementRoute := oppositeDeliveryRoute(route)
				replacementAt := time.Date(2026, time.October, 30, 9, 0, 0, 0, time.UTC)
				before, err := h.store.GetErrand(context.Background(), id)
				if err != nil {
					t.Fatalf("GetErrand before CAS conflict: %v", err)
				}
				h.service.store = &replaceClaimOnConflictStore{
					Store: h.store, replacementToken: replacementToken, replacementRoute: replacementRoute,
					replacementAt: replacementAt,
				}
				waiter := h.service.awaits.register(testParent, "observe-cas-conflict")
				defer h.service.awaits.remove(waiter)
				if err := releaseDelivery(h, route, tokenA); err != nil {
					t.Fatalf("release A: %v", err)
				}
				row, err := h.store.GetErrand(context.Background(), id)
				if err != nil {
					t.Fatalf("GetErrand after CAS conflict: %v", err)
				}
				if row.ResultState != models.ErrandResultDelivered || row.DeliveryToken != replacementToken ||
					row.DeliveredRoute != replacementRoute || row.DeliveredAt == nil || !row.DeliveredAt.Equal(replacementAt) || row.Version != before.Version+1 {
					t.Fatalf("CAS retry changed replacement claim: state=%q token=%q route=%q delivered_at=%v version=%d",
						row.ResultState, row.DeliveryToken, row.DeliveredRoute, row.DeliveredAt, row.Version)
				}
				select {
				case <-waiter.wake:
					t.Fatal("stale release woke parent waiters")
				default:
				}
				if err := releaseDelivery(h, replacementRoute, row.DeliveryToken); err != nil {
					t.Fatalf("replacement release positive control: %v", err)
				}
				row, err = h.store.GetErrand(context.Background(), id)
				if err != nil {
					t.Fatalf("GetErrand after replacement release: %v", err)
				}
				if row.ResultState != models.ErrandResultPending || row.DeliveryToken != "" {
					t.Fatalf("replacement release left state=%q token=%q", row.ResultState, row.DeliveryToken)
				}
			})
		}
	}
}

func claimDeliveryForErrand(t *testing.T, h *harness, errandID string, route models.ErrandDeliveryRoute, token string) string {
	t.Helper()
	ctx := context.Background()
	switch route {
	case models.ErrandDeliveryAwait:
		result, err := h.service.Await(ctx, AwaitRequest{
			ParentSessionID: testParent, ErrandIDs: []string{errandID}, Mode: awaitModeAll, Token: token,
		})
		if err != nil {
			t.Fatalf("Await: %v", err)
		}
		return result.Token
	case models.ErrandDeliveryTurnContext, models.ErrandDeliveryNotification:
		composition, err := h.service.ComposeParentTurn(ctx, testParent, route)
		if err != nil {
			t.Fatalf("ComposeParentTurn: %v", err)
		}
		return composition.Token
	default:
		t.Fatalf("unsupported delivery route %q", route)
		return ""
	}
}

// @covers AC-TASKS-ERRANDS-004.11
func TestBlockedParentIsNotNotifiedUntilTheSweep(t *testing.T) {
	h, _ := settleFirstTurn(t, "the answer")
	h.deliverer.parents = nil
	h.deliverer.blocked = true
	if err := h.service.SweepDelivery(context.Background()); err != nil {
		t.Fatalf("SweepDelivery: %v", err)
	}
	if got := h.deliverer.delivered(); len(got) != 0 {
		t.Fatalf("deliveries while blocked = %v, want none", got)
	}
	h.deliverer.blocked = false
	if err := h.service.SweepDelivery(context.Background()); err != nil {
		t.Fatalf("SweepDelivery: %v", err)
	}
	if got := h.deliverer.delivered(); len(got) != 1 {
		t.Fatalf("deliveries after unblock = %v, want one", got)
	}
}

// @covers AC-TASKS-ERRANDS-006.1
func TestOutstandingParentReportsOutstandingUntilTheResultIsDelivered(t *testing.T) {
	h, _ := settleFirstTurn(t, "the answer")
	ctx := context.Background()
	outstanding, err := h.service.OnParentTurnSettled(ctx, testParent, SettleCauseTurnDone)
	if err != nil || !outstanding {
		t.Fatalf("OnParentTurnSettled = %v/%v, want outstanding while a result is pending", outstanding, err)
	}
	if _, err := h.service.ComposeParentTurn(ctx, testParent, models.ErrandDeliveryNotification); err != nil {
		t.Fatalf("ComposeParentTurn: %v", err)
	}
	outstanding, err = h.service.ParentOutstanding(ctx, testParent)
	if err != nil || outstanding {
		t.Fatalf("ParentOutstanding after delivery = %v/%v, want false", outstanding, err)
	}
}

// @covers AC-TASKS-ERRANDS-003.5
func TestDeletedOutstandingErrandAdmitsItsCancellationNotice(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand
	if err := h.service.PreserveCancellationNotice(context.Background(), errand.ID); err != nil {
		t.Fatalf("PreserveCancellationNotice: %v", err)
	}
	if len(h.deliverer.prepared) != 1 || !strings.Contains(h.deliverer.prepared[0], cancellationNotice) {
		t.Fatalf("admitted = %v, want one cancellation notice", h.deliverer.prepared)
	}
	if err := h.store.DeleteErrand(context.Background(), errand.ID); err != nil {
		t.Fatalf("DeleteErrand after notice admission: %v", err)
	}
	metadata := h.deliverer.preparedMetadata[0]
	envelope, ok := metadata[DeliveryEnvelopeMetadataKey].(DeliveryEnvelope)
	if !ok || envelope.Route != models.ErrandDeliveryNotification || len(envelope.Items) != 1 ||
		envelope.Items[0].ErrandID != errand.ID || envelope.Items[0].Status != models.ErrandStatusCancelled {
		t.Fatalf("cancellation notice snapshot = %#v, want the cancelled errand item", metadata[DeliveryEnvelopeMetadataKey])
	}
	if metadata[DeliveryTokenMetadataKey] != envelope.Token || envelope.Token == "" {
		t.Fatalf("cancellation notice token = %#v envelope=%q, want a matching immutable token", metadata[DeliveryTokenMetadataKey], envelope.Token)
	}
}

func TestDisabledErrandDeletionDoesNotQueueCancellationNotice(t *testing.T) {
	h := newHarness(t)
	err := h.mustStart("audit").Errand
	h.service.SetEnabled(func() bool { return false })

	if err := h.service.PreserveCancellationNotice(context.Background(), err.ID); err != nil {
		t.Fatalf("PreserveCancellationNotice: %v", err)
	}
	if len(h.deliverer.prepared) != 0 || len(h.deliverer.delivered()) != 0 {
		t.Fatalf("disabled deletion queued output: prepared=%v delivered=%v", h.deliverer.prepared, h.deliverer.delivered())
	}
	row, getErr := h.store.GetErrand(context.Background(), err.ID)
	if getErr != nil {
		t.Fatalf("GetErrand: %v", getErr)
	}
	if row.Lifecycle != models.ErrandLifecycleCancelled || row.ResultState != models.ErrandResultDiscarded || row.ResultText != "" {
		t.Fatalf("disabled deletion left lifecycle=%q result=%q/%q, want cancelled with no queued result", row.Lifecycle, row.ResultState, row.ResultText)
	}
}

// @covers AC-TASKS-ERRANDS-003.5
func TestRefusedCancellationNoticeFailsTheDeleteInsteadOfLosingIt(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand
	h.deliverer.admitErr = errors.New("queue full")
	if err := h.service.PreserveCancellationNotice(context.Background(), errand.ID); err == nil {
		t.Fatal("PreserveCancellationNotice succeeded although the notice was refused")
	}
}

// @covers AC-TASKS-ERRANDS-003.3
func TestSendResultCapturesAUserTurnOnlyWhenTheRuleAllows(t *testing.T) {
	h, id := settleFirstTurn(t, "captured")
	ctx := context.Background()
	if _, err := h.service.SendResult(ctx, id); err == nil {
		t.Fatal("SendResult accepted a turn whose result is already pending or delivered")
	}
	h.store.setSessionState(id, models.TaskSessionStateWaitingForInput)
	h.service.SetTurnReader(&fakeTurns{views: map[string]TurnView{
		"user-turn": {LastAgentText: "sent by the user", EarliestUserSender: "user"},
	}})
	if err := h.service.OnErrandTurnSettled(ctx, TurnSettlement{
		SessionID: id, TurnID: "user-turn", Outcome: models.ErrandOutcomeDone,
	}); err != nil {
		t.Fatalf("OnErrandTurnSettled: %v", err)
	}
	if _, err := h.service.ComposeParentTurn(ctx, testParent, models.ErrandDeliveryTurnContext); err != nil {
		t.Fatalf("ComposeParentTurn: %v", err)
	}
	snapshot, err := h.service.SendResult(ctx, id)
	if err != nil {
		t.Fatalf("SendResult after a user turn: %v", err)
	}
	if snapshot.Errand.ResultText != "sent by the user" || snapshot.Errand.ResultState != models.ErrandResultPending {
		t.Fatalf("result = %q/%s, want the user turn pending", snapshot.Errand.ResultText, snapshot.Errand.ResultState)
	}
}
