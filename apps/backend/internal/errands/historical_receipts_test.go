package errands

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type enumerationFailureStore struct {
	Store
	err error
}

func (s enumerationFailureStore) PendingErrandTurnSettlements(ctx context.Context, id string) ([]*models.AgentDeliveryEffect, error) {
	pending, err := s.Store.PendingErrandTurnSettlements(ctx, id)
	if err != nil {
		return pending, err
	}
	return pending, s.err
}

func TestErrandHistoricalReceiptDoesNotPublish(t *testing.T) {
	h, repo, row, effect := newTerminalFixture(t)
	completeTerminalTurn(t, repo, effect, "original output", "failed")
	at := time.Now().UTC().Add(time.Second)
	_, err := repo.DB().Exec(`INSERT INTO task_session_turns
 (id, task_id, task_session_id, started_at, completed_at, created_at, updated_at, metadata)
 VALUES ('unknown-B', ?, ?, ?, ?, ?, ?, '{}')`, row.TaskID, row.ID, at, at, at, at)
	require.NoError(t, err)
	before, err := repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	logs := observeErrandLogs(t, h)
	count := errandMetricValue("errands_settled_total", "status=failed")
	require.ErrorIs(t, h.service.ReconcileTerminalDecisions(t.Context(), row.ID), db.ErrErrandSettlementPending)
	require.ErrorIs(t, h.service.ReconcileTerminalDecisions(t.Context(), row.ID), db.ErrErrandSettlementPending)
	var state string
	require.NoError(t, repo.DB().QueryRow(`SELECT state FROM agent_delivery_effects WHERE effect_key = ?`, effect.EffectKey).Scan(&state))
	require.Equal(t, models.DeliveryEffectCompleted, state)
	after, err := repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Empty(t, h.publisher.statuses())
	require.Empty(t, h.deliverer.delivered())
	require.Zero(t, logs.FilterMessage("errand.settled").Len())
	require.Equal(t, count, errandMetricValue("errands_settled_total", "status=failed"))
}

func TestErrandTerminalEnumerationStorageErrorRetainsFence(t *testing.T) {
	h, repo, row, effect := newTerminalFixture(t)
	completeTerminalTurn(t, repo, effect, "output", "failed")
	failure := errors.New("storage unavailable")
	h.service.store = enumerationFailureStore{Store: repo, err: failure}
	require.ErrorIs(t, h.service.ReconcileTerminalDecisions(t.Context(), row.ID), failure)
	var state string
	require.NoError(t, repo.DB().QueryRow(`SELECT state FROM agent_delivery_effects WHERE effect_key = ?`, effect.EffectKey).Scan(&state))
	require.Equal(t, models.DeliveryEffectPending, state)
	require.Empty(t, h.publisher.statuses())
	h.service.store = repo
	require.NoError(t, h.service.ReconcileTerminalDecisions(t.Context(), row.ID))
	require.NotEmpty(t, h.publisher.statuses())
}
