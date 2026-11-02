package errands

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func TestReconcileDoesNotInferFailureWithoutExactTerminalEvidence(t *testing.T) {
	for _, sealed := range []bool{false, true} {
		t.Run(fmt.Sprint("sealed=", sealed), func(t *testing.T) {
			h, repo, row, effect := newTerminalFixture(t)
			cause := ""
			if sealed {
				cause = "failed"
			}
			completeTerminalTurn(t, repo, effect, "", cause)
			require.NoError(t, repo.UpdateTaskSessionState(t.Context(), row.ID, models.TaskSessionStateFailed, ""))
			// Store wrappers must preserve the same settlement authority as the repository.
			h.service.store = struct{ Store }{repo}
			require.NoError(t, h.service.Reconcile(t.Context()))
			after, err := repo.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			if sealed {
				require.Equal(t, models.ErrandResultPending, after.ResultState)
				require.Equal(t, models.ErrandResultFailure, after.ResultKind)
				require.Equal(t, failureNotice, after.ResultText)
			} else {
				require.Equal(t, models.ErrandResultNone, after.ResultState, "session projection is not terminal evidence")
				require.True(t, after.AwaitingResult)
				require.Empty(t, after.LatestSettledTurnID)
				require.Empty(t, h.deliverer.delivered())
			}
		})
	}
}

type reconcileFailureConflictStore struct {
	Store
	attempted chan struct{}
	release   chan struct{}
	cancelled chan struct{}
	once      sync.Once
}

func (s *reconcileFailureConflictStore) UpdateErrand(ctx context.Context, row *models.Errand) error {
	err := s.Store.UpdateErrand(ctx, row)
	if err == nil && row.Lifecycle == models.ErrandLifecycleCancelled {
		select {
		case s.cancelled <- struct{}{}:
		default:
		}
	}
	return err
}

func (s *reconcileFailureConflictStore) CommitErrandTurnSettlement(ctx context.Context, row *models.Errand, effect *models.AgentDeliveryEffect) (bool, error) {
	blocked := false
	s.once.Do(func() {
		blocked = true
		s.attempted <- struct{}{}
		<-s.release
	})
	if blocked {
		return false, models.ErrErrandVersionConflict
	}
	return s.Store.CommitErrandTurnSettlement(ctx, row, effect)
}

// fakeReceipts reports which delivery tokens reached a parent message.
type fakeReceipts struct{ tokens map[string]bool }

func (f *fakeReceipts) HasDeliveryToken(_ context.Context, _, token string) (bool, error) {
	return f.tokens[token], nil
}

func reconcileRow(t *testing.T, store Store, id string) *models.Errand {
	t.Helper()
	row, err := store.GetErrand(context.Background(), id)
	if err != nil {
		t.Fatalf("GetErrand: %v", err)
	}
	return row
}

func TestReconcileCapturesRecordedFailedTurn(t *testing.T) {
	h, repo, row, effect := newTerminalFixture(t)
	completeTerminalTurn(t, repo, effect, "", "failed")
	require.NoError(t, repo.UpdateTaskSessionState(t.Context(), row.ID, models.TaskSessionStateFailed, ""))
	require.NoError(t, h.service.Reconcile(t.Context()))
	after := reconcileRow(t, repo, row.ID)
	require.Equal(t, models.ErrandOutcomeFailed, after.LastOutcome)
	require.Equal(t, models.ErrandResultFailure, after.ResultKind)
	require.Equal(t, failureNotice, after.ResultText)
	require.Equal(t, models.ErrandResultPending, after.ResultState)
	require.Equal(t, effect.TurnID, after.ResultTurnID)
}

func TestReconcileCapturesCompletedUncapturedParentTurn(t *testing.T) {
	h, repo, row, effect := newTerminalFixture(t)
	completeTerminalTurn(t, repo, effect, "finished", "done")
	require.NoError(t, repo.UpdateTaskSessionState(t.Context(), row.ID, models.TaskSessionStateWaitingForInput, ""))
	require.NoError(t, h.service.Reconcile(t.Context()))
	after := reconcileRow(t, repo, row.ID)
	require.Equal(t, models.ErrandResultPending, after.ResultState)
	require.Equal(t, "finished", after.ResultText)
	require.Equal(t, effect.TurnID, after.ResultTurnID)
	require.NoError(t, h.service.Reconcile(t.Context()))
	require.Equal(t, after, reconcileRow(t, repo, row.ID), "recovery is idempotent")
}

// @covers AC-TASKS-ERRANDS-004.8
func TestReconcileCapturesRecordedInterruptedTurn(t *testing.T) {
	h, repo, row, effect := newTerminalFixture(t)
	completeTerminalTurn(t, repo, effect, "unfinished output", "interrupted")
	require.NoError(t, repo.UpdateTaskSessionState(t.Context(), row.ID, models.TaskSessionStateWaitingForInput, ""))
	require.NoError(t, h.service.Reconcile(t.Context()))
	after := reconcileRow(t, repo, row.ID)
	require.Equal(t, models.ErrandResultInterrupted, after.ResultKind)
	require.Equal(t, interruptedTurnNotice, after.ResultText)
	require.Equal(t, models.ErrandOutcomeFailed, after.LastOutcome)
	require.False(t, after.AwaitingResult)
}

// @covers AC-TASKS-ERRANDS-003.5, AC-TASKS-ERRANDS-007.1
func TestReconcileStaleSettlementDoesNotOverwriteCancellation(t *testing.T) {
	h, repo, errand, effect := newTerminalFixture(t)
	completeTerminalTurn(t, repo, effect, "unfinished output", "interrupted")
	require.NoError(t, repo.UpdateTaskSessionState(t.Context(), errand.ID, models.TaskSessionStateWaitingForInput, ""))
	reader := &blockedTurnReader{
		reader: terminalTranscript{repo}, readStarted: make(chan struct{}, 1), readRelease: make(chan struct{}),
	}
	h.service.SetTurnReader(reader)

	reconciled := make(chan error, 1)
	go func() { reconciled <- h.service.Reconcile(t.Context()) }()
	select {
	case <-reader.readStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("reconciliation did not reach the exact transcript read barrier")
	}
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(reader.readRelease) }) })
	if _, err := h.service.Cancel(t.Context(), cancelRequest(errand, models.ErrandCancelSourceUser)); err != nil {
		t.Fatalf("cancel before stale reconciliation write: %v", err)
	}
	cancelled, err := repo.GetErrand(t.Context(), errand.ID)
	if err != nil {
		t.Fatalf("read cancelled row: %v", err)
	}
	release.Do(func() { close(reader.readRelease) })
	select {
	case err := <-reconciled:
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Reconcile did not return")
	}
	after, err := repo.GetErrand(t.Context(), errand.ID)
	if err != nil {
		t.Fatalf("read row after reconciliation: %v", err)
	}
	if !reflect.DeepEqual(after, expectedAfterTeardownReceipt(cancelled, after)) {
		t.Fatalf("stale interrupted settlement changed cancelled row:\nbefore: %+v\nafter:  %+v", cancelled, after)
	}
}

// @covers AC-TASKS-ERRANDS-003.5, AC-TASKS-ERRANDS-007.1
func TestReconcileStaleFailureRetryDoesNotOverwriteCancellation(t *testing.T) {
	h, repo, errand, effect := newTerminalFixture(t)
	completeTerminalTurn(t, repo, effect, "", "failed")
	require.NoError(t, repo.UpdateTaskSessionState(t.Context(), errand.ID, models.TaskSessionStateFailed, ""))
	store := &reconcileFailureConflictStore{
		Store: repo, attempted: make(chan struct{}, 1), release: make(chan struct{}),
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
	cancelled, err := repo.GetErrand(t.Context(), errand.ID)
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
	after, err := repo.GetErrand(t.Context(), errand.ID)
	if err != nil {
		t.Fatalf("read row after reconciliation: %v", err)
	}
	if !reflect.DeepEqual(after, expectedAfterTeardownReceipt(cancelled, after)) {
		t.Fatalf("stale launch failure changed cancelled row:\nbefore: %+v\nafter:  %+v", cancelled, after)
	}
}

func TestReconcileReturnsClaimWithoutParentMessageToPending(t *testing.T) {
	h, repo, row := settledTerminalFixture(t, "the answer")
	id := row.ID
	composition, err := h.service.ComposeParentTurn(context.Background(), testParent, models.ErrandDeliveryNotification)
	if err != nil || composition.Token == "" {
		t.Fatalf("ComposeParentTurn token %q err %v, want a claim", composition.Token, err)
	}
	h.service.SetDeliveryLog(&fakeReceipts{tokens: map[string]bool{}})

	if err := h.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if row := reconcileRow(t, repo, id); row.ResultState != models.ErrandResultPending || row.DeliveryToken != "" {
		t.Fatalf("state %q token %q, want the orphaned claim returned to pending", row.ResultState, row.DeliveryToken)
	}
}

func TestReconcileReturnsTurnContextClaimWithoutPersistedMessage(t *testing.T) {
	h, repo, row := settledTerminalFixture(t, "the answer")
	id := row.ID
	composition, err := h.service.ComposeParentTurn(context.Background(), testParent, models.ErrandDeliveryTurnContext)
	if err != nil || composition.Token == "" {
		t.Fatalf("ComposeParentTurn token %q err %v, want a claim", composition.Token, err)
	}
	h.service.SetDeliveryLog(&fakeReceipts{tokens: map[string]bool{}})

	if err := h.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if row := reconcileRow(t, repo, id); row.ResultState != models.ErrandResultPending || row.DeliveryToken != "" {
		t.Fatalf("state %q token %q, want the orphaned turn-context claim returned to pending", row.ResultState, row.DeliveryToken)
	}
}

func TestReconcileKeepsClaimWhoseTokenReachedParent(t *testing.T) {
	h, repo, row := settledTerminalFixture(t, "the answer")
	id := row.ID
	composition, err := h.service.ComposeParentTurn(context.Background(), testParent, models.ErrandDeliveryNotification)
	if err != nil || composition.Token == "" {
		t.Fatalf("ComposeParentTurn token %q err %v, want a claim", composition.Token, err)
	}
	h.service.SetDeliveryLog(&fakeReceipts{tokens: map[string]bool{composition.Token: true}})

	if err := h.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if row := reconcileRow(t, repo, id); row.ResultState != models.ErrandResultDelivered {
		t.Fatalf("state %q, want the claim delivered because its token reached the parent", row.ResultState)
	}
}

func TestReconcileDiscardsPendingResultOfTerminalParent(t *testing.T) {
	h, repo, row := settledTerminalFixture(t, "the answer")
	id := row.ID
	require.NoError(t, repo.UpdateTaskSessionState(t.Context(), testParent, models.TaskSessionStateCompleted, ""))

	if err := h.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if row := reconcileRow(t, repo, id); row.ResultState != models.ErrandResultDiscarded {
		t.Fatalf("state %q, want discarded for a terminal parent", row.ResultState)
	}
}
