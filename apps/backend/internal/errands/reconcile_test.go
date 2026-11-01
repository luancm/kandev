package errands

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// reconcileTurns reports a fixed latest turn for startup reconciliation.
type reconcileTurns struct {
	fakeTurns
	turnID        string
	view          TurnView
	latestStarted chan struct{}
	latestRelease chan struct{}
}

func (r *reconcileTurns) LatestTurn(context.Context, string) (string, TurnView, error) {
	if r.latestStarted != nil {
		r.latestStarted <- struct{}{}
		<-r.latestRelease
	}
	return r.turnID, r.view, nil
}

type reconcileFailureConflictStore struct {
	Store
	attempted chan struct{}
	release   chan struct{}
	cancelled chan struct{}
	once      sync.Once
}

func (s *reconcileFailureConflictStore) UpdateErrand(ctx context.Context, row *models.Errand) error {
	if row.Lifecycle == models.ErrandLifecycleCancelled {
		err := s.Store.UpdateErrand(ctx, row)
		if err == nil {
			select {
			case s.cancelled <- struct{}{}:
			default:
			}
		}
		return err
	}
	if row.ResultKind == models.ErrandResultFailure && row.ResultText == failureNotice {
		blocked := false
		s.once.Do(func() {
			blocked = true
			s.attempted <- struct{}{}
			<-s.release
		})
		if blocked {
			return models.ErrErrandVersionConflict
		}
	}
	return s.Store.UpdateErrand(ctx, row)
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

// @covers AC-TASKS-ERRANDS-003.5, AC-TASKS-ERRANDS-007.1
func TestReconcileStaleSettlementDoesNotOverwriteCancellation(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand
	h.store.setSessionState(errand.ID, models.TaskSessionStateWaitingForInput)
	reader := &reconcileTurns{
		fakeTurns:     fakeTurns{views: map[string]TurnView{}},
		turnID:        errand.FirstTurnID,
		view:          TurnView{EarliestUserSender: testParent},
		latestStarted: make(chan struct{}, 1),
		latestRelease: make(chan struct{}),
	}
	h.service.SetTurnReader(reader)

	reconciled := make(chan error, 1)
	go func() { reconciled <- h.service.Reconcile(t.Context()) }()
	select {
	case <-reader.latestStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("reconciliation did not reach the latest-turn barrier")
	}
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(reader.latestRelease) }) })
	if _, err := h.service.Cancel(t.Context(), cancelRequest(errand, models.ErrandCancelSourceUser)); err != nil {
		t.Fatalf("cancel before stale reconciliation write: %v", err)
	}
	cancelled, err := h.store.GetErrand(t.Context(), errand.ID)
	if err != nil {
		t.Fatalf("read cancelled row: %v", err)
	}
	release.Do(func() { close(reader.latestRelease) })
	select {
	case err := <-reconciled:
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Reconcile did not return")
	}
	after, err := h.store.GetErrand(t.Context(), errand.ID)
	if err != nil {
		t.Fatalf("read row after reconciliation: %v", err)
	}
	if !reflect.DeepEqual(after, expectedAfterTeardownReceipt(cancelled, after)) {
		t.Fatalf("stale interrupted settlement changed cancelled row:\nbefore: %+v\nafter:  %+v", cancelled, after)
	}
}

// @covers AC-TASKS-ERRANDS-003.5, AC-TASKS-ERRANDS-007.1
func TestReconcileStaleLaunchFailureDoesNotOverwriteCancellation(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand
	h.store.setSessionState(errand.ID, models.TaskSessionStateFailed)
	store := &reconcileFailureConflictStore{
		Store: h.store, attempted: make(chan struct{}, 1), release: make(chan struct{}),
		cancelled: make(chan struct{}, 1),
	}
	h.service.store = store
	reconciled := make(chan error, 1)
	go func() { reconciled <- h.service.Reconcile(t.Context()) }()
	select {
	case <-store.attempted:
	case <-time.After(10 * time.Second):
		t.Fatal("reconciliation did not reach the failure write barrier")
	}
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(store.release) }) })
	cancelledBy := make(chan error, 1)
	go func() { cancelledBy <- cancelForSettlementRace(t.Context(), h, errand, "user") }()
	select {
	case <-store.cancelled:
	case <-time.After(10 * time.Second):
		t.Fatal("cancellation did not commit before stale failure retry")
	}
	cancelled, err := h.store.GetErrand(t.Context(), errand.ID)
	if err != nil {
		t.Fatalf("read cancelled row: %v", err)
	}
	release.Do(func() { close(store.release) })
	select {
	case err := <-reconciled:
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Reconcile did not return")
	}
	if err := <-cancelledBy; err != nil {
		t.Fatalf("cancellation: %v", err)
	}
	after, err := h.store.GetErrand(t.Context(), errand.ID)
	if err != nil {
		t.Fatalf("read row after reconciliation: %v", err)
	}
	if !reflect.DeepEqual(after, expectedAfterTeardownReceipt(cancelled, after)) {
		t.Fatalf("stale launch failure changed cancelled row:\nbefore: %+v\nafter:  %+v", cancelled, after)
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

func TestReconcileReturnsTurnContextClaimWithoutPersistedMessage(t *testing.T) {
	h, id := settleFirstTurn(t, "the answer")
	composition, err := h.service.ComposeParentTurn(context.Background(), testParent, models.ErrandDeliveryTurnContext)
	if err != nil || composition.Token == "" {
		t.Fatalf("ComposeParentTurn token %q err %v, want a claim", composition.Token, err)
	}
	h.service.SetDeliveryLog(&fakeReceipts{tokens: map[string]bool{}})

	if err := h.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if row := reconcileRow(t, h, id); row.ResultState != models.ErrandResultPending || row.DeliveryToken != "" {
		t.Fatalf("state %q token %q, want the orphaned turn-context claim returned to pending", row.ResultState, row.DeliveryToken)
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
