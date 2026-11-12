package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-003.1 AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
func TestErrandCancellationSettlementDoesNotReenterCompletionGuard(t *testing.T) {
	f := seedOrchestratorSettledSessionState(t, "errand-cancel-guard", false)
	controller := &settlingErrandController{}
	f.service.SetErrandController(controller)
	guard := f.service.lockCancelInFlightGuard(f.sessionID)
	defer guard.release()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	f.service.settleCancelledErrandTurn(ctx, f.sessionID, f.turnID, f.submission)
	require.NoError(t, ctx.Err())
	require.Equal(t, []cancelledErrandTurn{{sessionID: f.sessionID, turnID: f.turnID, submissionID: f.submission}}, controller.cancellations)
	terminal, err := f.repo.GetAgentDeliveryEffect(t.Context(), "agent_delivery.terminal:"+f.streamID+":1")
	require.NoError(t, err)
	require.Equal(t, models.DeliveryEffectCompleted, terminal.State)
	requireDeliverySessionState(t, f.repo, f.sessionID, models.TaskSessionStateRunning)
}
