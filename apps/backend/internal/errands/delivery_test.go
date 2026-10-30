package errands

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
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
	if err := h.service.OnParentPromptAdmitted(context.Background(), id, testParent); err != nil {
		t.Fatalf("OnParentPromptAdmitted: %v", err)
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
