package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestErrandTranscriptSchemaWithoutSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	for _, table := range []string{"errand_turns", "errand_messages"} {
		exists, err := db.TableExistsContext(context.Background(), repo.db, table)
		if err != nil || !exists {
			t.Fatalf("%s exists = %v, err = %v; Errand transcript must not require task_sessions", table, exists, err)
		}
	}
}

type errandTranscriptStore interface {
	CreateErrandTurn(context.Context, string, *models.Turn) error
	GetErrandTurn(context.Context, string, string) (*models.Turn, error)
	UpdateErrandTurn(context.Context, string, *models.Turn) error
	ListErrandTurns(context.Context, string) ([]*models.Turn, error)
	CreateErrandMessage(context.Context, string, *models.Message) error
	GetErrandMessage(context.Context, string, string) (*models.Message, error)
	UpdateErrandMessage(context.Context, string, *models.Message) error
	ListErrandMessages(context.Context, string, string, int) ([]*models.Message, bool, error)
}

func TestErrandTranscriptRoundTrip(t *testing.T) {
	exerciseErrandTranscript(t, newRepoForSessionTests(t))
}
func TestPostgresErrandTranscriptRoundTrip(t *testing.T) {
	exerciseErrandTranscript(t, newPostgresErrandRepo(t))
}

// The transcript must survive schema replay without creating a normal session.
func exerciseErrandTranscript(t *testing.T, repo *Repository) {
	t.Helper()
	store, ok := any(repo).(errandTranscriptStore)
	require.True(t, ok, "repository must expose separate Errand transcript operations")
	ctx := context.Background()
	taskID := newErrandTask(t, repo, "transcript")
	mustCreateErrand(t, repo, sampleErrand("errand-transcript", taskID, "parent", errandScenarioBase))
	mustCreateErrand(t, repo, sampleErrand("other-transcript", taskID, "parent", errandScenarioBase))
	turn := &models.Turn{ID: "et", TaskID: taskID, Metadata: map[string]interface{}{"runtime": "preserved"}}
	require.NoError(t, store.CreateErrandTurn(ctx, "errand-transcript", turn))
	for _, id := range []string{"a", "b", "c"} {
		msg := &models.Message{ID: id, TaskID: taskID, TurnID: turn.ID, Type: models.MessageTypeToolCall, Content: id, CreatedAt: errandScenarioBase, Metadata: map[string]interface{}{"tool_call_id": "call-" + id, "output": "raw\noutput"}}
		require.NoError(t, store.CreateErrandMessage(ctx, "errand-transcript", msg))
	}
	var sessions int
	require.NoError(t, repo.db.GetContext(ctx, &sessions, repo.db.Rebind(`SELECT COUNT(*) FROM task_sessions WHERE task_id = ?`), taskID))
	require.Zero(t, sessions)
	assertErrandTranscriptPage(t, store)
	assertErrandLargeToolPayload(t, store, taskID, turn.ID)
	_, err := store.GetErrandMessage(ctx, "other-transcript", "a")
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, _, err = store.ListErrandMessages(ctx, "other-transcript", "a", 2)
	require.ErrorIs(t, err, sql.ErrNoRows)
	msg, err := store.GetErrandMessage(ctx, "errand-transcript", "a")
	require.NoError(t, err)
	msg.Content = "changed"
	require.NoError(t, store.UpdateErrandMessage(ctx, "errand-transcript", msg))
	msg.TurnID = "foreign"
	require.Error(t, store.UpdateErrandMessage(ctx, "errand-transcript", msg))
	msg.TurnID = turn.ID
	msg.TaskID = "foreign"
	require.Error(t, store.UpdateErrandMessage(ctx, "errand-transcript", msg))
	foreign := &models.Message{ID: "foreign", TaskID: taskID, TurnID: turn.ID}
	require.Error(t, store.CreateErrandMessage(ctx, "other-transcript", foreign))
	completed := errandScenarioBase.Add(time.Minute)
	turn.CompletedAt = &completed
	require.NoError(t, store.UpdateErrandTurn(ctx, "errand-transcript", turn))
	turns, err := store.ListErrandTurns(ctx, "errand-transcript")
	require.NoError(t, err)
	require.Len(t, turns, 1)
	require.Equal(t, "preserved", turns[0].Metadata["runtime"])
	_, err = NewWithDB(repo.db, repo.ro, nil)
	require.NoError(t, err)
	got, err := store.GetErrandTurn(ctx, "errand-transcript", turn.ID)
	require.NoError(t, err)
	require.True(t, got.CompletedAt.Equal(completed))
	require.Empty(t, got.TaskSessionID)
	require.NoError(t, repo.DeleteErrand(ctx, "errand-transcript"))
	_, err = store.GetErrandMessage(ctx, "errand-transcript", "a")
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = store.GetErrandTurn(ctx, "errand-transcript", turn.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = repo.GetErrand(ctx, "other-transcript")
	require.NoError(t, err)
}

func TestErrandTranscriptSchemaReplayPreservesMainRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	taskID := newErrandTask(t, repo, "main-replay")
	session := &models.TaskSession{ID: "main-replay", TaskID: taskID}
	require.NoError(t, repo.CreateTaskSession(ctx, session))
	turn := &models.Turn{ID: "main-turn", TaskSessionID: session.ID, TaskID: taskID, Metadata: map[string]interface{}{"main": "turn"}}
	require.NoError(t, repo.CreateTurn(ctx, turn))
	message := &models.Message{ID: "main-message", TaskID: taskID, TaskSessionID: session.ID, TurnID: turn.ID, Content: "main conversation", AuthorType: models.MessageAuthorUser, Metadata: map[string]interface{}{"main": "message"}}
	require.NoError(t, repo.CreateMessage(ctx, message))
	beforeTurn, err := repo.GetTurn(ctx, turn.ID)
	require.NoError(t, err)
	beforeMessage, err := repo.GetMessage(ctx, message.ID)
	require.NoError(t, err)
	// Simulate the additive schema being absent, without changing main tables.
	_, err = repo.db.ExecContext(ctx, `DROP TABLE errand_messages; DROP TABLE errand_turns`)
	require.NoError(t, err)
	_, err = NewWithDB(repo.db, repo.ro, nil)
	require.NoError(t, err)
	afterTurn, err := repo.GetTurn(ctx, turn.ID)
	require.NoError(t, err)
	require.Equal(t, beforeTurn, afterTurn)
	afterMessage, err := repo.GetMessage(ctx, message.ID)
	require.NoError(t, err)
	require.Equal(t, beforeMessage, afterMessage)
	var foreignKeys []struct {
		Table string `db:"table"`
	}
	rows, err := repo.db.QueryContext(ctx, `PRAGMA foreign_key_list(task_session_turns)`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, seq int
		var table, from, to, onUpdate, onDelete, match string
		require.NoError(t, rows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match))
		foreignKeys = append(foreignKeys, struct {
			Table string `db:"table"`
		}{table})
	}
	require.NoError(t, rows.Err())
	require.Contains(t, foreignKeys, struct {
		Table string `db:"table"`
	}{"task_sessions"})
}

func assertErrandTranscriptPage(t *testing.T, store errandTranscriptStore) {
	t.Helper()
	ctx := context.Background()
	page, more, err := store.ListErrandMessages(ctx, "errand-transcript", "", 2)
	require.NoError(t, err)
	require.True(t, more)
	require.Equal(t, []string{"b", "c"}, []string{page[0].ID, page[1].ID})
	older, more, err := store.ListErrandMessages(ctx, "errand-transcript", page[0].ID, 2)
	require.NoError(t, err)
	require.False(t, more)
	require.Len(t, older, 1)
	require.Equal(t, "a", older[0].ID)
	require.Equal(t, "raw\noutput", older[0].Metadata["output"])
	require.Equal(t, models.MessageTypeToolCall, older[0].Type)
	require.Equal(t, "a", older[0].Content)
	require.Equal(t, "call-a", older[0].Metadata["tool_call_id"])
}

func assertErrandLargeToolPayload(t *testing.T, store errandTranscriptStore, taskID, turnID string) {
	t.Helper()
	ctx := context.Background()
	output := strings.Repeat("\x00full stdout\n", 1000)
	message := &models.Message{ID: "large-tool", TaskID: taskID, TurnID: turnID, AuthorType: models.MessageAuthorAgent, AuthorID: "execution", Type: models.MessageTypeToolCall, Content: "command", RequestsInput: true, Metadata: shellOutputMetadata(output, "stderr"), CreatedAt: errandScenarioBase.Add(time.Second), UpdatedAt: errandScenarioBase.Add(time.Second)}
	require.NoError(t, store.CreateErrandMessage(ctx, "errand-transcript", message))
	got, err := store.GetErrandMessage(ctx, "errand-transcript", message.ID)
	require.NoError(t, err)
	require.Equal(t, message, got)
	got.Content = "updated command"
	require.NoError(t, store.UpdateErrandMessage(ctx, "errand-transcript", got))
	updated, err := store.GetErrandMessage(ctx, "errand-transcript", message.ID)
	require.NoError(t, err)
	require.Equal(t, got, updated)
	projected := *updated
	projected.ID = "projected"
	projected.PayloadDigest = "main-payload"
	require.Error(t, store.CreateErrandMessage(ctx, "errand-transcript", &projected))
}

func TestErrandTranscriptTimestampNormalization(t *testing.T) {
	exerciseErrandTranscriptTimestamps(t, newRepoForSessionTests(t))
}
func TestPostgresErrandTranscriptTimestampNormalization(t *testing.T) {
	exerciseErrandTranscriptTimestamps(t, newPostgresErrandRepo(t))
}
func exerciseErrandTranscriptTimestamps(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	taskID := newErrandTask(t, repo, "offset")
	mustCreateErrand(t, repo, sampleErrand("offset", taskID, "parent", errandScenarioBase))
	offset := time.Date(2026, time.November, 17, 15, 30, 20, 123456789, time.FixedZone("offset", 5*3600+1800))
	completed := offset.Add(time.Minute)
	turn := &models.Turn{ID: "offset-turn", TaskID: taskID, StartedAt: offset, CreatedAt: offset, CompletedAt: &completed}
	require.NoError(t, repo.CreateErrandTurn(ctx, "offset", turn))
	for _, timestamp := range []time.Time{turn.StartedAt, turn.CreatedAt, turn.UpdatedAt, *turn.CompletedAt} {
		require.Equal(t, timestamp.UTC().Truncate(time.Microsecond), timestamp)
	}
	gotTurn, err := repo.GetErrandTurn(ctx, "offset", turn.ID)
	require.NoError(t, err)
	require.Equal(t, turn, gotTurn)
	nextCompleted := offset.Add(2 * time.Minute)
	turn.CompletedAt = &nextCompleted
	require.NoError(t, repo.UpdateErrandTurn(ctx, "offset", turn))
	gotTurn, err = repo.GetErrandTurn(ctx, "offset", turn.ID)
	require.NoError(t, err)
	require.Equal(t, turn, gotTurn)
	message := &models.Message{ID: "offset-message", TaskID: taskID, TurnID: turn.ID, Content: "offset payload", CreatedAt: offset, UpdatedAt: offset.Add(time.Second)}
	require.NoError(t, repo.CreateErrandMessage(ctx, "offset", message))
	require.Equal(t, message.UpdatedAt.UTC().Truncate(time.Microsecond), message.UpdatedAt)
	gotMessage, err := repo.GetErrandMessage(ctx, "offset", message.ID)
	require.NoError(t, err)
	require.Equal(t, message, gotMessage)
	message.Content = "updated offset payload"
	require.NoError(t, repo.UpdateErrandMessage(ctx, "offset", message))
	gotMessage, err = repo.GetErrandMessage(ctx, "offset", message.ID)
	require.NoError(t, err)
	require.Equal(t, message, gotMessage)
}
