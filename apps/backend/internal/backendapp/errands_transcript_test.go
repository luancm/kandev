package backendapp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	sqlitetaskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestErrandTranscriptReadsDeliverySnapshotAndSubmissionAfterReload(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "errand-delivery.db")
	openRepo := func() (*sqlx.DB, *sqlx.DB, *sqlitetaskrepo.Repository) {
		writerRaw, err := db.OpenSQLite(databasePath)
		require.NoError(t, err)
		readerRaw, err := db.OpenSQLiteReader(databasePath)
		require.NoError(t, err)
		writer := sqlx.NewDb(writerRaw, "sqlite3")
		reader := sqlx.NewDb(readerRaw, "sqlite3")
		repo, _, err := taskrepo.Provide(writer, reader, nil)
		require.NoError(t, err)
		return writer, reader, repo
	}
	closeRepo := func(writer, reader *sqlx.DB, repo *sqlitetaskrepo.Repository) {
		require.NoError(t, repo.Close())
		require.NoError(t, reader.Close())
		require.NoError(t, writer.Close())
	}

	writer, reader, repo := openRepo()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "errand-transcript-workspace", Name: "Workspace"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "errand-transcript-task", WorkspaceID: "errand-transcript-workspace", Title: "Task",
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "errand-transcript-session", TaskID: "errand-transcript-task"}))
	session, err := repo.GetTaskSession(ctx, "errand-transcript-session")
	require.NoError(t, err)
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "errand-transcript-turn", TaskID: "errand-transcript-task", TaskSessionID: session.ID,
	}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{
		ID: "errand-transcript-message", TaskID: "errand-transcript-task", TaskSessionID: session.ID,
		TurnID: "errand-transcript-turn", AuthorType: models.MessageAuthorUser, Content: "hello",
		Metadata: map[string]interface{}{
			"errand_delivery": errands.DeliveryEnvelope{
				Token: "delivery-token", Route: models.ErrandDeliveryTurnContext,
				Items: []errands.DeliveryItem{{ErrandID: "errand-1", Title: "Review API", Status: models.ErrandStatusDone, Preview: "done"}},
			},
			orchestrator.ErrandDeliveryTokenMetadataKey:      "delivery-token",
			orchestrator.ErrandDeliverySubmissionMetadataKey: "prompt:errand-transcript-message",
		},
	}))
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: "prompt:errand-transcript-message", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash", Payload: []byte("hello"),
		State: models.DeliverySubmissionDispatching,
	})
	require.NoError(t, err)
	closeRepo(writer, reader, repo)

	writer, reader, repo = openRepo()
	message, err := repo.GetMessage(ctx, "errand-transcript-message")
	require.NoError(t, err)
	require.Equal(t, "hello", message.Content)
	require.Equal(t, "delivery-token", message.Metadata[orchestrator.ErrandDeliveryTokenMetadataKey])
	wire, err := json.Marshal(message.Metadata["errand_delivery"])
	require.NoError(t, err)
	require.JSONEq(t, `{"token":"delivery-token","route":"turn_context","items":[{"errand_id":"errand-1","title":"Review API","status":"done","preview":"done"}]}`, string(wire))
	transcript := errandTranscript{repo: repo}
	delivered, err := transcript.HasDeliveryToken(ctx, session.ID, "delivery-token")
	require.NoError(t, err)
	require.True(t, delivered, "an in-flight durable submission is preserved as accepted or uncertain")
	_, err = repo.TransitionAgentDeliverySubmission(ctx, "prompt:errand-transcript-message",
		models.DeliverySubmissionDispatching, models.DeliverySubmissionFailed, "rejected", time.Now().UTC())
	require.NoError(t, err)
	transcript = errandTranscript{repo: repo}
	delivered, err = transcript.HasDeliveryToken(ctx, session.ID, "delivery-token")
	require.NoError(t, err)
	require.False(t, delivered, "durable refusal is safe to return to pending after reload")
	closeRepo(writer, reader, repo)

}
