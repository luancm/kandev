package process

import (
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestAwaitInvocationCapturesOriginalSubmission(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "journal.bbolt")
	j, err := journal.Open(journal.Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = j.Close() })
	cfg := &config.InstanceConfig{TaskID: "task", SessionID: "parent", DeliveryIncarnationID: "owner", DeliveryHarnessGeneration: 2, DurableJournalPath: path}
	manager := &Manager{cfg: cfg, deliveryJournal: j, deliveryActiveID: "original"}
	_, err = j.PutSubmission(ctx, journal.Submission{ID: "original", SessionID: "parent", IncarnationID: "owner", HarnessGeneration: 2, Hash: "hash", Payload: []byte("prompt")})
	require.NoError(t, err)
	id, err := manager.CaptureAwaitInvocation(ctx, "token")
	require.NoError(t, err)
	require.Equal(t, "original", id.SubmissionID)
	require.Equal(t, "owner", id.ParentIncarnationID)
	manager.deliveryActiveID = "successor"
	require.Equal(t, "original", id.SubmissionID)
	require.NoError(t, manager.RecordAwaitAbort(ctx, id))
	pending, err := manager.PendingAwaitAborts(ctx)
	require.NoError(t, err)
	require.Equal(t, id, pending[0])
	_, err = manager.CaptureAwaitInvocation(ctx, "other")
	require.Error(t, err, "missing exact submission cannot be replaced with latest")
	manager.deliveryActiveID = ""
	unknown, err := manager.CaptureAwaitInvocation(ctx, "unassociated")
	require.NoError(t, err)
	require.Empty(t, unknown.SubmissionID)
	require.Empty(t, unknown.TurnID)
	require.NoError(t, j.Close())
	_, err = manager.CaptureAwaitInvocation(ctx, "closed")
	require.Error(t, err, "failed journal evidence must fail closed")
}

func TestAwaitInvocationUsesUniqueRetainedPromptAssociation(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "journal.bbolt")
	j, err := journal.Open(journal.Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = j.Close() })
	manager := &Manager{cfg: &config.InstanceConfig{TaskID: "task", SessionID: "parent", DeliveryIncarnationID: "owner", DeliveryHarnessGeneration: 2, DurableJournalPath: path}, deliveryJournal: j}
	_, err = j.PutSubmission(ctx, journal.Submission{ID: "origin", SessionID: "parent", IncarnationID: "owner", HarnessGeneration: 2, Hash: "hash", Payload: []byte("prompt")})
	require.NoError(t, err)
	manager.TrackDeliverySubmission("origin", 7)
	id, err := manager.CaptureAwaitInvocation(ctx, "token")
	require.NoError(t, err)
	require.Equal(t, "origin", id.SubmissionID)
	manager.TrackDeliverySubmission("ambiguous", 8)
	id, err = manager.CaptureAwaitInvocation(ctx, "other")
	require.NoError(t, err)
	require.Empty(t, id.SubmissionID, "multiple associations cannot become latest turn")
}
