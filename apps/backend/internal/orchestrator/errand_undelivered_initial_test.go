package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type unavailableUnsentReceipt struct{ *sqliterepo.Repository }

func (*unavailableUnsentReceipt) AgentDeliverySubmissionWasUnsent(context.Context, *models.AgentDeliverySubmission) (bool, error) {
	return false, errors.New("receipt read unavailable")
}

// @covers AC-PLATFORM-UNDELIVERED-PROMPT-001.1 AC-TASKS-ERRANDS-002.7
func TestErrandInitialFailureUsesCommittedNonDelivery(t *testing.T) {
	for _, outcome := range []string{"unsent", "generic failure", "changed hash", "accepted", "read failure"} {
		t.Run(outcome, func(t *testing.T) {
			f := newAdmissionIntegrationFixture(t, messagequeue.DeliveryProtocolV1)
			manager := &retentionInitialPromptManager{durableDeliveryTestAgentManager: f.manager.durableDeliveryTestAgentManager}
			manager.isAgentRunning = false
			started := make(chan struct{}, 1)
			manager.startAgentProcessFunc = func(context.Context, string) error { started <- struct{}{}; return nil }
			f.svc.agentManager = manager
			f.svc.executor = withTestExecutorActor(executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{}))
			if outcome == "read failure" {
				f.svc.repo = &unavailableUnsentReceipt{f.repo}
			}
			ctx := errands.WithParentPromptOrigin(t.Context(), errands.ParentPromptOrigin{SenderSessionID: f.parent, SubmissionID: "peer:initial-unsent"})
			require.NoError(t, f.repo.UpdateTaskSessionState(ctx, f.session.ID, models.TaskSessionStateCreated, ""))
			_, err := f.svc.StartCreatedSession(ctx, f.taskID, f.session.ID, "errand-profile", "initial instruction", true, false, true, nil, nil)
			require.NoError(t, err)
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("initial worker did not start")
			}
			manager.mu.Lock()
			admit, accepted, failed := manager.initialPromptAdmissionCallback, manager.initialPromptDispatchCallback, manager.initialPromptFailureCallback
			manager.mu.Unlock()
			require.NotNil(t, admit)
			require.NotNil(t, failed)
			require.NoError(t, admit(ctx, "prepared initial instruction", nil))
			pending, err := f.repo.ListUnresolvedErrandParentAdmissions(ctx, f.row.ID)
			require.NoError(t, err)
			require.Len(t, pending, 1)
			submission, err := f.repo.GetAgentDeliverySubmission(ctx, pending[0].SubmissionID)
			require.NoError(t, err)
			if outcome == "generic failure" {
				_, err = f.repo.TransitionAgentDeliverySubmission(ctx, submission.ID, submission.State, models.DeliverySubmissionFailed, "failed", time.Now().UTC())
				require.NoError(t, err)
			} else {
				changed, err := f.repo.FailUnsentAgentDeliverySubmission(ctx, submission, time.Now().UTC())
				require.NoError(t, err)
				require.True(t, changed)
			}
			if outcome == "changed hash" {
				_, err := f.repo.DB().ExecContext(ctx, "UPDATE agent_delivery_submissions SET payload_hash = ? WHERE id = ?", "changed", submission.ID)
				require.NoError(t, err)
			}
			if outcome == "accepted" {
				accepted()
			}
			failed()
			reservation, err := f.repo.ReserveErrandParentAdmission(ctx, pending[0])
			require.NoError(t, err)
			expected := models.ErrandAdmissionUnknown
			switch outcome {
			case "unsent":
				expected = models.ErrandAdmissionRefused
			case "accepted":
				expected = models.ErrandAdmissionAccepted
			}
			require.Equal(t, expected, reservation.State)
			row, err := f.repo.GetErrand(ctx, f.row.ID)
			require.NoError(t, err)
			if outcome == "accepted" {
				require.EqualValues(t, 1, row.ParentAdmissionGeneration)
			} else {
				require.Zero(t, row.ParentAdmissionGeneration)
				require.Equal(t, models.ErrandResultPending, row.ResultState)
				require.Equal(t, f.row.ResultRevision, row.ResultRevision)
			}
			require.Empty(t, f.manager.capturedPrompts, "failure reconciliation cannot replay a provider prompt")
		})
	}
}
