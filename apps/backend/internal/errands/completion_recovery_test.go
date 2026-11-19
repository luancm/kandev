package errands

import (
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestErrandCompletionCleanupRecoversIntentFromSealedTerminal(t *testing.T) {
	h, repo, row := newOwnedSettlementFixture(t, models.ErrandOutcomeDone, "result saved after restart")
	require.NoError(t, h.service.reconcileOwnedErrandResults(t.Context()))
	recovered, err := repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, "result saved after restart", recovered.ResultText)
	require.NotNil(t, recovered.TeardownIntent)
	require.Equal(t, models.ErrandTeardownPurposeCompletion, recovered.TeardownIntent.Purpose)
	require.Equal(t, row.ID+"-execution", recovered.TeardownIntent.Target.ExecutionID)
	require.Equal(t, row.FirstTurnID, recovered.TeardownIntent.Target.TurnID)
	require.Equal(t, row.SubmissionID, recovered.TeardownIntent.Target.SubmissionID)
}
