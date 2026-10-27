package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

// settlingErrandController adds the settlement methods to the recording
// controller so the type assertion in errandSettler succeeds.
type settlingErrandController struct {
	recordingErrandController
	settlements []errands.TurnSettlement
	causes      []errands.SettleCause
	outstanding bool
	settleErr   error
}

func (c *settlingErrandController) OnErrandTurnSettled(_ context.Context, settlement errands.TurnSettlement) error {
	c.settlements = append(c.settlements, settlement)
	return c.settleErr
}

func (c *settlingErrandController) OnParentTurnSettled(_ context.Context, _ string, cause errands.SettleCause) (bool, error) {
	c.causes = append(c.causes, cause)
	return c.outstanding, c.settleErr
}

// @covers AC-TASKS-ERRANDS-003.1
func TestSettleErrandTurnForwardsOutcomeAndIgnoresMissingIdentity(t *testing.T) {
	svc := &Service{logger: testLogger()}
	controller := &settlingErrandController{}
	svc.SetErrandController(controller)

	svc.settleErrandTurn(context.Background(), "errand-1", "turn-1", models.ErrandOutcomeFailed, "boom")
	svc.settleErrandTurn(context.Background(), "", "turn-1", models.ErrandOutcomeDone, "")
	svc.settleErrandTurn(context.Background(), "errand-1", "", models.ErrandOutcomeDone, "")

	require.Equal(t, []errands.TurnSettlement{{
		SessionID: "errand-1", TurnID: "turn-1", Outcome: models.ErrandOutcomeFailed, FailureSummary: "boom",
	}}, controller.settlements)
}

// @covers AC-TASKS-ERRANDS-006.1
func TestSettleParentTurnReportsOutstandingAndPassesCause(t *testing.T) {
	svc := &Service{logger: testLogger()}
	controller := &settlingErrandController{outstanding: true}
	svc.SetErrandController(controller)

	require.True(t, svc.settleParentTurn(context.Background(), "parent-1", errands.SettleCauseTurnDone))
	require.False(t, svc.settleParentTurn(context.Background(), "", errands.SettleCauseTurnDone))
	require.Equal(t, []errands.SettleCause{errands.SettleCauseTurnDone}, controller.causes)
}

func TestSettleParentTurnFailureIsNotOutstanding(t *testing.T) {
	svc := &Service{logger: testLogger()}
	controller := &settlingErrandController{outstanding: true, settleErr: errors.New("store down")}
	svc.SetErrandController(controller)

	require.False(t, svc.settleParentTurn(context.Background(), "parent-1", errands.SettleCauseUserCancel))
	require.Equal(t, []errands.SettleCause{errands.SettleCauseUserCancel}, controller.causes)
}

func TestSettlementHooksIgnoreControllerWithoutSettlementMethods(t *testing.T) {
	svc := &Service{logger: testLogger()}
	svc.SetErrandController(&recordingErrandController{})

	require.NotPanics(t, func() {
		svc.settleErrandTurn(context.Background(), "errand-1", "turn-1", models.ErrandOutcomeDone, "")
	})
	require.False(t, svc.settleParentTurn(context.Background(), "parent-1", errands.SettleCauseTurnDone))

	svc.SetErrandController(nil)
	require.False(t, svc.settleParentTurn(context.Background(), "parent-1", errands.SettleCauseTurnDone))
}
