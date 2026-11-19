package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestOwnedErrandBrowserUnread(t *testing.T) {
	exerciseOwnedErrandBrowserUnread(t, newRepoForSessionTests(t))
}

func TestPostgresOwnedErrandBrowserUnread(t *testing.T) {
	exerciseOwnedErrandBrowserUnread(t, newPostgresErrandRepo(t))
}

func exerciseOwnedErrandBrowserUnread(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	taskID := newErrandTask(t, repo, "owned-browser-unread")
	const parentID = "owned-unread-parent"
	const errandID = "owned-unread-errand"
	item := sampleErrand(errandID, taskID, parentID, errandScenarioBase)
	markErrandFinished(item)
	mustCreateErrand(t, repo, item)
	turn := &models.Turn{ID: item.FirstTurnID}
	require.NoError(t, repo.CreateErrandTurn(ctx, errandID, turn))
	createdAt := errandScenarioBase.Add(time.Second + 123456100*time.Nanosecond)

	for _, message := range []*models.Message{
		{ID: "agent-output", AuthorType: models.MessageAuthorAgent, Content: "completed"},
		{ID: "user-output", AuthorType: models.MessageAuthorUser, Content: "follow up"},
		{ID: "setup-output", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeScriptExecution,
			Metadata: map[string]any{"script_type": "setup"}},
		{ID: "internal-output", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeStatus,
			Metadata: map[string]any{"recovery_actions": true}},
	} {
		message.TurnID = turn.ID
		message.CreatedAt = createdAt
		require.NoError(t, repo.CreateErrandMessage(ctx, errandID, message))
	}
	older := &models.Message{
		ID: "0-earlier-output", TurnID: turn.ID, AuthorType: models.MessageAuthorAgent,
		Content: "earlier", CreatedAt: createdAt.Add(800 * time.Nanosecond),
	}
	require.NoError(t, repo.CreateErrandMessage(ctx, errandID, older))
	assertOwnedErrandUnread(t, repo, taskID, parentID, 1)
	changed, err := repo.AdvanceErrandReadCursor(ctx, taskID, errandID, "agent-output")
	require.NoError(t, err)
	require.True(t, changed)
	assertOwnedErrandUnread(t, repo, taskID, parentID, 0)
	changed, err = repo.AdvanceErrandReadCursor(ctx, taskID, errandID, older.ID)
	require.NoError(t, err)
	require.False(t, changed, "an older byte-ordered ID in the same microsecond cannot move the cursor")
	changed, err = repo.AdvanceErrandReadCursor(ctx, "outside-task", errandID, "user-output")
	require.ErrorIs(t, err, models.ErrErrandNotFound)
	require.False(t, changed)
	changed, err = repo.AdvanceErrandReadCursor(ctx, taskID, errandID, "unknown-message")
	require.ErrorIs(t, err, models.ErrErrandNotFound)
	require.False(t, changed)

	require.NoError(t, repo.CreateErrandMessage(ctx, errandID, &models.Message{
		ID: "later-output", TurnID: turn.ID, AuthorType: models.MessageAuthorAgent,
		Content: "new output", CreatedAt: createdAt.Add(time.Microsecond),
	}))
	assertOwnedErrandUnread(t, repo, taskID, parentID, 1)
}

func assertOwnedErrandUnread(t *testing.T, repo *Repository, taskID, parentID string, unread int) {
	t.Helper()
	ctx := t.Context()
	summary, err := repo.GetErrandBrowserSummary(ctx, taskID)
	require.NoError(t, err)
	require.Empty(t, summary.Errands, "finished Errand is outside the initial summary")
	require.Equal(t, unread, summary.Counts.Unread)
	require.Len(t, summary.ParentCounts, 1)
	require.Equal(t, unread, summary.ParentCounts[0].Counts.Unread)
	page, err := repo.GetErrandHistoryPage(ctx, models.ErrandHistoryPageRequest{
		TaskID: taskID, ParentSessionID: parentID, Limit: 1,
	})
	require.NoError(t, err)
	require.Len(t, page.Errands, 1)
	require.Equal(t, unread != 0, page.Errands[0].Unread)
}
