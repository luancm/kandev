package errands

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func TestReconcileDoesNotInferFailureWithoutExactTerminalEvidence(t *testing.T) {
	h, repo, row := newOwnedSettlementFixture(t, "", "")
	require.NoError(t, h.service.Reconcile(t.Context()))
	after := reconcileRow(t, repo, row.ID)
	require.Equal(t, models.ErrandResultNone, after.ResultState)
	require.True(t, after.AwaitingResult)
	require.Empty(t, after.LatestSettledTurnID)
	require.Empty(t, h.deliverer.delivered())
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
