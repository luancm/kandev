package messagequeue

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func openSendNowSnapshotRepo(t *testing.T, path string) *sqliteRepository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(1)
	repo, err := NewSQLiteRepository(db, db)
	require.NoError(t, err)
	return repo.(*sqliteRepository)
}

func sendNowSnapshotClaim(t *testing.T, repo *sqliteRepository) *SendNowClaim {
	t.Helper()
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "parent", SessionIncarnationID: "incarnation"}
	seedQueueSessionIdentity(t, repo, identity)
	sources := []QueuedMessage{
		{ID: "one", SessionID: "parent", TaskID: "task", Content: "  ordinary one\n ", QueuedBy: QueuedByUser},
		{ID: "two", SessionID: "parent", TaskID: "task", Content: "ordinary two", QueuedBy: QueuedByUser, PlanMode: true},
	}
	for i := range sources {
		require.NoError(t, repo.InsertForSession(t.Context(), identity, &sources[i], DefaultMaxPerSession))
	}
	claim, err := repo.ClaimSendNowForSession(t.Context(), identity, sources)
	require.NoError(t, err)
	return claim
}

func enrichSendNowSnapshot(claim *SendNowClaim) {
	sources := make([]map[string]interface{}, 0, len(claim.Sources))
	ids := make([]string, 0, len(claim.Sources))
	for _, source := range claim.Sources {
		ids = append(ids, source.ID)
		sources = append(sources, map[string]interface{}{"id": source.ID, "kind": "ordinary", "queued_by": source.QueuedBy, "content": source.Content, "queue_generation": claim.SourceGenerations[source.TaskID]})
	}
	claim.Dispatch.Metadata["errand_send_now"] = map[string]interface{}{
		"version": 1, "task_id": claim.Identity.TaskID, "session_id": claim.Identity.SessionID, "session_incarnation_id": claim.Identity.SessionIncarnationID,
		"queue_generation": claim.SessionGeneration, "operation_generation": claim.OperationGeneration, "sources": sources, "source_ids": ids,
		"receipts": []interface{}{}, "effective_prompt": "canonical provider prompt\n" + claim.Dispatch.Content}
}

func TestSQLiteSendNowDeliverySnapshotReopensAndRestoresOriginalSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.db")
	repo := openSendNowSnapshotRepo(t, path)
	claim := sendNowSnapshotClaim(t, repo)
	original, err := json.Marshal(claim.Sources)
	require.NoError(t, err)
	enrichSendNowSnapshot(claim)
	require.NoError(t, repo.SetPendingSendNowClaimDelivery(t.Context(), claim, DeliveryProtocolLegacy, "stable-submission", ""))
	expected, err := json.Marshal(claim.Dispatch)
	require.NoError(t, err)
	require.NoError(t, repo.db.Close())
	reopened := openSendNowSnapshotRepo(t, path)
	pending, err := reopened.ListPendingSendNowClaims(t.Context())
	require.NoError(t, err)
	require.Len(t, pending, 1)
	actual, err := json.Marshal(pending[0].Claim.Dispatch)
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(actual))
	retained, err := json.Marshal(pending[0].Claim.Sources)
	require.NoError(t, err)
	require.JSONEq(t, string(original), string(retained))
	snapshot := pending[0].Claim.Dispatch.Metadata["errand_send_now"].(map[string]interface{})
	require.Equal(t, []interface{}{"one", "two"}, snapshot["source_ids"])
	require.Equal(t, "canonical provider prompt\n  ordinary one\n \n\nordinary two", snapshot["effective_prompt"])
	require.NoError(t, reopened.RestoreSendNowClaim(t.Context(), &pending[0].Claim))
	restored, err := reopened.ListBySession(t.Context(), "parent")
	require.NoError(t, err)
	require.Len(t, restored, 2)
	require.Equal(t, "one", restored[0].ID)
	require.Equal(t, "  ordinary one\n ", restored[0].Content)
	require.Equal(t, "two", restored[1].ID)
	require.True(t, restored[1].PlanMode)
}

func TestSQLiteSendNowDeliverySnapshotRejectsChangedRestoreAuthority(t *testing.T) {
	for _, change := range []string{"source", "source_generation", "operation_generation", "incarnation"} {
		t.Run(change, func(t *testing.T) {
			repo := openSendNowSnapshotRepo(t, filepath.Join(t.TempDir(), "queue.db"))
			claim := sendNowSnapshotClaim(t, repo)
			enrichSendNowSnapshot(claim)
			before, err := repo.ListPendingSendNowClaims(t.Context())
			require.NoError(t, err)
			switch change {
			case "source":
				claim.Sources[0].Content = "replacement source"
			case "source_generation":
				claim.SourceGenerations["task"]++
			case "operation_generation":
				claim.OperationGeneration++
			case "incarnation":
				claim.Identity.SessionIncarnationID = "replacement-incarnation"
			}
			require.Error(t, repo.SetPendingSendNowClaimDelivery(t.Context(), claim, DeliveryProtocolLegacy, "changed", ""))
			after, err := repo.ListPendingSendNowClaims(t.Context())
			require.NoError(t, err)
			require.Equal(t, before, after, "a refused enrichment must leave restore authority intact")
		})
	}
}

func TestSQLiteSendNowDeliverySnapshotOldOwnerCannotEnrichSuccessor(t *testing.T) {
	repo := openSendNowSnapshotRepo(t, filepath.Join(t.TempDir(), "queue.db"))
	old := sendNowSnapshotClaim(t, repo)
	enrichSendNowSnapshot(old)
	require.NoError(t, repo.RestoreSendNowClaim(t.Context(), old))
	sources, err := repo.ListBySession(t.Context(), "parent")
	require.NoError(t, err)
	selected := make([]QueuedMessage, len(sources))
	copy(selected, sources)
	successor, err := repo.ClaimSendNowForSession(t.Context(), old.Identity, selected)
	require.NoError(t, err)
	enrichSendNowSnapshot(successor)
	require.NoError(t, repo.SetPendingSendNowClaimDelivery(t.Context(), successor, DeliveryProtocolLegacy, "successor", ""))
	before, err := repo.ListPendingSendNowClaims(t.Context())
	require.NoError(t, err)
	require.ErrorIs(t, repo.SetPendingSendNowClaimDelivery(context.Background(), old, DeliveryProtocolLegacy, "old", ""), ErrSendNowClaimChanged)
	after, err := repo.ListPendingSendNowClaims(t.Context())
	require.NoError(t, err)
	require.Equal(t, before, after)
}
