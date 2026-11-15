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
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestErrandParentUnsentFailureFences(t *testing.T) {
	for _, scenario := range []string{"accepted", "attempted write", "lost reply", "nested uncertainty", "owner", "hash", "incarnation", "attempt", "storage failure", "referenced turn"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAdmissionIntegrationFixture(t, messagequeue.DeliveryProtocolV1)
			removed, err := f.repo.DeleteTurnIfUnreferenced(t.Context(), f.session.ID, "admission-turn")
			require.NoError(t, err)
			require.True(t, removed)
			notSent := (&agentctl.Client{}).PromptWithSubmissionID(t.Context(), "unsent", nil, 1, "peer:fenced")
			f.manager.promptAcceptedOnError = scenario == "accepted"
			var turnID string
			f.manager.promptAgentFunc = func(ctx context.Context, _, _ string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
				turn, err := f.repo.GetActiveTurnBySessionID(ctx, f.session.ID)
				require.NoError(t, err)
				turnID = turn.ID
				switch scenario {
				case "attempted write":
					return nil, agentctl.ErrAgentStreamNotConnected
				case "lost reply":
					return nil, context.DeadlineExceeded
				case "nested uncertainty":
					return nil, errors.Join(notSent, agentruntime.ErrUncertainPromptDelivery)
				case "owner", "hash", "incarnation", "attempt":
					column := map[string]string{"owner": "owner_generation", "hash": "payload_hash", "incarnation": "incarnation_id", "attempt": "dispatch_attempt_id"}[scenario]
					value := any("changed")
					if scenario == "owner" {
						value = 2
					}
					_, err := f.repo.DB().ExecContext(ctx, "UPDATE agent_delivery_submissions SET "+column+" = ? WHERE id = ?", value, "prompt:peer:fenced")
					require.NoError(t, err)
				case "storage failure":
					_, err := f.repo.DB().ExecContext(ctx, `CREATE TRIGGER refuse_unsent_settlement BEFORE UPDATE ON agent_delivery_submissions WHEN NEW.fully_settled_at IS NOT NULL BEGIN SELECT RAISE(ABORT, 'injected settlement failure'); END`)
					require.NoError(t, err)
				case "referenced turn":
					require.NoError(t, f.repo.CreateMessage(ctx, &models.Message{ID: "retained-output", TaskID: f.taskID,
						TaskSessionID: f.session.ID, TurnID: turn.ID, AuthorType: models.MessageAuthorAgent, Content: "retained concurrent output"}))
				}
				return nil, notSent
			}
			ctx := errands.WithParentPromptOrigin(t.Context(), errands.ParentPromptOrigin{SenderSessionID: f.parent, SubmissionID: "peer:fenced"})
			_, dispatchErr := f.svc.PromptTask(ctx, f.taskID, f.session.ID, "fenced instruction", "", false, nil, true)
			require.Error(t, dispatchErr)
			submission, err := f.repo.GetAgentDeliverySubmissionMetadata(ctx, "prompt:peer:fenced")
			require.NoError(t, err)
			turn, err := f.repo.GetTurn(ctx, turnID)
			require.NoError(t, err, "uncertain, accepted and referenced turns must survive")
			if scenario == "referenced turn" {
				require.NotNil(t, submission.FullySettledAt)
				require.NotErrorIs(t, dispatchErr, agentruntime.ErrUncertainPromptDelivery)
			} else {
				require.Nil(t, submission.FullySettledAt)
				block, err := f.svc.GetOpenSessionRecoveryBlock(ctx, f.session.ID)
				require.NoError(t, err)
				require.NotNil(t, block)
			}
			if scenario != "accepted" {
				require.Nil(t, turn.CompletedAt)
			}
			row, err := f.repo.GetErrand(ctx, f.row.ID)
			require.NoError(t, err)
			if scenario == "accepted" {
				require.EqualValues(t, 1, row.ParentAdmissionGeneration)
				require.Equal(t, models.ErrandResultSuperseded, row.ResultState)
				require.NotEqual(t, true, turn.Metadata[models.TurnMetaKeyPromptDispatchPending])
			} else {
				require.Zero(t, row.ParentAdmissionGeneration)
				require.Equal(t, models.ErrandResultPending, row.ResultState)
				require.Equal(t, f.row.ResultRevision, row.ResultRevision)
			}
			require.Len(t, f.manager.capturedPrompts, 1)
		})
	}
}

type preparingErrandPromptManager struct {
	*retentionPromptAgentManager
	prepare func(context.Context)
}

func (m *preparingErrandPromptManager) PromptAgentWithAdmissionCallbackAndSubmissionID(
	ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool,
	admit func(context.Context, string, []v1.MessageAttachment) error, accepted func(), submissionID string,
) (*executor.PromptResult, error) {
	m.prepare(ctx)
	return m.retentionPromptAgentManager.PromptAgentWithAdmissionCallbackAndSubmissionID(ctx, executionID, prompt, attachments, dispatchOnly, admit, accepted, submissionID)
}

func TestErrandParentPromptPreparationRejectsStaleOwner(t *testing.T) {
	for _, scenario := range []string{"cancellation", "successor"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAdmissionIntegrationFixture(t, messagequeue.DeliveryProtocolV1)
			removed, err := f.repo.DeleteTurnIfUnreferenced(t.Context(), f.session.ID, "admission-turn")
			require.NoError(t, err)
			require.True(t, removed)
			manager := &preparingErrandPromptManager{retentionPromptAgentManager: f.manager}
			f.svc.agentManager = manager
			f.svc.executor = withTestExecutorActor(executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{}))
			var cancellation *cancelOperation
			manager.prepare = func(ctx context.Context) {
				guard := f.svc.lockCancelInFlightGuard(f.session.ID)
				defer guard.release()
				if scenario == "cancellation" {
					var owner bool
					cancellation, owner = f.svc.claimCancellation(f.session.ID, cancellationKindExplicit)
					require.True(t, owner)
					return
				}
				f.svc.rollbackReservedPromptTurn(ctx, f.session.ID, f.svc.reservedPromptTurnID(f.session.ID))
				now := time.Now().UTC()
				require.NoError(t, f.repo.CreateTurn(ctx, &models.Turn{ID: "successor-turn", TaskID: f.taskID,
					TaskSessionID: f.session.ID, StartedAt: now, CreatedAt: now, UpdatedAt: now}))
				seedExecutorRunning(t, f.repo, f.session.ID, f.taskID, "successor-execution")
			}
			ctx := errands.WithParentPromptOrigin(t.Context(), errands.ParentPromptOrigin{SenderSessionID: f.parent, SubmissionID: "peer:stale"})
			_, dispatchErr := f.svc.PromptTask(ctx, f.taskID, f.session.ID, "stale instruction", "", false, nil, true)
			require.Error(t, dispatchErr)
			if cancellation != nil {
				f.svc.finishCancellation(f.session.ID, cancellation, nil)
			} else {
				turn, err := f.repo.GetTurn(ctx, "successor-turn")
				require.NoError(t, err)
				require.Nil(t, turn.CompletedAt)
				session, err := f.repo.GetTaskSession(ctx, f.session.ID)
				require.NoError(t, err)
				require.Equal(t, models.TaskSessionStateRunning, session.State)
				require.Equal(t, "successor-execution", session.AgentExecutionID)
			}
			require.Empty(t, f.manager.capturedPrompts)
			rows, err := f.repo.ListAgentDeliverySubmissions(ctx, f.session.ID)
			require.NoError(t, err)
			require.Empty(t, rows)
		})
	}
}
