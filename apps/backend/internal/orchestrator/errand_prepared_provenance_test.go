package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-012.2
func TestPreparedErrandSourceRequiresCompleteConsistentProvenance(t *testing.T) {
	for _, kind := range []string{"complete", "source_id_mismatch", "missing_revision", "missing_claims", "parent_identity_mismatch", "block_mismatch"} {
		t.Run(kind, func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			row := f.result(t, "notice")
			metadata := preparedSendNowMetadata("token", row.ID)
			block := "full prepared block beyond preview"
			stamp, err := f.svc.capturePreparedErrandSource(t.Context(), "task-1", "session-1", block, metadata)
			require.NoError(t, err)
			switch kind {
			case "source_id_mismatch":
				stamp.Receipt.SourceID = "unrelated-source"
			case "missing_revision":
				stamp.Receipt.Claims[0].ResultRevision = ""
			case "missing_claims":
				stamp.Receipt.Claims = nil
			case "parent_identity_mismatch":
				stamp.Identity.SessionIncarnationID = "unrelated-incarnation"
			case "block_mismatch":
				stamp.PreparedBlock = "unrelated prepared content"
			}
			metadata[errandSourceMetadataKey] = stamp
			source := f.queue(t, block, messagequeue.QueuedByServer, metadata)
			require.NoError(t, f.repo.DeleteErrand(t.Context(), row.ID))
			f.restartDelivery(t, true)
			require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", true))
			dispatched, prompt := f.attemptFIFO(t)
			if kind == "complete" {
				require.True(t, dispatched)
				require.Equal(t, block, prompt)
				require.Empty(t, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries)
			} else {
				require.False(t, dispatched, "contradictory ownership cannot authorize provider I/O")
				retained := f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries
				require.Len(t, retained, 1)
				require.Equal(t, source.ID, retained[0].ID)
				require.Equal(t, source.Content, retained[0].Content)
			}
		})
	}
}
