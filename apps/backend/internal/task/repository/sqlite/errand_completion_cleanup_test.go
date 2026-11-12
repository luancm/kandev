package sqlite

import (
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestErrandCompletionCleanupPreservesPriorIntentAcrossLaterFailure(t *testing.T) {
	prior := &models.ErrandTeardownIntent{ID: "prior", Purpose: models.ErrandTeardownPurposeCompletion, State: models.ErrandTeardownIntentCompleted, Target: models.ErrandTeardownTarget{TurnID: "successful-parent"}}
	current := &models.Errand{TeardownIntent: prior}
	next := *current
	decision := &models.AgentDeliveryEffect{TurnID: "failed-user-followup", State: models.DeliveryEffectCompleted, Outcome: "user:failed"}
	require.NoError(t, validateErrandCompletionIntentWrite(current, &next, decision))
	changed := *prior
	changed.ID = "unproven-replacement"
	next.TeardownIntent = &changed
	require.Error(t, validateErrandCompletionIntentWrite(current, &next, decision))
}
