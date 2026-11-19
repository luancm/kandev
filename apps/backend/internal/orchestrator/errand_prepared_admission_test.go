package orchestrator

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-003.5 AC-TASKS-ERRANDS-004.9
func TestPreparedErrandCancellationOwnsImmutableRevisionAndFullBlock(t *testing.T) {
	f := newSendNowErrandFixture(t)
	service := f.restartDelivery(t, true)
	row := f.result(t, "immutable")
	row.ResultText = strings.Repeat("complete cancellation output ", 80) + "tail beyond preview"
	require.NoError(t, f.repo.UpdateErrand(t.Context(), row))
	row, err := f.repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.NoError(t, service.PreserveCancellationNotice(t.Context(), row.ID))
	first := f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries
	require.Len(t, first, 1)
	stamp, present, err := decodeSendNowErrandSource(first[0].Metadata)
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, first[0].Content, stamp.PreparedBlock)
	require.Contains(t, stamp.PreparedBlock, row.ResultText)
	require.NotContains(t, stamp.Receipt.Envelope.Items[0].Preview, "tail beyond preview")
	require.Equal(t, row.ResultRevision, stamp.Receipt.Claims[0].ResultRevision)
	require.Equal(t, row.ParentAdmissionGeneration, stamp.Receipt.Claims[0].ParentAdmissionGeneration)
	require.Equal(t, stamp.Receipt.Envelope.Token, first[0].Metadata[ErrandDeliveryTokenMetadataKey])
	require.Equal(t, stamp.Receipt.Envelope.Token, stamp.Receipt.Claims[0].Token)
	service = f.restartDelivery(t, true)
	require.NoError(t, service.PreserveCancellationNotice(t.Context(), row.ID))
	require.Equal(t, first, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries)
	// Reusing the exact key with different content or token cannot replace the admitted notice.
	metadata := preparedSendNowMetadata(stamp.Receipt.Envelope.Token, row.ID)
	err = NewErrandParentDeliverer(f.svc).AdmitPrepared(t.Context(), "session-1", "replacement content", metadata)
	require.ErrorIs(t, err, messagequeue.ErrQueueIDConflict)
	err = NewErrandParentDeliverer(f.svc).AdmitPrepared(t.Context(), "session-1", first[0].Content, preparedSendNowMetadata("different-token", row.ID))
	require.ErrorIs(t, err, messagequeue.ErrQueueIDConflict)
	require.Equal(t, first, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries)
	row, err = f.repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	row.ResultState, row.DeliveredRoute, row.DeliveryToken, row.DeliveredAt = models.ErrandResultPending, "", "", nil
	row.ResultText = "successor revision output"
	require.NoError(t, f.repo.UpdateErrand(t.Context(), row))
	require.NoError(t, service.PreserveCancellationNotice(t.Context(), row.ID))
	entries := f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries
	require.Len(t, entries, 2)
	require.Equal(t, first[0], entries[0])
	require.NotEqual(t, entries[0].ID, entries[1].ID)
	require.NotEqual(t, entries[0].Metadata[errands.DeliveryTokenMetadataKey], entries[1].Metadata[errands.DeliveryTokenMetadataKey])
}

// @covers AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-004.11 AC-TASKS-ERRANDS-012.2
func TestDisabledErrandRetainsCapacityAutoRunAndTerminalCleanup(t *testing.T) {
	for _, terminal := range []string{"task", "parent"} {
		t.Run(terminal, func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			source := f.prepared(t, "prepared-token", "deleted", "complete prepared notice")
			f.restartDelivery(t, false)
			f.svc.messageQueue.SetMaxPerSession(1)
			_, err := f.svc.messageQueue.QueueMessage(t.Context(), "session-1", "task-1", "ordinary", "", messagequeue.QueuedByUser, false, nil)
			require.ErrorIs(t, err, messagequeue.ErrQueueFull, "disabled retained source still consumes capacity")
			dispatched, _ := f.attemptFIFO(t)
			require.False(t, dispatched, "Auto-run OFF must be preserved")
			require.Equal(t, source.ID, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries[0].ID)
			if terminal == "task" {
				_, err = f.svc.messageQueue.PurgeTask(t.Context(), "task-1")
			} else {
				identity, resolveErr := f.svc.parentQueueIdentity(t.Context(), "session-1")
				require.NoError(t, resolveErr)
				session, getErr := f.repo.GetTaskSession(t.Context(), "session-1")
				require.NoError(t, getErr)
				require.NoError(t, f.repo.DeleteTaskSession(t.Context(), session))
				err = f.svc.messageQueue.PurgeDeletedSession(t.Context(), identity)
			}
			require.NoError(t, err)
			_, queue := f.reopen(t)
			require.Empty(t, queue.GetStatus(t.Context(), "session-1").Entries)
		})
	}
}
