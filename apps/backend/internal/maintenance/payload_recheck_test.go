package maintenance

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExecutePreservesSharedPayloadReferences(t *testing.T) {
	f := newTestFixture(t)
	ctx := context.Background()
	f.seedTask("payload-task")
	f.seedSession("payload-task", "parent")
	digest := f.seedOrphanedMessagePayload("payload-task", "parent", "parent-turn")
	candidates, err := f.repo.ListOrphanedMessagePayloadCandidates(ctx, 0)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, digest, candidates[0].Digest)
	metadata := shellOutputMetadataFixture(strings.Repeat("o", 5000))
	ordinary := &models.Message{ID: "ordinary", TaskSessionID: "parent", TurnID: "parent-turn", Type: models.MessageTypeToolCall, Metadata: metadata}
	require.NoError(t, f.repo.CreateMessage(ctx, ordinary))
	require.Equal(t, digest, ordinary.PayloadDigest)
	now := time.Now().UTC()
	errand := &models.Errand{ID: "errand", TaskID: "payload-task", ParentSessionID: "parent", Title: "title", Objective: "objective", AgentProfileID: "profile", FirstTurnID: "errand-turn", SubmissionID: "submission", Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, f.repo.CreateErrand(ctx, errand))
	require.NoError(t, f.repo.CreateErrandTurn(ctx, errand.ID, &models.Turn{ID: "errand-turn", TaskID: "payload-task"}))
	errandMessage := &models.Message{ID: "errand-message", TurnID: "errand-turn", TaskID: "payload-task", Type: models.MessageTypeToolCall, Metadata: shellOutputMetadataFixture(strings.Repeat("o", 5000))}
	require.NoError(t, f.repo.CreateErrandMessage(ctx, errand.ID, errandMessage))
	require.Equal(t, digest, errandMessage.PayloadDigest)
	candidates, err = f.repo.ListOrphanedMessagePayloadCandidates(ctx, 0)
	require.NoError(t, err)
	require.Empty(t, candidates)
	homeDir := t.TempDir()
	result, err := Run(ctx, homeDir, "sqlite", f.dbPath, RunOptions{Execute: true}, nil)
	require.NoError(t, err)
	require.Zero(t, result.Execution.DeletedMessagePayloads)
	_, err = f.repo.DB().ExecContext(ctx, `DELETE FROM task_session_messages WHERE id=?`, ordinary.ID)
	require.NoError(t, err)
	candidates, err = f.repo.ListOrphanedMessagePayloadCandidates(ctx, 0)
	require.NoError(t, err)
	require.Empty(t, candidates)
	result, err = Run(ctx, homeDir, "sqlite", f.dbPath, RunOptions{Execute: true}, nil)
	require.NoError(t, err)
	require.Zero(t, result.Execution.DeletedMessagePayloads)
	_, err = f.repo.DB().ExecContext(ctx, `DELETE FROM errand_messages WHERE id=?`, errandMessage.ID)
	require.NoError(t, err)
	candidates, err = f.repo.ListOrphanedMessagePayloadCandidates(ctx, 0)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, digest, candidates[0].Digest)
	result, err = Run(ctx, homeDir, "sqlite", f.dbPath, RunOptions{Execute: true}, nil)
	require.NoError(t, err)
	require.Equal(t, 1, result.Execution.DeletedMessagePayloads)
}

func TestDryRunWithMainOriginPayloadSchema(t *testing.T) {
	f := newTestFixture(t)
	f.seedTask("main-origin")
	f.seedSession("main-origin", "main-session")
	f.seedOrphanedMessagePayload("main-origin", "main-session", "main-turn")
	_, err := f.repo.DB().Exec(`DROP TABLE errand_message_shell_output_chunks; DROP TABLE errand_message_shell_output; DROP TABLE errand_messages`)
	require.NoError(t, err)
	_, err = Run(context.Background(), t.TempDir(), "sqlite", f.dbPath, RunOptions{}, nil)
	require.NoError(t, err)
}
