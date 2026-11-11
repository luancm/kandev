package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/db"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type recoveryQueueDrainer struct{ mockQueueDrainer }

func (*recoveryQueueDrainer) ValidateErrandContinuation(context.Context, string) error {
	return db.ErrErrandSettlementPending
}

// @covers AC-TASKS-ERRANDS-002.8
func TestQueueErrandContinuationRefusal(t *testing.T) {
	h, queue := setupQueueHandlersWithoutStart(t, &recoveryQueueDrainer{})
	for range 2 {
		req, err := ws.NewRequest("refused", ws.ActionMessageQueueAdd, map[string]interface{}{"task_id": "task", "session_id": "child", "content": "new work"})
		require.NoError(t, err)
		response, err := h.wsQueueMessage(t.Context(), req)
		require.NoError(t, err)
		requireRecoveryQueueReason(t, response)
	}
	require.Zero(t, queue.GetStatus(t.Context(), "child").Count)
	req, err := ws.NewRequest("send-now", ws.ActionMessageQueueSendNow, nil)
	require.NoError(t, err)
	response, err := h.sendNowErrorResponse(req, "child", db.ErrErrandSettlementPending)
	require.NoError(t, err)
	requireRecoveryQueueReason(t, response)
}
func requireRecoveryQueueReason(t *testing.T, response *ws.Message) {
	t.Helper()
	var payload struct {
		Details map[string]interface{} `json:"details"`
	}
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.Equal(t, "errand_recovery_unresolved", payload.Details["reason"])
}

func TestQueueErrandDispatchRefusal(t *testing.T) {
	h, _ := setupQueueHandlersWithoutStart(t, &mockQueueDrainer{err: db.ErrErrandSettlementPending, autoRunErr: db.ErrErrandSettlementPending})
	for _, action := range []string{ws.ActionMessageQueueDrain, ws.ActionMessageQueueAutoRunSet} {
		req, err := ws.NewRequest("dispatch", action, map[string]interface{}{"task_id": "task", "session_id": "child", "enabled": true})
		require.NoError(t, err)
		var response *ws.Message
		if action == ws.ActionMessageQueueDrain {
			response, err = h.wsDrainQueue(t.Context(), req)
		} else {
			response, err = h.wsSetAutoRun(t.Context(), req)
		}
		require.NoError(t, err)
		requireRecoveryQueueReason(t, response)
	}
}
