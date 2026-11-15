package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type absentPreparedDeliveryLog struct{}

func (absentPreparedDeliveryLog) HasDeliveryToken(context.Context, string, string) (bool, error) {
	return false, nil
}

func preparedOwnershipCancelledChild(t *testing.T, f *sendNowErrandFixture, id string) *models.Errand {
	t.Helper()
	row := f.result(t, id)
	require.NoError(t, f.repo.CreateTaskSession(t.Context(), &models.TaskSession{
		ID: row.ID, TaskID: row.TaskID, ParentSessionID: row.ParentSessionID, Role: models.SessionRoleErrand,
		State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(),
	}))
	session, err := f.repo.GetTaskSession(t.Context(), row.ID)
	require.NoError(t, err)
	row.TeardownIntent = &models.ErrandTeardownIntent{Version: models.ErrandTeardownIntentVersion, ID: "teardown-" + id,
		State: models.ErrandTeardownIntentPending, RequestedAt: time.Now().UTC(),
		Target: models.ErrandTeardownTarget{TaskID: row.TaskID, SessionID: row.ID, SessionIncarnation: session.QueueIncarnationID, NoTurn: true}}
	require.NoError(t, f.repo.UpdateErrand(t.Context(), row))
	f.manager.getExecutionIDForSessionFunc = func(_ context.Context, sessionID string) (string, error) {
		if sessionID == row.ID {
			return "", agentruntime.ErrNoExecutionForSession
		}
		return "exec-1", nil
	}
	outcome, err := NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), *row.TeardownIntent)
	require.NoError(t, err)
	require.Equal(t, errands.TeardownCompleted, outcome)
	row, err = f.repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
	return row
}

// @covers AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-004.11
func TestPreparedErrandOwnershipSurvivesReconcileAndExplicitRelease(t *testing.T) {
	for _, operation := range []string{"reconcile", "release", "turn_release"} {
		t.Run(operation, func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			service := f.restartDelivery(t, true)
			row := preparedOwnershipCancelledChild(t, f, "retained-ownership")
			require.NoError(t, service.PreserveCancellationNotice(t.Context(), row.ID))
			before, err := f.repo.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			require.Equal(t, models.ErrandResultDelivered, before.ResultState)
			service = f.restartDelivery(t, true)
			switch operation {
			case "reconcile":
				service.SetDeliveryLog(absentPreparedDeliveryLog{})
				require.NoError(t, service.Reconcile(t.Context()))
			case "turn_release":
				require.NoError(t, service.ReleaseTurnClaim(t.Context(), before.DeliveryToken))
			default:
				claims, err := f.repo.ListErrandResultClaims(t.Context(), row.TaskID, row.ParentSessionID, before.DeliveryToken)
				require.NoError(t, err)
				require.Len(t, claims, 1)
				changed, err := f.repo.ReleaseErrandResultClaims(t.Context(), row.TaskID, row.ParentSessionID, claims)
				require.NoError(t, err)
				require.Empty(t, changed, "an independently owned queued notice must retain its exact result claim")
			}
			after, err := f.repo.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			require.Equal(t, before.ResultState, after.ResultState)
			require.Equal(t, before.DeliveryToken, after.DeliveryToken)
			require.Equal(t, before.Version, after.Version)
			require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", true))
			require.Equal(t, 1, strings.Count(f.completeFIFO(t), row.ResultText))
		})
	}
}

// @covers AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-004.11
func TestPreparedErrandRefusedSourceReopenDeliversRevisionOnce(t *testing.T) {
	for _, path := range []string{"fifo", "send_now"} {
		t.Run(path, func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			service := f.restartDelivery(t, true)
			row := preparedOwnershipCancelledChild(t, f, "refused-retained")
			require.NoError(t, service.PreserveCancellationNotice(t.Context(), row.ID))
			require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", true))
			identity, err := f.svc.parentQueueIdentity(t.Context(), "session-1")
			require.NoError(t, err)
			if path == "fifo" {
				source, exists, _, err := f.svc.messageQueue.ReserveQueuedWithAutoRunForSession(t.Context(), identity)
				require.NoError(t, err)
				require.True(t, exists)
				_, err = f.svc.messageQueue.RestoreMessage(t.Context(), source)
				require.NoError(t, err)
			} else {
				claim, err := f.svc.messageQueue.ClaimSendNowForSession(t.Context(), identity, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries)
				require.NoError(t, err)
				require.NoError(t, f.svc.messageQueue.RestoreSendNowClaim(t.Context(), claim))
			}
			// Reconcile before dispatch and release from the same restored authority.
			require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", false))
			service = f.restartDelivery(t, true)
			service.SetDeliveryLog(absentPreparedDeliveryLog{})
			require.NoError(t, service.Reconcile(t.Context()))
			after, err := f.repo.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			require.Equal(t, models.ErrandResultDelivered, after.ResultState)
			require.NoError(t, service.ReleaseTurnClaim(t.Context(), after.DeliveryToken))
			entries := f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries
			require.Len(t, entries, 1)
			require.Equal(t, messagequeue.QueuedByServer, entries[0].QueuedBy)
			require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", true))
			require.Equal(t, 1, strings.Count(f.completeFIFO(t), row.ResultText))
		})
	}
}
