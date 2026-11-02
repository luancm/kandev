package orchestrator

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-012.2
func TestLegacyPreparedErrandFIFOClaimOwnsProvenRevisionBeforeRowDeletion(t *testing.T) {
	f := newSendNowErrandFixture(t)
	row := f.result(t, "legacy-owned")
	source := f.queue(t, "full legacy block beyond preview", messagequeue.QueuedByServer, preparedSendNowMetadata("legacy-token", row.ID))
	f.restartDelivery(t, true)
	require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", true))
	identity, err := f.svc.parentQueueIdentity(t.Context(), "session-1")
	require.NoError(t, err)
	reserved, exists, autoRun, err := f.svc.messageQueue.ReserveQueuedWithAutoRunForSession(t.Context(), identity)
	require.NoError(t, err)
	require.True(t, exists)
	require.True(t, autoRun)
	require.Equal(t, source.ID, reserved.ID)
	stamp, present, err := decodeSendNowErrandSource(reserved.Metadata)
	require.NoError(t, err)
	require.True(t, present, "legacy normalization must travel in the durable claim before the row can disappear")
	require.Equal(t, source.ID, stamp.Receipt.SourceID)
	require.Equal(t, row.ResultRevision, stamp.Receipt.Claims[0].ResultRevision)
	require.Equal(t, identity, *stamp.Identity)
	require.Equal(t, source.Content, stamp.PreparedBlock)
	pending, err := f.svc.messageQueue.ListPendingQueueDispatches(t.Context())
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, source.ID, pending[0].Message.ID)
	stored, storedPresent, err := decodeSendNowErrandSource(pending[0].Message.Metadata)
	require.NoError(t, err)
	require.True(t, storedPresent)
	require.Equal(t, stamp, stored)
	require.NoError(t, f.repo.DeleteErrand(t.Context(), row.ID))
	// A definite pre-dispatch refusal restores the complete normalized source.
	_, err = f.svc.messageQueue.RestoreMessage(t.Context(), reserved)
	require.NoError(t, err)
	f.restartDelivery(t, false)
	dispatched, _ := f.attemptFIFO(t)
	require.False(t, dispatched)
	retained := f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries
	require.Len(t, retained, 1)
	require.Equal(t, source.ID, retained[0].ID)
	require.Equal(t, source.Content, retained[0].Content)
	f.restartDelivery(t, true)
	require.Equal(t, source.Content, f.completeFIFO(t))
	require.Empty(t, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries)
}

// @covers AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-012.2
func TestLegacyPreparedErrandSendNowClaimOwnsProvenRevisionBeforeRowDeletion(t *testing.T) {
	f := newSendNowErrandFixture(t)
	row := f.result(t, "legacy-owned")
	source := f.queue(t, "full legacy block beyond preview", messagequeue.QueuedByServer, preparedSendNowMetadata("legacy-token", row.ID))
	identity, err := f.svc.parentQueueIdentity(t.Context(), "session-1")
	require.NoError(t, err)
	claim, err := f.svc.messageQueue.ClaimSendNowForSession(t.Context(), identity, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries)
	require.NoError(t, err)
	input, err := f.svc.prepareSendNowErrandInput(t.Context(), claim)
	require.NoError(t, err)
	_, _, err = f.svc.composeSendNowErrandTurn(t.Context(), input, claim.Dispatch.Content)
	require.NoError(t, err)
	pending, err := f.svc.messageQueue.ListPendingSendNowClaims(t.Context())
	require.NoError(t, err)
	require.Len(t, pending, 1)
	snapshot, present, err := errands.DecodeSendNowDelivery(pending[0].Claim.Dispatch.Metadata)
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, source.ID, snapshot.Receipts[0].SourceID)
	require.Equal(t, row.ResultRevision, snapshot.Receipts[0].Claims[0].ResultRevision)
	require.Equal(t, identity.SessionIncarnationID, snapshot.SessionIncarnationID)
	require.Equal(t, source.Content, snapshot.Sources[0].PreparedBlock)
	_, normalized, err := decodeSendNowErrandSource(pending[0].Claim.Sources[0].Metadata)
	require.NoError(t, err)
	require.False(t, normalized, "dispatch enrichment cannot change immutable original sources")
	require.NoError(t, f.repo.DeleteErrand(t.Context(), row.ID))
	require.NoError(t, f.svc.restoreSendNowClaimWithRetry(t.Context(), &pending[0].Claim))
	f.restartDelivery(t, false)
	require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", true))
	dispatched, _ := f.attemptFIFO(t)
	require.False(t, dispatched)
	retained := f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries
	require.Len(t, retained, 1)
	require.Equal(t, source.ID, retained[0].ID)
	require.Equal(t, source.Content, retained[0].Content)
	f.restartDelivery(t, true)
	require.Equal(t, source.Content, f.completeFIFO(t))
	require.Empty(t, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries)
}

// @covers AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-004.9
func TestLegacyPreparedErrandRefusalRetainsExactClaim(t *testing.T) {
	for _, path := range []string{"fifo", "send_now"} {
		for _, generation := range []int{0, 3} {
			for _, recovery := range []string{"reconcile", "exact_release"} {
				t.Run(fmt.Sprintf("%s/generation=%d/%s", path, generation, recovery), func(t *testing.T) {
					f, before, source := legacyPreparedErrandClaim(t, generation)
					claims, err := f.repo.ListErrandResultClaims(t.Context(), before.TaskID, before.ParentSessionID, before.DeliveryToken)
					require.NoError(t, err)
					require.Len(t, claims, 1)
					refuseLegacyPreparedErrand(t, f, path)
					f.svc.agentManager = f.manager
					require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", false))
					service := f.restartDelivery(t, true)
					entries := f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries
					require.Len(t, entries, 1, "definite refusal must restore the full original source")
					require.Equal(t, source.ID, entries[0].ID)
					require.Equal(t, source.Content, entries[0].Content)
					if recovery == "reconcile" {
						service.SetDeliveryLog(absentPreparedDeliveryLog{})
						require.NoError(t, service.Reconcile(t.Context()))
					} else {
						changed, err := f.repo.ReleaseErrandResultClaims(t.Context(), before.TaskID, before.ParentSessionID, claims)
						require.NoError(t, err)
						require.Empty(t, changed, "a normalized pending source must retain its original exact result claim")
					}
					after, err := f.repo.GetErrand(t.Context(), before.ID)
					require.NoError(t, err)
					require.Equal(t, before, after, "refusal, reopen and recovery must not release or reclaim the original revision")
					retained, err := f.repo.ListErrandResultClaims(t.Context(), before.TaskID, before.ParentSessionID, before.DeliveryToken)
					require.NoError(t, err)
					require.Equal(t, claims, retained)
					stamp, present, err := decodeSendNowErrandSource(entries[0].Metadata)
					require.NoError(t, err)
					require.True(t, present)
					require.NotNil(t, stamp.QueueGeneration)
					require.EqualValues(t, generation, *stamp.QueueGeneration)
					require.Equal(t, claims, stamp.Receipt.Claims)
					require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", true))
					prompt := f.completeFIFO(t)
					require.Equal(t, source.Content, prompt, "positive control dispatches the complete block without recomposing its result")
					require.Equal(t, 1, strings.Count(prompt, before.ResultText))
					require.Empty(t, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries)
					_, queue := f.reopen(t)
					require.Empty(t, queue.GetStatus(t.Context(), "session-1").Entries, "successful delivery must not restore a second copy")
					pending, err := queue.ListPendingQueueDispatches(t.Context())
					require.NoError(t, err)
					require.Empty(t, pending)
				})
			}
		}
	}
}

func legacyPreparedErrandClaim(t *testing.T, generation int) (*sendNowErrandFixture, *models.Errand, *messagequeue.QueuedMessage) {
	t.Helper()
	f := newSendNowErrandFixture(t)
	f.restartDelivery(t, true)
	for range generation {
		_, err := f.svc.messageQueue.PurgeTask(t.Context(), "task-1")
		require.NoError(t, err)
	}
	for range 5 {
		_, err := f.svc.messageQueue.PurgeSession(t.Context(), "session-1")
		require.NoError(t, err)
	}
	taskGeneration, err := f.svc.messageQueue.LifecycleGeneration(t.Context(), "task-1")
	require.NoError(t, err)
	require.EqualValues(t, generation, taskGeneration)
	sessionGeneration, err := f.svc.messageQueue.SessionGeneration(t.Context(), "session-1")
	require.NoError(t, err)
	require.GreaterOrEqual(t, sessionGeneration, int64(5))
	require.NotEqual(t, taskGeneration, sessionGeneration)
	require.NotEqual(t, taskGeneration, sessionGeneration+1, "Send Now operation generation must differ from source task generation")
	row := preparedOwnershipCancelledChild(t, f, "legacy-retained")
	candidates := []models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}
	claimed, err := f.repo.ClaimResults(t.Context(), row.TaskID, row.ParentSessionID, models.ErrandDeliveryNotification,
		"legacy-original-token", candidates, time.Now().UTC(), candidates)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	row, err = f.repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultDelivered, row.ResultState)
	source := f.queue(t, "complete legacy notice\n"+row.ResultText+"\nlegacy tail beyond preview", messagequeue.QueuedByServer,
		preparedSendNowMetadata(row.DeliveryToken, row.ID))
	return f, row, source
}

func refuseLegacyPreparedErrand(t *testing.T, f *sendNowErrandFixture, path string) {
	t.Helper()
	manager := &outcomeSendNowManager{mockAgentManager: f.manager, outcome: "refused"}
	f.svc.agentManager = manager
	f.svc.executor = executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{})
	if path == "send_now" {
		n, err := f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
		require.NoError(t, err)
		require.Equal(t, 1, n)
		waitSendNowErrandWorkers(t, f.svc)
	} else {
		require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", true))
		identity, err := f.svc.parentQueueIdentity(t.Context(), "session-1")
		require.NoError(t, err)
		reserved, exists, autoRun, err := f.svc.messageQueue.ReserveQueuedWithAutoRunForSession(t.Context(), identity)
		require.NoError(t, err)
		require.True(t, exists)
		require.True(t, autoRun)
		// A definite refusal before provider dispatch returns the exact reservation.
		_, err = f.svc.messageQueue.RestoreMessage(t.Context(), reserved)
		require.NoError(t, err)
	}
	fifo, err := f.svc.messageQueue.ListPendingQueueDispatches(t.Context())
	require.NoError(t, err)
	require.Empty(t, fifo)
	sendNow, err := f.svc.messageQueue.ListPendingSendNowClaims(t.Context())
	require.NoError(t, err)
	require.Empty(t, sendNow)
}
