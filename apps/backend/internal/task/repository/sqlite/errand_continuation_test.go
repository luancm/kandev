package sqlite

import (
	"context"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// @covers AC-TASKS-ERRANDS-002.7 AC-TASKS-ERRANDS-002.8
func TestErrandContinuationPreflightExactReceipts(t *testing.T) {
	repo := newRepoForSessionTests(t)
	guard, ok := any(repo).(interface {
		ValidateErrandContinuation(context.Context, string) error
	})
	require.True(t, ok)
	row, effect := seedExactSettlement(t, repo)
	require.NoError(t, repo.ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
	require.NoError(t, repo.CompleteTurn(t.Context(), effect.TurnID))
	require.ErrorIs(t, guard.ValidateErrandContinuation(t.Context(), row.ID), db.ErrErrandSettlementPending)
	effect.Outcome = "parent:interrupted"
	require.NoError(t, repo.SealErrandTurnSettlement(t.Context(), row.TaskID, effect))
	at := time.Now().UTC()
	effect.State, effect.CompletedAt = models.DeliveryEffectCompleted, &at
	require.NoError(t, repo.CommitHistoricalErrandTurnReceipt(t.Context(), row.TaskID, effect))
	require.NoError(t, guard.ValidateErrandContinuation(t.Context(), row.ID))
	_, err := repo.DB().Exec(`UPDATE agent_delivery_effects SET completed_at=NULL WHERE effect_key=?`, effect.EffectKey)
	require.NoError(t, err)
	require.ErrorIs(t, guard.ValidateErrandContinuation(t.Context(), row.ID), db.ErrErrandSettlementPending)
}
