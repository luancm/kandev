package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

// settlingErrandController records settlement calls and their outcomes.
type settlingErrandController struct {
	recordingErrandController

	causes []errands.SettleCause

	outstanding bool
	settleErr   error
}

func (c *settlingErrandController) OnParentTurnSettled(_ context.Context, _ string, cause errands.SettleCause) (bool, error) {
	c.causes = append(c.causes, cause)
	return c.outstanding, c.settleErr
}

// @covers AC-TASKS-ERRANDS-006.1
func TestSettleParentTurnReportsOutstandingAndPassesCause(t *testing.T) {
	svc := &Service{logger: testLogger()}
	controller := &settlingErrandController{outstanding: true}
	svc.SetErrandController(controller)

	outstanding, err := svc.settleParentTurn(context.Background(), "parent-1", errands.SettleCauseTurnDone)
	require.NoError(t, err)
	require.True(t, outstanding)
	outstanding, err = svc.settleParentTurn(context.Background(), "", errands.SettleCauseTurnDone)
	require.NoError(t, err)
	require.False(t, outstanding)
	require.Equal(t, []errands.SettleCause{errands.SettleCauseTurnDone}, controller.causes)
}

func TestParentSettlementReadFailureDefersWorkflow(t *testing.T) {
	svc := &Service{logger: testLogger()}
	controller := &settlingErrandController{outstanding: true, settleErr: errors.New("store down")}
	svc.SetErrandController(controller)

	outstanding, err := svc.settleParentTurn(context.Background(), "parent-1", errands.SettleCauseUserCancel)
	require.True(t, outstanding)
	require.ErrorIs(t, err, controller.settleErr, "preserve the original read error for fail-closed callers")
	require.Equal(t, []errands.SettleCause{errands.SettleCauseUserCancel}, controller.causes)
}

func TestSettlementHooksIgnoreUnwiredController(t *testing.T) {
	svc := &Service{logger: testLogger()}

	require.NotPanics(t, func() {
	})
	outstanding, err := svc.settleParentTurn(context.Background(), "parent-1", errands.SettleCauseTurnDone)
	require.NoError(t, err)
	require.False(t, outstanding)
}

// @covers AC-TASKS-ERRANDS-007.5
func TestErrandParentCancellationDefersWorkflowWithoutCascading(t *testing.T) {
	for _, silent := range []bool{false, true} {
		t.Run(map[bool]string{false: "CancelAgent", true: "CancelTurn"}[silent], func(t *testing.T) {
			f := newCancelTurnFixture(t)
			f.svc = createEngineService(t, f.repo, cancelCompletionStepGetter(true, false), f.agentMgr)
			f.svc.turnService = &repoTurnService{repo: f.repo}
			controller := &settlingErrandController{outstanding: true}
			f.svc.SetErrandController(controller)
			if silent {
				require.NoError(t, f.svc.CancelTurn(t.Context(), "s1", "turn-1"))
			} else {
				require.NoError(t, f.svc.CancelAgent(t.Context(), "s1"))
			}
			require.Equal(t, []errands.SettleCause{errands.SettleCauseUserCancel}, controller.causes)
			require.Empty(t, controller.seen(), "turn cancellation must not cascade to live errands")
			session, err := f.repo.GetTaskSession(t.Context(), "s1")
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
			task, err := f.repo.GetTask(t.Context(), "t1")
			require.NoError(t, err)
			require.Equal(t, "step1", task.WorkflowStepID)
		})
	}
}
