package backendapp

import (
	"context"
	"encoding/json"
	"errors"
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

	writer, reader, repo := openErrandTranscriptRepo(t, databasePath)
	session := seedErrandTranscriptSession(t, repo)
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{
		ID: "errand-transcript-message", TaskID: "errand-transcript-task", TaskSessionID: session.ID,
		TurnID: "errand-transcript-turn", AuthorType: models.MessageAuthorUser, Content: "hello",
		Metadata: map[string]interface{}{
			"errand_delivery": errands.DeliveryEnvelope{
				Token: "delivery-token", Route: models.ErrandDeliveryTurnContext,
				Items: []errands.DeliveryItem{{ErrandID: "errand-1", Title: "Review API", Status: models.ErrandStatusDone, Preview: "done"}},
			},
			"errand_send_now": transcriptSendNowMetadata(session),
			orchestrator.ErrandDeliveryTokenMetadataKey:      "delivery-token",
			orchestrator.ErrandDeliverySubmissionMetadataKey: "prompt:errand-transcript-message",
		},
	}))
	_, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: "prompt:errand-transcript-message", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash", Payload: []byte("hello"),
		State: models.DeliverySubmissionDispatching,
	})
	require.NoError(t, err)
	closeErrandTranscriptRepo(t, writer, reader, repo)

	writer, reader, repo = openErrandTranscriptRepo(t, databasePath)
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
	delivered, err = transcript.HasDeliveryToken(ctx, session.ID, "second-token")
	require.NoError(t, err)
	require.True(t, delivered, "every represented source token survives durable reopen")
	delivered, err = transcript.HasDeliveryToken(ctx, session.ID, "unrelated-token")
	require.NoError(t, err)
	require.False(t, delivered)
	_, err = repo.TransitionAgentDeliverySubmission(ctx, "prompt:errand-transcript-message",
		models.DeliverySubmissionDispatching, models.DeliverySubmissionFailed, "rejected", time.Now().UTC())
	require.NoError(t, err)
	transcript = errandTranscript{repo: repo}
	delivered, err = transcript.HasDeliveryToken(ctx, session.ID, "delivery-token")
	require.NoError(t, err)
	require.False(t, delivered, "durable refusal is safe to return to pending after reload")
	closeErrandTranscriptRepo(t, writer, reader, repo)

}

// @covers AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-004.11
func TestErrandTranscriptSubmissionPolicyAfterReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "submission-policy.db")
	writer, reader, repo := openErrandTranscriptRepo(t, path)
	session := seedErrandTranscriptSession(t, repo)
	cases := []struct {
		name  string
		state models.DeliverySubmissionState
		want  bool
	}{
		{"prepared", models.DeliverySubmissionPrepared, true},
		{"accepted", models.DeliverySubmissionAccepted, true},
		{"dispatching", models.DeliverySubmissionDispatching, true},
		{"completed", models.DeliverySubmissionCompleted, true},
		{"interrupted_unknown", models.DeliverySubmissionInterruptedUnknown, true},
		{"failed", models.DeliverySubmissionFailed, false},
		{"cancelled", models.DeliverySubmissionCancelled, false},
		{"no_submission_id", "", true},
		{"missing_submission", "", true},
	}
	for _, format := range []string{"legacy", "structured"} {
		for _, tc := range cases {
			id := format + "-" + tc.name
			metadata := map[string]interface{}{orchestrator.ErrandDeliveryTokenMetadataKey: id + "-A"}
			if format == "structured" {
				snapshot := transcriptSendNowMetadata(session)
				snapshot["receipts"] = []map[string]interface{}{
					transcriptSendNowReceipt("source-A", id+"-A", id+"-errand-A", id+"-revision-A"),
					transcriptSendNowReceipt("source-B", id+"-B", id+"-errand-B", id+"-revision-B"),
				}
				metadata[errands.SendNowDeliveryMetadataKey] = snapshot
			}
			if tc.name != "no_submission_id" {
				metadata[orchestrator.ErrandDeliverySubmissionMetadataKey] = "prompt:" + id
			}
			if tc.state != "" {
				_, err := repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{
					ID: "prompt:" + id, SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
					HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash-" + id, Payload: []byte("result A\n\nresult B"), State: tc.state,
				})
				require.NoError(t, err)
			}
			require.NoError(t, repo.CreateMessage(t.Context(), &models.Message{
				ID: id, TaskID: session.TaskID, TaskSessionID: session.ID, TurnID: "errand-transcript-turn",
				AuthorType: models.MessageAuthorUser, Content: "result A\n\nresult B", Metadata: metadata,
			}))
		}
	}
	closeErrandTranscriptRepo(t, writer, reader, repo)
	_, _, repo = openErrandTranscriptRepo(t, path)
	transcript := errandTranscript{repo: repo}
	for _, format := range []string{"legacy", "structured"} {
		for _, tc := range cases {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				tokens := []string{format + "-" + tc.name + "-A"}
				if format == "structured" {
					tokens = append(tokens, format+"-"+tc.name+"-B")
				}
				for _, token := range tokens {
					got, err := transcript.HasDeliveryToken(t.Context(), session.ID, token)
					require.NoError(t, err)
					require.Equal(t, tc.want, got, token)
				}
			})
		}
	}
	got, err := transcript.HasDeliveryToken(t.Context(), session.ID, "unrelated")
	require.NoError(t, err)
	require.False(t, got)
}

func TestErrandTranscriptRejectsStructuredContradictionsAfterReload(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]interface{})
	}{
		{"unsupported", func(m map[string]interface{}) { m["version"] = 2 }},
		{"missing receipt identities", func(m map[string]interface{}) { delete(m, "receipts") }},
		{"wrong session", func(m map[string]interface{}) { m["session_id"] = "another-session" }},
		{"wrong task", func(m map[string]interface{}) { m["task_id"] = "another-task" }},
		{"wrong incarnation", func(m map[string]interface{}) { m["session_incarnation_id"] = "another-incarnation" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid-receipt.db")
			writer, reader, repo := openErrandTranscriptRepo(t, path)
			session := seedErrandTranscriptSession(t, repo)
			snapshot := transcriptSendNowMetadata(session)
			tc.mutate(snapshot)
			require.NoError(t, repo.CreateMessage(t.Context(), &models.Message{
				ID: "invalid", TaskID: session.TaskID, TaskSessionID: session.ID, TurnID: "errand-transcript-turn",
				AuthorType: models.MessageAuthorUser, Content: "result A\n\nresult B",
				Metadata: map[string]interface{}{
					errands.SendNowDeliveryMetadataKey: snapshot, errands.DeliveryTokenMetadataKey: "delivery-token",
					orchestrator.ErrandDeliverySubmissionMetadataKey: "prompt:invalid",
				},
			}))
			_, err := repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{
				ID: "prompt:invalid", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
				HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash", Payload: []byte("result A\n\nresult B"), State: models.DeliverySubmissionAccepted,
			})
			require.NoError(t, err)
			closeErrandTranscriptRepo(t, writer, reader, repo)
			_, _, repo = openErrandTranscriptRepo(t, path)
			for _, token := range []string{"delivery-token", "second-token", "unrelated"} {
				got, err := (errandTranscript{repo: repo}).HasDeliveryToken(t.Context(), session.ID, token)
				if tc.name == "wrong incarnation" && token == "unrelated" {
					require.NoError(t, err, "unrepresented tokens do not read a submission")
				} else {
					require.ErrorIs(t, err, errands.ErrInvalidSendNowDelivery)
				}
				require.False(t, got, "corruption cannot become scalar or unrelated evidence")
			}
		})
	}
}

func TestErrandTranscriptReadErrorsAndUserContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "read-errors.db")
	writer, reader, repo := openErrandTranscriptRepo(t, path)
	session := seedErrandTranscriptSession(t, repo)
	snapshot := transcriptSendNowMetadata(session)
	wire, err := json.Marshal(map[string]interface{}{errands.SendNowDeliveryMetadataKey: snapshot})
	require.NoError(t, err)
	require.NoError(t, repo.CreateMessage(t.Context(), &models.Message{
		ID: "lookalike", TaskID: session.TaskID, TaskSessionID: session.ID, TurnID: "errand-transcript-turn",
		AuthorType: models.MessageAuthorUser, Content: string(wire),
	}))
	closeErrandTranscriptRepo(t, writer, reader, repo)
	_, _, repo = openErrandTranscriptRepo(t, path)
	transcript := errandTranscript{repo: repo}
	for _, token := range []string{"", "delivery-token", "second-token"} {
		got, err := transcript.HasDeliveryToken(t.Context(), session.ID, token)
		require.NoError(t, err)
		require.False(t, got, "user content does not establish delivery ownership")
	}
	require.NoError(t, repo.CreateMessage(t.Context(), &models.Message{
		ID: "receipt", TaskID: session.TaskID, TaskSessionID: session.ID, TurnID: "errand-transcript-turn",
		AuthorType: models.MessageAuthorUser, Content: "result A\n\nresult B",
		Metadata: map[string]interface{}{
			errands.SendNowDeliveryMetadataKey:               snapshot,
			orchestrator.ErrandDeliverySubmissionMetadataKey: "prompt:receipt",
		},
	}))
	readErr := errors.New("storage unavailable")
	for _, fault := range []errandTranscriptReadFault{
		{errandTranscriptRepo: repo, listErr: readErr},
		{errandTranscriptRepo: repo, submissionErr: readErr},
	} {
		got, err := (errandTranscript{repo: fault}).HasDeliveryToken(t.Context(), session.ID, "second-token")
		require.ErrorIs(t, err, readErr)
		require.False(t, got)
	}
	got, err := transcript.HasDeliveryToken(t.Context(), session.ID, "second-token")
	require.NoError(t, err)
	require.True(t, got, "real missing submission remains uncertain")
}

type errandTranscriptReadFault struct {
	errandTranscriptRepo
	listErr       error
	submissionErr error
}

func (r errandTranscriptReadFault) ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.errandTranscriptRepo.ListMessages(ctx, sessionID)
}

func (r errandTranscriptReadFault) GetAgentDeliverySubmission(ctx context.Context, id string) (*models.AgentDeliverySubmission, error) {
	if r.submissionErr != nil {
		return nil, r.submissionErr
	}
	return r.errandTranscriptRepo.GetAgentDeliverySubmission(ctx, id)
}

// The internal receipt shape is persisted as JSON, just like the dispatch producer.
func transcriptSendNowMetadata(session *models.TaskSession) map[string]interface{} {
	return map[string]interface{}{
		"version": 1, "task_id": session.TaskID, "session_id": session.ID,
		"session_incarnation_id": session.QueueIncarnationID,
		"operation_generation":   0, "queue_generation": 0,
		"source_ids": []string{"source-A", "source-B"},
		"sources": []map[string]interface{}{
			{"id": "source-A", "queue_generation": 0, "kind": "prepared", "queued_by": "server", "provenance": "errands", "content": "", "prepared_block": "result A"},
			{"id": "source-B", "queue_generation": 0, "kind": "prepared", "queued_by": "server", "provenance": "errands", "content": "", "prepared_block": "result B"},
		},
		"effective_prompt": "result A\n\nresult B",
		"receipts": []map[string]interface{}{
			transcriptSendNowReceipt("source-A", "delivery-token", "errand-1", "revision-A"),
			transcriptSendNowReceipt("source-B", "second-token", "errand-2", "revision-B"),
		},
	}
}

func transcriptSendNowReceipt(sourceID, token, errandID, revision string) map[string]interface{} {
	return map[string]interface{}{
		"source_id": sourceID,
		"envelope": errands.DeliveryEnvelope{Token: token, Route: models.ErrandDeliveryNotification,
			Items: []errands.DeliveryItem{{ErrandID: errandID, Title: "Review API", Status: models.ErrandStatusCancelled, Preview: "cancelled"}}},
		"claims": []models.ErrandResultClaim{{ErrandID: errandID, ResultRevision: revision,
			ParentAdmissionGeneration: 0, Token: token, Route: models.ErrandDeliveryNotification}},
	}
}

func openErrandTranscriptRepo(t *testing.T, databasePath string) (*sqlx.DB, *sqlx.DB, *sqlitetaskrepo.Repository) {
	t.Helper()
	writerRaw, err := db.OpenSQLite(databasePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, writerRaw.Close()) })
	readerRaw, err := db.OpenSQLiteReader(databasePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, readerRaw.Close()) })
	writer := sqlx.NewDb(writerRaw, "sqlite3")
	reader := sqlx.NewDb(readerRaw, "sqlite3")
	repo, _, err := taskrepo.Provide(writer, reader, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, repo.Close()) })
	return writer, reader, repo
}
func closeErrandTranscriptRepo(t *testing.T, writer, reader *sqlx.DB, repo *sqlitetaskrepo.Repository) {
	t.Helper()
	require.NoError(t, repo.Close())
	require.NoError(t, reader.Close())
	require.NoError(t, writer.Close())
}

func seedErrandTranscriptSession(t *testing.T, repo *sqlitetaskrepo.Repository) *models.TaskSession {
	t.Helper()
	ctx := t.Context()
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
	return session
}
