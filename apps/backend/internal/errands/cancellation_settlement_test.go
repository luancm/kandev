package errands

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-ERRANDS-007.5
func TestParentTurnCancellationLeavesResultsAndLiveErrandsForLaterTurn(t *testing.T) {
	h, settledID := settleFirstTurn(t, "the answer")
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

// @covers AC-TASKS-ERRANDS-002.1
func TestCancelledErrandTurnPreservesCompletionThatWonTheRace(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand
	view := TurnView{Completed: true, EarliestUserSender: testParent, LastAgentText: "finished first"}
	h.service.SetTurnReader(&cancellationTurns{
		views: map[string]TurnView{errand.FirstTurnID: view}, latestID: errand.FirstTurnID, latest: view,
	})

	require.NoError(t, h.service.OnErrandTurnCancelled(t.Context(), errand.ID, errand.FirstTurnID, errand.SubmissionID))
	row, err := h.store.GetErrand(t.Context(), errand.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandOutcomeDone, row.LastOutcome)
	require.Equal(t, models.ErrandResultOutput, row.ResultKind)
	require.Equal(t, "finished first", row.ResultText)
	require.False(t, row.AwaitingResult)
}
