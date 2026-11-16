package orchestrator

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-002.8
func TestErrandContinuationRefusesUnresolvedWork(t *testing.T) {
	for _, parent := range []bool{false, true} {
		t.Run(map[bool]string{false: "user", true: "parent"}[parent], func(t *testing.T) {
			f := newQueueSettlementFixture(t, parent)
			require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), f.child.ID, false))
			f.svc.abandonOpenTurnsOnStartup(t.Context(), f.child.ID, "continuation test")
			require.NoError(t, f.repo.UpdateTaskSessionState(t.Context(), f.child.ID, models.TaskSessionStateWaitingForInput, ""))
			revision, err := f.repo.ReadConversationRevision(t.Context(), f.child.ID)
			require.NoError(t, err)
			before, err := f.repo.ListTurnsBySession(t.Context(), f.child.ID)
			require.NoError(t, err)
			ctx := t.Context()
			if parent {
				ctx = errands.WithParentPromptOrigin(ctx, errands.ParentPromptOrigin{SenderSessionID: f.parent, SubmissionID: "new-parent-request"})
			}
			for range 2 {
				_, err = f.svc.PromptTask(ctx, f.taskID, f.child.ID, "new instruction", "", false, nil, false)
				require.ErrorIs(t, err, db.ErrErrandSettlementPending)
				response, ok := errands.WSError("request", "message.add", err)
				require.True(t, ok, "settlement refusal must reach the transport")
				var payload struct {
					Details map[string]interface{} `json:"details"`
				}
				require.NoError(t, json.Unmarshal(response.Payload, &payload))
				require.Equal(t, "errand_recovery_unresolved", payload.Details["reason"])
			}
			after, err := f.repo.ListTurnsBySession(t.Context(), f.child.ID)
			require.NoError(t, err)
			require.Equal(t, before, after)
			afterRevision, err := f.repo.ReadConversationRevision(t.Context(), f.child.ID)
			require.NoError(t, err)
			require.Equal(t, revision, afterRevision)

			select {
			case <-f.manager.finished:
				t.Fatal("refusal dispatched provider work")
			default:
			}
			require.Zero(t, f.taskRepo.stateWrites[f.taskID])
		})
	}
}
