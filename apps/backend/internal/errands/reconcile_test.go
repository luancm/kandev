package errands

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// reconcileTurns reports a fixed latest turn for startup reconciliation.
type reconcileTurns struct {
	fakeTurns
	turnID string
	view   TurnView
}

func (r *reconcileTurns) LatestTurn(context.Context, string) (string, TurnView, error) {
	return r.turnID, r.view, nil
}

// fakeReceipts reports which delivery tokens reached a parent message.
type fakeReceipts struct{ tokens map[string]bool }

func (f *fakeReceipts) HasDeliveryToken(_ context.Context, _, token string) (bool, error) {
	return f.tokens[token], nil
}

func reconcileRow(t *testing.T, h *harness, id string) *models.Errand {
	t.Helper()
	row, err := h.store.GetErrand(context.Background(), id)
	if err != nil {
		t.Fatalf("GetErrand: %v", err)
	}
	return row
}

func TestReconcileFailsLaunchedRowWhoseSessionFailed(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand
	h.store.setSessionState(errand.ID, models.TaskSessionStateFailed)

	if err := h.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	row := reconcileRow(t, h, errand.ID)
	if row.LastOutcome != models.ErrandOutcomeFailed || row.ResultKind != models.ErrandResultFailure ||
		row.ResultState != models.ErrandResultPending {
		t.Fatalf("row = outcome %q kind %q state %q, want a pending failure", row.LastOutcome, row.ResultKind, row.ResultState)
	}
}

func TestReconcileCapturesCompletedUncapturedParentTurn(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand
	h.store.setSessionState(errand.ID, models.TaskSessionStateWaitingForInput)
	h.service.SetTurnReader(&reconcileTurns{
		fakeTurns: fakeTurns{views: map[string]TurnView{
			"turn-9": {LastAgentText: "finished", EarliestUserSender: testParent, Completed: true},
		}},
		turnID: "turn-9",
		view:   TurnView{LastAgentText: "finished", EarliestUserSender: testParent, Completed: true},
	})

	if err := h.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	row := reconcileRow(t, h, errand.ID)
	if row.ResultState != models.ErrandResultPending || row.ResultText != "finished" || row.ResultTurnID != "turn-9" {
		t.Fatalf("result = %q/%s turn %q, want the completed turn captured as pending", row.ResultText, row.ResultState, row.ResultTurnID)
	}
}

// @covers AC-TASKS-ERRANDS-004.8
func TestReconcileInterruptsAbandonedParentTurn(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand
	h.store.setSessionState(errand.ID, models.TaskSessionStateWaitingForInput)
	h.service.SetTurnReader(&reconcileTurns{
		fakeTurns: fakeTurns{views: map[string]TurnView{}},
		turnID:    errand.FirstTurnID,
		view:      TurnView{EarliestUserSender: testParent, Completed: false},
	})

	if err := h.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	row := reconcileRow(t, h, errand.ID)
	if row.ResultKind != models.ErrandResultInterrupted || row.ResultText != interruptedNotice ||
		row.LastOutcome != models.ErrandOutcomeFailed || row.AwaitingResult {
		t.Fatalf("row = kind %q outcome %q awaiting %v, want an interrupted failure", row.ResultKind, row.LastOutcome, row.AwaitingResult)
	}
}

func TestReconcileReturnsClaimWithoutParentMessageToPending(t *testing.T) {
	h, id := settleFirstTurn(t, "the answer")
	composition, err := h.service.ComposeParentTurn(context.Background(), testParent, models.ErrandDeliveryNotification)
	if err != nil || composition.Token == "" {
		t.Fatalf("ComposeParentTurn token %q err %v, want a claim", composition.Token, err)
	}
	h.service.SetDeliveryLog(&fakeReceipts{tokens: map[string]bool{}})

	if err := h.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if row := reconcileRow(t, h, id); row.ResultState != models.ErrandResultPending || row.DeliveryToken != "" {
		t.Fatalf("state %q token %q, want the orphaned claim returned to pending", row.ResultState, row.DeliveryToken)
	}
}

func TestReconcileLeavesTurnContextClaimDelivered(t *testing.T) {
	h, id := settleFirstTurn(t, "the answer")
	composition, err := h.service.ComposeParentTurn(context.Background(), testParent, models.ErrandDeliveryTurnContext)
	if err != nil || composition.Token == "" {
		t.Fatalf("ComposeParentTurn token %q err %v, want a claim", composition.Token, err)
	}
	h.service.SetDeliveryLog(&fakeReceipts{tokens: map[string]bool{}})

	if err := h.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if row := reconcileRow(t, h, id); row.ResultState != models.ErrandResultDelivered {
		t.Fatalf("state %q, want a turn-context claim left delivered: its message may predate composition", row.ResultState)
	}
}

func TestReconcileKeepsClaimWhoseTokenReachedParent(t *testing.T) {
	h, id := settleFirstTurn(t, "the answer")
	composition, err := h.service.ComposeParentTurn(context.Background(), testParent, models.ErrandDeliveryNotification)
	if err != nil || composition.Token == "" {
		t.Fatalf("ComposeParentTurn token %q err %v, want a claim", composition.Token, err)
	}
	h.service.SetDeliveryLog(&fakeReceipts{tokens: map[string]bool{composition.Token: true}})

	if err := h.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if row := reconcileRow(t, h, id); row.ResultState != models.ErrandResultDelivered {
		t.Fatalf("state %q, want the claim delivered because its token reached the parent", row.ResultState)
	}
}

func TestReconcileDiscardsPendingResultOfTerminalParent(t *testing.T) {
	h, id := settleFirstTurn(t, "the answer")
	h.store.setSessionState(testParent, models.TaskSessionStateCompleted)

	if err := h.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if row := reconcileRow(t, h, id); row.ResultState != models.ErrandResultDiscarded {
		t.Fatalf("state %q, want discarded for a terminal parent", row.ResultState)
	}
}
