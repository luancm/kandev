package journal

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/errandawait"
	"github.com/stretchr/testify/require"
)

type awaitAbortJournal interface {
	RecordAwaitAbort(context.Context, errandawait.Identity) error
	PendingAwaitAborts(context.Context) ([]errandawait.Identity, error)
	ConfirmAwaitAbort(context.Context, errandawait.Identity) error
}

func TestAwaitAbortJournalSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	ctx := context.Background()
	j, err := Open(Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = j.Close() })
	store, ok := any(j).(awaitAbortJournal)
	require.True(t, ok, "error disposition needs durable abort evidence")
	id := errandawait.Identity{TaskID: "task", ParentSessionID: "parent", Token: "token", ParentIncarnationID: "owner", HarnessGeneration: 2, TurnID: "origin", SubmissionID: "submission"}
	require.NoError(t, store.RecordAwaitAbort(ctx, id))
	require.NoError(t, store.RecordAwaitAbort(ctx, id))
	bad := id
	bad.HarnessGeneration++
	require.Error(t, store.RecordAwaitAbort(ctx, bad))
	require.Error(t, store.ConfirmAwaitAbort(ctx, bad))
	require.NoError(t, j.Close())
	j, err = Open(Config{Path: path})
	require.NoError(t, err)
	store = any(j).(awaitAbortJournal)
	pending, err := store.PendingAwaitAborts(ctx)
	require.NoError(t, err)
	require.Equal(t, []errandawait.Identity{id}, pending)
	require.NoError(t, store.ConfirmAwaitAbort(ctx, id))
	pending, err = store.PendingAwaitAborts(ctx)
	require.NoError(t, err)
	require.Empty(t, pending)
	require.NoError(t, j.Close())
	require.Error(t, store.RecordAwaitAbort(ctx, id))
	_, err = store.PendingAwaitAborts(ctx)
	require.Error(t, err)
}

func TestAwaitAbortJournalFailureDoesNotMintEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "full.bbolt")
	ctx := t.Context()
	j, err := Open(Config{Path: path, MaxJournalBytes: 1, ReserveBytes: 1})
	require.NoError(t, err)
	t.Cleanup(func() { _ = j.Close() })
	id := errandawait.Identity{TaskID: "task", ParentSessionID: "parent", Token: "error", ParentIncarnationID: "owner", HarnessGeneration: 1}
	require.ErrorIs(t, j.RecordAwaitAbort(ctx, id), ErrJournalFull)
	pending, err := j.PendingAwaitAborts(ctx)
	require.NoError(t, err)
	require.Empty(t, pending)
	require.NoError(t, j.Close())
	j, err = Open(Config{Path: path})
	require.NoError(t, err)
	pending, err = j.PendingAwaitAborts(ctx)
	require.NoError(t, err)
	require.Empty(t, pending)
}
