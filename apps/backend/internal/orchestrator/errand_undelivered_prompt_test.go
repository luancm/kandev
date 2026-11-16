package orchestrator

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-PLATFORM-UNDELIVERED-PROMPT-001.1 AC-TASKS-ERRANDS-002.7
func TestErrandParentPromptDefinitelyUnsent(t *testing.T) {
	f := newAdmissionIntegrationFixture(t, messagequeue.DeliveryProtocolV1)
	removed, err := f.repo.DeleteTurnIfUnreferenced(t.Context(), f.session.ID, "admission-turn")
	require.NoError(t, err)
	require.True(t, removed, "the public prompt must own a new turn")
	notSentErr := (&agentctl.Client{}).PromptWithSubmissionID(t.Context(), "unsent", nil, 1, "peer:unsent")
	var notSent *agentruntime.RequestNotSentError
	require.ErrorAs(t, notSentErr, &notSent)
	var attemptedTurnID string
	f.manager.promptAgentFunc = func(ctx context.Context, _, _ string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
		turn, err := f.svc.turnService.GetActiveTurn(ctx, f.session.ID)
		require.NoError(t, err)
		require.NotNil(t, turn)
		attemptedTurnID = turn.ID
		return nil, notSentErr
	}
	ctx := errands.WithParentPromptOrigin(t.Context(), errands.ParentPromptOrigin{
		SenderSessionID: f.parent, SubmissionID: "peer:unsent",
	})
	_, dispatchErr := f.svc.PromptTask(ctx, f.taskID, f.session.ID, "unsent instruction", "", false, nil, true)
	require.Error(t, dispatchErr)
	submission, err := f.repo.GetAgentDeliverySubmissionMetadata(ctx, "prompt:peer:unsent")
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionFailed, submission.State)
	require.Equal(t, "failed", submission.Outcome)
	require.NotNil(t, submission.FullySettledAt)
	require.Empty(t, submission.TerminalTurnID)
	require.Empty(t, submission.TerminalStreamID)
	require.Zero(t, submission.TerminalSequence)
	require.NotErrorIs(t, dispatchErr, agentruntime.ErrUncertainPromptDelivery,
		"committed non-delivery must survive the parent admission boundary")
	reservation, err := f.repo.ReserveErrandParentAdmission(ctx, models.ErrandParentAdmission{
		TaskID: f.taskID, ParentSessionID: f.parent, ErrandID: f.row.ID, SenderSessionID: f.parent,
		TurnID: attemptedTurnID, SubmissionID: "prompt:peer:unsent",
	})
	require.NoError(t, err)
	require.False(t, reservation.Created)
	require.Equal(t, models.ErrandAdmissionRefused, reservation.State)
	_, err = f.repo.GetTurn(ctx, attemptedTurnID)
	require.ErrorIs(t, err, sql.ErrNoRows, "only the rejected empty reservation is removed")
	decision, err := f.repo.GetErrandTurnSettlementDecision(ctx, f.session.ID, submission.IncarnationID, attemptedTurnID)
	require.NoError(t, err)
	require.Equal(t, models.DeliveryEffectPending, decision.State, "non-delivery cannot fabricate an Errand terminal decision")
	row, err := f.repo.GetErrand(ctx, f.row.ID)
	require.NoError(t, err)
	require.Zero(t, row.ParentAdmissionGeneration)
	require.Equal(t, models.ErrandResultPending, row.ResultState)
	require.Equal(t, f.row.ResultRevision, row.ResultRevision)
	require.Equal(t, "old output", row.ResultText)
	block, err := f.svc.GetOpenSessionRecoveryBlock(ctx, f.session.ID)
	require.NoError(t, err)
	require.Nil(t, block)

	f.manager.promptAgentFunc = nil
	next := errands.WithParentPromptOrigin(t.Context(), errands.ParentPromptOrigin{
		SenderSessionID: f.parent, SubmissionID: "peer:distinct",
	})
	result, err := f.svc.PromptTask(next, f.taskID, f.session.ID, "distinct instruction", "", false, nil, true)
	require.NoError(t, err)
	require.NotEqual(t, attemptedTurnID, result.TurnID)
	require.Equal(t, []string{"unsent instruction", "distinct instruction"}, f.manager.capturedPrompts,
		"a later instruction must never replay the refused prompt")
	row, err = f.repo.GetErrand(ctx, f.row.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, row.ParentAdmissionGeneration)
	require.Equal(t, models.ErrandResultSuperseded, row.ResultState)
}

// A clarification can reuse an open turn. Refusing its next instruction must
// not manufacture terminal evidence for the work already owned by that turn.
func TestErrandParentUnsentPromptPreservesReusedTurn(t *testing.T) {
	f := newAdmissionIntegrationFixture(t, messagequeue.DeliveryProtocolV1)
	require.NoError(t, f.repo.CreateMessage(t.Context(), &models.Message{
		ID: "earlier-input", TaskID: f.taskID, TaskSessionID: f.session.ID,
		TurnID: "admission-turn", AuthorType: models.MessageAuthorUser, Content: "earlier work",
	}))
	before, err := f.repo.GetTurn(t.Context(), "admission-turn")
	require.NoError(t, err)
	f.manager.promptErr = (&agentctl.Client{}).PromptWithSubmissionID(t.Context(), "unsent", nil, 1, "peer:unsent")
	ctx := errands.WithParentPromptOrigin(t.Context(), errands.ParentPromptOrigin{
		SenderSessionID: f.parent, SubmissionID: "peer:unsent",
	})
	_, dispatchErr := f.svc.PromptTask(ctx, f.taskID, f.session.ID, "unsent instruction", "", false, nil, true)
	require.Error(t, dispatchErr)
	require.NotErrorIs(t, dispatchErr, agentruntime.ErrUncertainPromptDelivery)
	after, err := f.repo.GetTurn(ctx, before.ID)
	require.NoError(t, err)
	require.Nil(t, after.CompletedAt)
	require.Equal(t, before.StartedAt, after.StartedAt)
	messages, err := f.repo.ListMessages(ctx, f.session.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "earlier work", messages[0].Content)
	row, err := f.repo.GetErrand(ctx, f.row.ID)
	require.NoError(t, err)
	require.Equal(t, f.row.ResultRevision, row.ResultRevision)
	require.Equal(t, models.ErrandResultPending, row.ResultState)
	require.Zero(t, row.ParentAdmissionGeneration)
}
