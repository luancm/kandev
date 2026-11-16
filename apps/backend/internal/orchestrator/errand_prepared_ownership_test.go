package orchestrator

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-003.5 AC-TASKS-ERRANDS-004.9
func TestPreparedErrandCancellationRetainedBeforeDeletionDeliversRevisionOnce(t *testing.T) {
	f := newSendNowErrandFixture(t)
	service := f.restartDelivery(t, true)
	row := f.result(t, "retained-before-delete")
	require.NoError(t, service.PreserveCancellationNotice(t.Context(), row.ID))
	source := f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries
	require.Len(t, source, 1)
	// The process can stop after durable admission but before deleting the row.
	f.restartDelivery(t, true)
	require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", true))
	prompt := f.completeFIFO(t)
	require.Equal(t, 1, strings.Count(prompt, row.ResultText), "the prepared source and retained errand must not deliver the same cancellation revision twice")
	require.Contains(t, prompt, source[0].Content)
}
