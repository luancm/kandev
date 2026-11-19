package errands

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-ERRANDS-007.5
func TestParentTurnCancellationLeavesResultsAndLiveErrandsForLaterTurn(t *testing.T) {
	h, settledID := pendingResultHarness(t, "the answer")
	live := h.mustStart("still running").Errand
	before := len(h.deliverer.delivered())

	outstanding, err := h.service.OnParentTurnSettled(t.Context(), testParent, SettleCauseUserCancel)
	require.NoError(t, err)
	require.True(t, outstanding)
	require.Len(t, h.deliverer.delivered(), before)
	row, err := h.store.GetErrand(t.Context(), settledID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultPending, row.ResultState)
	row, err = h.store.GetErrand(t.Context(), live.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleLaunched, row.Lifecycle)
	require.True(t, row.AwaitingResult)

	_, err = h.service.OnParentTurnSettled(t.Context(), testParent, SettleCauseTurnDone)
	require.NoError(t, err)
	require.Len(t, h.deliverer.delivered(), before+1, "a later completed turn can request notification")
}
