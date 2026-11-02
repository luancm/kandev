package messagequeue

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-004.9
func TestSendNowRestoreNormalizationPreservesOriginalSourcesAndSuccessorFences(t *testing.T) {
	for _, factory := range []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{"memory", func(*testing.T) Repository { return NewMemoryRepository() }}, {"sqlite", newTestSQLiteRepo}, {"postgres", newTestPostgresRepo},
	} {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			service := NewService(repo, 10, logger.Default())
			source := insertTestEntry(t, repo, "session-1", "task-1", "complete original prepared block", QueuedByServer, nil, map[string]interface{}{"legacy": "kept"})
			ordinary := insertTestEntry(t, repo, "session-1", "task-1", "  literal <kandev-system>ordinary input</kandev-system> ", QueuedByUser,
				[]MessageAttachment{{Type: "image", Name: "diagram.png", MimeType: "image/png", Data: "eA=="}}, nil)
			entries, err := repo.ListBySession(t.Context(), "session-1")
			require.NoError(t, err)
			claim, err := service.ClaimSendNow(t.Context(), "session-1", entries)
			require.NoError(t, err)
			original, err := json.Marshal(claim.Sources)
			require.NoError(t, err)
			normalizedCtx, err := WithRestoredSourceMetadata(t.Context(), claim, map[string]map[string]interface{}{source.ID: {"proved_normalization": "exact revision"}})
			require.NoError(t, err)
			require.NoError(t, service.RestoreSendNowClaim(normalizedCtx, claim))
			unchanged, err := json.Marshal(claim.Sources)
			require.NoError(t, err)
			require.JSONEq(t, string(original), string(unchanged), "normalization must not mutate the original dispatch claim")
			restored := service.GetStatus(t.Context(), "session-1").Entries
			require.Len(t, restored, 2)
			require.Equal(t, source.ID, restored[0].ID)
			require.Equal(t, source.Content, restored[0].Content)
			require.Equal(t, "kept", restored[0].Metadata["legacy"])
			require.Equal(t, "exact revision", restored[0].Metadata["proved_normalization"])
			require.Equal(t, ordinary.ID, restored[1].ID)
			require.Equal(t, ordinary.Content, restored[1].Content)
			require.Equal(t, ordinary.Attachments, restored[1].Attachments)
			successor, err := service.ClaimSendNow(t.Context(), "session-1", restored)
			require.NoError(t, err)
			require.ErrorIs(t, service.RestoreSendNowClaim(normalizedCtx, successor), ErrSendNowClaimChanged, "old normalization cannot affect a successor claim")
			require.Empty(t, service.GetStatus(t.Context(), "session-1").Entries)
			require.NoError(t, service.RestoreSendNowClaim(t.Context(), successor))
			require.Equal(t, restored, service.GetStatus(t.Context(), "session-1").Entries)
		})
	}
}
