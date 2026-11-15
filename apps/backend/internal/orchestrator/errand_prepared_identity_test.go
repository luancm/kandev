package orchestrator

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-012.2
func TestDisabledErrandPreparedIdentityAndGenerationFences(t *testing.T) {
	for _, generation := range []int{0, 3} {
		for _, invalidate := range []string{"none", "session", "task", "incarnation"} {
			t.Run(fmt.Sprintf("generation=%d/%s", generation, invalidate), func(t *testing.T) {
				f := newSendNowErrandFixture(t)
				for range generation {
					_, err := f.svc.messageQueue.PurgeTask(t.Context(), "task-1")
					require.NoError(t, err)
				}
				require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", false))
				prepared := f.prepared(t, "token", "deleted", "complete prepared block")
				user := f.queue(t, "ordinary original", messagequeue.QueuedByUser, nil)
				identity, err := f.svc.parentQueueIdentity(t.Context(), "session-1")
				require.NoError(t, err)
				claim, err := f.svc.messageQueue.ClaimSendNowForSession(t.Context(), identity, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries)
				require.NoError(t, err)
				require.EqualValues(t, generation, claim.SourceGenerations["task-1"])
				f.restartDelivery(t, false)
				switch invalidate {
				case "session":
					_, err = f.svc.messageQueue.PurgeSession(t.Context(), "session-1")
				case "task":
					_, err = f.svc.messageQueue.PurgeTask(t.Context(), "task-1")
				case "incarnation":
					_, err = f.repo.DB().Exec(`UPDATE task_sessions SET queue_incarnation_id = 'successor-incarnation' WHERE id = 'session-1'`)
				}
				require.NoError(t, err)
				err = f.svc.messageQueue.RestoreSendNowClaim(t.Context(), claim)
				if invalidate != "none" {
					if invalidate == "task" {
						require.NoError(t, err, "fully invalidated claims are discarded by existing task cleanup")
					} else {
						require.Error(t, err, "stale mixed claims cannot restore into a successor")
					}
					require.Empty(t, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries)
					return
				}
				require.NoError(t, err)
				require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", true))
				require.Equal(t, user.Content, f.completeFIFO(t))
				entries := f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries
				require.Len(t, entries, 1)
				require.Equal(t, prepared.ID, entries[0].ID)
				require.Equal(t, prepared.Content, entries[0].Content)
				turn, turnErr := f.repo.GetActiveTurnBySessionID(t.Context(), "session-1")
				require.NoError(t, turnErr)
				require.NoError(t, f.repo.CompleteTurn(t.Context(), turn.ID))
				f.svc.activeTurns.Delete("session-1")
				require.NoError(t, f.repo.UpdateTaskSessionState(t.Context(), "session-1", models.TaskSessionStateWaitingForInput, ""))
				f.restartDelivery(t, true)
				require.Equal(t, prepared.Content, f.completeFIFO(t))
			})
		}
	}
}

// @covers AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-012.2
func TestLegacyPreparedErrandCompleteAndContradictorySourceSelection(t *testing.T) {
	for _, kind := range []string{"complete", "missing_block", "contradictory_token", "contradictory_revision"} {
		t.Run(kind, func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			f.result(t, "legacy")
			metadata := preparedSendNowMetadata("legacy-token", "legacy")
			block := "complete legacy output"
			if kind == "missing_block" {
				block = ""
			}
			if kind == "contradictory_token" {
				metadata[ErrandDeliveryTokenMetadataKey] = "other-token"
			}
			if kind == "contradictory_revision" {
				stamp, err := f.svc.capturePreparedErrandSource(t.Context(), "task-1", "session-1", block, metadata)
				require.NoError(t, err)
				stamp.Receipt.Claims[0].Token = "other-token"
				metadata[errandSourceMetadataKey] = stamp
			}
			source := f.queue(t, block, messagequeue.QueuedByServer, metadata)
			f.restartDelivery(t, true)
			require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", true))
			// A legacy full source remains independent of live composition and is not rewritten.
			f.svc.SetErrandController(&fakeErrandComposer{})
			dispatched, prompt := f.attemptFIFO(t)
			if kind == "complete" {
				require.True(t, dispatched)
				require.Equal(t, block, prompt)
				require.Empty(t, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries)
			} else {
				require.False(t, dispatched)
				_, queue := f.reopen(t)
				entries := queue.GetStatus(t.Context(), "session-1").Entries
				require.Len(t, entries, 1)
				want, err := json.Marshal(source)
				require.NoError(t, err)
				got, err := json.Marshal(entries[0])
				require.NoError(t, err)
				require.JSONEq(t, string(want), string(got))
			}
		})
	}
}

// @covers AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-004.9
func TestLegacyPreparedErrandWithdrawalRetainsFullSources(t *testing.T) {
	f := newSendNowErrandFixture(t)
	row := f.result(t, "legacy-withdraw")
	metadata := preparedSendNowMetadata("legacy-token", row.ID)
	metadata[errandNotificationKey] = true
	full := f.queue(t, "full row-independent cancellation notice", messagequeue.QueuedByServer, metadata)
	placeholder := f.queue(t, "", messagequeue.QueuedByServer, map[string]interface{}{errandNotificationKey: true})
	f.svc.withdrawErrandNotification(t.Context(), "session-1")
	entries := f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries
	require.Len(t, entries, 1, "withdrawing a placeholder cannot discard a full prepared source")
	require.Equal(t, full.ID, entries[0].ID)
	require.Equal(t, full.Content, entries[0].Content)
	require.NotEqual(t, placeholder.ID, entries[0].ID)
}
