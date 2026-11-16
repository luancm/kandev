package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

type refusedAdmissionWriteFailure struct{ *errands.Service }

func (s *refusedAdmissionWriteFailure) DecideParentAdmission(ctx context.Context, admission models.ErrandParentAdmission, state models.ErrandAdmissionState) error {
	if state == models.ErrandAdmissionRefused {
		return errors.New("parent refusal write unavailable")
	}
	return s.Service.DecideParentAdmission(ctx, admission, state)
}

// @covers AC-PLATFORM-UNDELIVERED-PROMPT-001.1 AC-TASKS-ERRANDS-002.7
func TestErrandParentUnsentRefusalWriteFailureRetainsTurn(t *testing.T) {
	f := newAdmissionIntegrationFixture(t, messagequeue.DeliveryProtocolV1)
	removed, err := f.repo.DeleteTurnIfUnreferenced(t.Context(), f.session.ID, "admission-turn")
	require.NoError(t, err)
	require.True(t, removed)
	f.svc.SetErrandController(&refusedAdmissionWriteFailure{Service: f.errands})
	f.manager.promptErr = (&agentctl.Client{}).PromptWithSubmissionID(t.Context(), "unsent", nil, 1, "peer:refusal-write")
	ctx := errands.WithParentPromptOrigin(t.Context(), errands.ParentPromptOrigin{
		SenderSessionID: f.parent, SubmissionID: "peer:refusal-write",
	})
	_, dispatchErr := f.svc.PromptTask(ctx, f.taskID, f.session.ID, "unsent instruction", "", false, nil, true)
	require.ErrorContains(t, dispatchErr, "parent refusal write unavailable")
	require.ErrorIs(t, dispatchErr, agentruntime.ErrUncertainPromptDelivery)
	var uncertain interface{ ErrandParentPromptUncertain() bool }
	require.ErrorAs(t, dispatchErr, &uncertain)
	require.True(t, uncertain.ErrandParentPromptUncertain())
	submission, err := f.repo.GetAgentDeliverySubmissionMetadata(ctx, "prompt:peer:refusal-write")
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionFailed, submission.State)
	require.Equal(t, "failed", submission.Outcome)
	require.NotNil(t, submission.FullySettledAt, "non-delivery committed before the separate bookkeeping error")
	require.Empty(t, submission.TerminalTurnID)
	pending, err := f.repo.ListUnresolvedErrandParentAdmissions(ctx, f.row.ID)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	turn, err := f.repo.GetTurn(ctx, pending[0].TurnID)
	require.NoError(t, err, "committed non-delivery alone does not permit reservation cleanup")
	require.Nil(t, turn.CompletedAt)
	require.Equal(t, true, turn.Metadata[models.TurnMetaKeyPromptDispatchPending])
	decision, err := f.repo.GetErrandTurnSettlementDecision(ctx, f.session.ID, submission.IncarnationID, turn.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliveryEffectPending, decision.State)
	row, err := f.repo.GetErrand(ctx, f.row.ID)
	require.NoError(t, err)
	require.Zero(t, row.ParentAdmissionGeneration)
	require.Equal(t, f.row.ResultRevision, row.ResultRevision)
	require.Equal(t, models.ErrandResultPending, row.ResultState)
	require.Equal(t, []string{"unsent instruction"}, f.manager.capturedPrompts)
}

func TestErrandInitialPreDispatchRefusalWriteFailureRetainsUnknownAdmission(t *testing.T) {
	f := newAdmissionIntegrationFixture(t, messagequeue.DeliveryProtocolV1)
	f.svc.SetErrandController(&refusedAdmissionWriteFailure{Service: f.errands})
	_, err := f.repo.DB().Exec(`CREATE TRIGGER reject_initial_submission BEFORE INSERT ON agent_delivery_submissions
		BEGIN SELECT RAISE(ABORT, 'submission preparation unavailable'); END`)
	require.NoError(t, err)
	ctx := errands.WithParentPromptOrigin(t.Context(), errands.ParentPromptOrigin{SenderSessionID: f.parent, SubmissionID: "peer:initial-bookkeeping"})
	require.NoError(t, f.repo.UpdateTaskSessionState(ctx, f.session.ID, models.TaskSessionStateCreated, ""))
	manager := &retentionInitialPromptManager{durableDeliveryTestAgentManager: f.manager.durableDeliveryTestAgentManager}
	manager.isAgentRunning = false
	started := make(chan struct{}, 1)
	manager.startAgentProcessFunc = func(context.Context, string) error { started <- struct{}{}; return nil }
	f.svc.agentManager = manager
	f.svc.executor = withTestExecutorActor(executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{}))
	_, err = f.svc.StartCreatedSession(ctx, f.taskID, f.session.ID, "errand-profile", "initial instruction", true, false, true, nil, nil)
	require.NoError(t, err)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("initial worker did not start")
	}
	manager.mu.Lock()
	admit, failed := manager.initialPromptAdmissionCallback, manager.initialPromptFailureCallback
	manager.mu.Unlock()
	require.NotNil(t, admit)
	require.NotNil(t, failed)
	require.Error(t, admit(ctx, "prepared initial instruction", nil))
	failed()

	pending, err := f.repo.ListUnresolvedErrandParentAdmissions(ctx, f.row.ID)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	replay, err := f.repo.ReserveErrandParentAdmission(ctx, pending[0])
	require.NoError(t, err)
	require.False(t, replay.Created)
	require.Equal(t, models.ErrandAdmissionUnknown, replay.State, "failed refusal bookkeeping must be followed by the legacy Unknown notification")
	_, err = f.repo.GetAgentDeliverySubmission(ctx, "prompt:peer:initial-bookkeeping")
	require.Error(t, err, "preparation failure must happen before a durable submission is created")
	require.Empty(t, manager.capturedPrompts, "pre-dispatch rejection must never reach the provider")
	turn, err := f.repo.GetTurn(ctx, pending[0].TurnID)
	require.NoError(t, err)
	require.Nil(t, turn.CompletedAt, "rollback eligibility must not assert reservation clearance")
}

func TestErrandPromptPreDispatchRefusalWriteFailureKeepsCleanupAndReplaySeparate(t *testing.T) {
	f := newAdmissionIntegrationFixture(t, messagequeue.DeliveryProtocolV1)
	removed, err := f.repo.DeleteTurnIfUnreferenced(t.Context(), f.session.ID, "admission-turn")
	require.NoError(t, err)
	require.True(t, removed)
	f.svc.SetErrandController(&refusedAdmissionWriteFailure{Service: f.errands})
	_, err = f.repo.DB().Exec(`CREATE TRIGGER reject_pre_dispatch_submission BEFORE INSERT ON agent_delivery_submissions
		BEGIN SELECT RAISE(ABORT, 'submission preparation unavailable'); END`)
	require.NoError(t, err)
	ctx := errands.WithParentPromptOrigin(t.Context(), errands.ParentPromptOrigin{SenderSessionID: f.parent, SubmissionID: "peer:pre-dispatch"})
	_, dispatchErr := f.svc.PromptTask(ctx, f.taskID, f.session.ID, "pre-dispatch failure", "", false, nil, true)
	require.Error(t, dispatchErr)
	require.ErrorContains(t, dispatchErr, "parent refusal write unavailable")
	require.NotErrorIs(t, dispatchErr, agentruntime.ErrUncertainPromptDelivery, "callback rejection remains on the established pre-dispatch error policy")
	require.Empty(t, f.manager.capturedPrompts, "submission preparation fails before provider dispatch")
	require.Empty(t, f.svc.reservedPromptTurnID(f.session.ID), "pre-dispatch rejection remains eligible for guarded turn cleanup")
	pending, err := f.repo.ListUnresolvedErrandParentAdmissions(ctx, f.row.ID)
	require.NoError(t, err)
	require.Len(t, pending, 1, "turn cleanup does not clear the durable parent reservation")
	replay, err := f.repo.ReserveErrandParentAdmission(ctx, pending[0])
	require.NoError(t, err)
	require.False(t, replay.Created, "rollback eligibility does not authorize prompt replay")
}
