package errands

import (
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestErrandCompletionCleanupRecoversIntentFromSealedTerminal(t *testing.T) {
	h, repo, row, effect := newTerminalFixture(t)
	completeTerminalTurn(t, repo, effect, "result saved after restart", "done")
	require.NoError(t, h.service.ReconcileTerminalDecisions(t.Context(), row.ID))
	recovered, err := repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, "result saved after restart", recovered.ResultText)
	require.NotNil(t, recovered.TeardownIntent)
	require.Equal(t, models.ErrandTeardownPurposeCompletion, recovered.TeardownIntent.Purpose)
	require.Equal(t, effect.StreamID, recovered.TeardownIntent.Target.ExecutionID)
	require.Equal(t, effect.TurnID, recovered.TeardownIntent.Target.TurnID)
	require.Equal(t, effect.SubmissionID, recovered.TeardownIntent.Target.SubmissionID)
}
