package errands

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type attentionStore struct {
	Store
	unresolved bool
	readError  error
}

func (s attentionStore) ReadErrandStatuses(ctx context.Context, taskID, id string) (map[string]models.ErrandBrowserObservation, error) {
	if s.readError != nil {
		return nil, s.readError
	}
	statuses, err := s.Store.ReadErrandStatuses(ctx, taskID, id)
	if err != nil || !s.unresolved {
		return statuses, err
	}
	for key, status := range statuses {
		status.Status = models.ErrandStatusNeedsAttention
		status.AttentionReason = models.ErrandAttentionRecoveryUnresolved
		statuses[key] = status
	}
	return statuses, nil
}

// AC-TASKS-ERRANDS-002.4: both service paths consume the repository observation.
func TestRecoveryAttentionProjection(t *testing.T) {
	h := newHarness(t)
	row := h.mustStart("recovered work").Errand
	h.store.setErrandWorking(row.ID, false)
	h.service.store = attentionStore{Store: h.store, unresolved: true}
	status, err := h.service.Project(t.Context(), row)
	require.NoError(t, err)
	require.Equal(t, models.ErrandStatusNeedsAttention, status)
	snapshots, err := h.service.List(t.Context(), testTask)
	require.NoError(t, err)
	require.Equal(t, status, snapshots[0].Status)
	require.Equal(t, models.ErrandAttentionRecoveryUnresolved, snapshots[0].AttentionReason)
	items, err := h.service.awaitItems(t.Context(), "", []*models.Errand{row}, true, false)
	require.NoError(t, err)
	require.Equal(t, string(models.ErrandStatusNeedsAttention), items[0].Status)
	require.Equal(t, models.ErrandAttentionRecoveryUnresolved, items[0].AttentionReason)
	h.service.store = attentionStore{Store: h.store, readError: errors.New("evidence unavailable")}
	_, err = h.service.Project(t.Context(), row)
	require.ErrorContains(t, err, "evidence unavailable")
	_, err = h.service.List(t.Context(), testTask)
	require.ErrorContains(t, err, "evidence unavailable")
}

func TestRecoveryAttentionReasonInParentBlock(t *testing.T) {
	h := newHarness(t)
	row := h.mustStart("uncertain work").Errand
	h.store.setErrandWorking(row.ID, false)
	h.service.store = attentionStore{Store: h.store, unresolved: true}
	composed, err := h.service.ComposeParentTurn(t.Context(), testParent, models.ErrandDeliveryTurnContext)
	require.NoError(t, err)
	require.Contains(t, composed.Block, "recovery_unresolved")
}

func TestRecoveryAttentionOutstandingWithoutAwaitingFlag(t *testing.T) {
	h := newHarness(t)
	row := h.mustStart("uncertain user follow-up").Errand
	h.store.setErrandWorking(row.ID, false)
	_, _, err := h.service.update(t.Context(), row.ID, func(e *models.Errand) bool {
		e.AwaitingResult = false
		e.LastOutcome = models.ErrandOutcomeDone
		return true
	})
	require.NoError(t, err)
	h.service.store = attentionStore{Store: h.store, unresolved: true}
	outstanding, err := h.service.ParentOutstanding(t.Context(), testParent)
	require.NoError(t, err)
	require.True(t, outstanding)
}
