package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

// settlingErrandController adds the settlement methods to the recording
// controller so the type assertion in errandSettler succeeds.
type settlingErrandController struct {
	recordingErrandController
	settlements   []errands.TurnSettlement
	causes        []errands.SettleCause
	cancellations []cancelledErrandTurn
	outstanding   bool
	settleErr     error
}

type cancelledErrandTurn struct {
	sessionID    string
	turnID       string
	submissionID string
}

func (c *settlingErrandController) OnErrandTurnSettled(_ context.Context, settlement errands.TurnSettlement) error {
	c.settlements = append(c.settlements, settlement)
	return c.settleErr
}

func (c *settlingErrandController) OnParentTurnSettled(_ context.Context, _ string, cause errands.SettleCause) (bool, error) {
	c.causes = append(c.causes, cause)
	return c.outstanding, c.settleErr
}

func (c *settlingErrandController) OnErrandTurnCancelled(_ context.Context, sessionID, turnID, submissionID string) error {
	c.cancellations = append(c.cancellations, cancelledErrandTurn{
		sessionID: sessionID, turnID: turnID, submissionID: submissionID,
	})
	return c.settleErr
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

func TestCancelAgentSettlesOnlyTheObservedErrandTurn(t *testing.T) {
	f := newErrandCancellationFixture(t)
	controller := &settlingErrandController{}
	f.svc.SetErrandController(controller)

	require.NoError(t, f.svc.CancelAgent(context.Background(), "s1"))
	require.Equal(t, []cancelledErrandTurn{{sessionID: "s1", turnID: "turn-1"}}, controller.cancellations)
	require.Empty(t, controller.causes, "an errand turn cancellation must not settle a parent turn")
}

// @covers AC-TASKS-ERRANDS-002.1 AC-TASKS-ERRANDS-002.2
func TestCancelTurnSettlesOnlyTheObservedErrandTurn(t *testing.T) {
	f := newErrandCancellationFixture(t)
	controller := &settlingErrandController{}
	f.svc.SetErrandController(controller)

	require.NoError(t, f.svc.CancelTurn(t.Context(), "s1", "turn-1"))
	require.Equal(t, []cancelledErrandTurn{{sessionID: "s1", turnID: "turn-1"}}, controller.cancellations)
	require.Empty(t, controller.causes, "an errand turn must not settle a parent turn")
}

func newErrandCancellationFixture(t *testing.T) *cancelTurnFixture {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "parent-1", "step1")
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{
		ID: "s1", TaskID: "t1", Role: models.SessionRoleErrand, ParentSessionID: "parent-1",
		State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTurn(t.Context(), &models.Turn{
		ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: now,
	}))
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	agentMgr.currentPromptExecutionID = "exec-1"
	agentMgr.currentPromptGeneration.Store(3)
	agentMgr.currentPromptActivityEpoch.Store(7)
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.turnService = &repoTurnService{repo: repo}
	return &cancelTurnFixture{svc: svc, repo: repo, agentMgr: agentMgr}
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
			require.Empty(t, controller.cancellations)
			session, err := f.repo.GetTaskSession(t.Context(), "s1")
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
			task, err := f.repo.GetTask(t.Context(), "t1")
			require.NoError(t, err)
			require.Equal(t, "step1", task.WorkflowStepID)
		})
	}
}
