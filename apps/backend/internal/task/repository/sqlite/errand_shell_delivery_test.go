package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestErrandShellStartDelivery(t *testing.T) {
	exerciseErrandShellStartDelivery(t, newRepoForSessionTests(t), true)
}

func TestPostgresErrandPayloadDetailSnapshotSurvivesConcurrentReplacement(t *testing.T) {
	repo := newPostgresErrandRepo(t)
	repo.db.SetMaxOpenConns(2)
	ctx := context.Background()
	task := newErrandTask(t, repo, "shell-snapshot")
	mustCreateErrand(t, repo, sampleErrand("snapshot", task, "parent", errandScenarioBase))
	require.NoError(t, repo.CreateErrandTurn(ctx, "snapshot", &models.Turn{ID: "snapshot-turn", TaskID: task}))
	output := strings.Repeat("snapshot output", 1000)
	message := &models.Message{ID: "snapshot-message", TurnID: "snapshot-turn", TaskID: task, Type: models.MessageTypeToolExecute, Metadata: shellOutputMetadata(output, "")}
	require.NoError(t, repo.CreateErrandMessage(ctx, "snapshot", message))
	require.NotEmpty(t, message.PayloadDigest)
	tx, err := repo.ro.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	snapshot, err := scanErrandMessage(tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+errandMessageColumns+` FROM errand_messages m JOIN task_session_errands e ON e.id=m.errand_id WHERE m.errand_id=? AND m.id=?`), "snapshot", message.ID))
	require.NoError(t, err)
	oldDigest := message.PayloadDigest
	message.PayloadDigest, message.PayloadSize = "", 0
	message.Metadata = shellOutputMetadata("replacement", "")
	require.NoError(t, repo.UpdateErrandMessage(ctx, "snapshot", message))
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`DELETE FROM task_message_payloads WHERE digest=?`), oldDigest)
	require.NoError(t, err)
	require.NoError(t, rehydrateMessagePayload(ctx, tx, snapshot))
	full, ok := models.ExtractShellExecOutput(snapshot.Metadata)
	require.True(t, ok)
	require.Equal(t, output, full.Stdout)
	require.NoError(t, tx.Commit())
}

func TestPostgresErrandShellStartDelivery(t *testing.T) {
	exerciseErrandShellStartDelivery(t, newPostgresErrandRepo(t), false)
}

func exerciseErrandShellStartDelivery(t *testing.T, repo *Repository, injectFailure bool) {
	t.Helper()
	ctx := context.Background()
	task := newErrandTask(t, repo, "shell-delivery")
	mustCreateErrand(t, repo, sampleErrand("shell-errand", task, "parent", errandScenarioBase))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "shell-errand", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "shell-errand", 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &models.ErrandHarnessBinding{ErrandID: "shell-errand", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "shell-stream"}, 0))
	require.NoError(t, repo.CreateErrandTurn(ctx, "shell-errand", &models.Turn{ID: "turn", TaskID: task}))
	sub := &models.AgentDeliverySubmission{ID: "submission", Payload: []byte("prompt"), PayloadHash: journal.SubmissionHash([]byte("prompt"))}
	_, err = repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{ErrandID: "shell-errand", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "shell-stream", PromptGeneration: 1, TurnID: "turn", Submission: sub})
	require.NoError(t, err)
	agent := streams.AgentEvent{Type: streams.EventTypeToolCall, ToolCallID: "tool", ToolTitle: "command", ToolStatus: "pending", TurnID: "turn", PromptGeneration: 1, NormalizedPayload: streams.NewShellExec("printf hello", "/tmp", "", 0, false)}
	payload, err := json.Marshal(agent)
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{SessionID: "errand:shell-errand", TaskID: task, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, OwnerGeneration: 1, ExpectedStreamID: "shell-stream", StreamID: "shell-stream", Sequence: 1, SubmissionID: "submission", EventType: agent.Type, Payload: payload}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	projected, err := repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	require.True(t, projected)
	messages, _, err := repo.ListErrandMessages(ctx, "shell-errand", "", 10)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, models.MessageTypeToolExecute, messages[0].Type)
	require.Empty(t, messages[0].TaskSessionID)
	output := strings.Repeat("hello from shell ", 1024)
	patch, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: output}, "epoch", true)
	require.NoError(t, err)
	agent.Type = streams.EventTypeShellOutputUpdate
	agent.ShellOutput = patch
	payload, err = json.Marshal(agent)
	require.NoError(t, err)
	event.Sequence = 2
	event.EventType = agent.Type
	event.Payload = payload
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 2)
	require.NoError(t, err)
	if injectFailure {
		_, err = repo.db.Exec(`CREATE TRIGGER fail_errand_shell BEFORE UPDATE ON errand_messages BEGIN SELECT RAISE(ABORT,'injected shell failure'); END`)
		require.NoError(t, err)
		_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
		require.Error(t, err)
		var heads int
		require.NoError(t, repo.db.GetContext(ctx, &heads, `SELECT COUNT(*) FROM errand_message_shell_output`))
		require.Zero(t, heads)
		cursor, cursorErr := repo.GetAgentDeliveryCursor(ctx, "shell-stream")
		require.NoError(t, cursorErr)
		require.EqualValues(t, 1, cursor.ProjectedSequence)
		_, err = repo.db.Exec(`DROP TRIGGER fail_errand_shell`)
		require.NoError(t, err)
	}
	projected, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	require.True(t, projected)
	projected, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	require.False(t, projected)
	var verifySnapshot func()
	if !injectFailure {
		repo.db.SetMaxOpenConns(2)
		tx, txErr := repo.ro.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
		require.NoError(t, txErr)
		defer func() { _ = tx.Rollback() }()
		_, txErr = scanErrandMessage(tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+errandMessageColumns+` FROM errand_messages m JOIN task_session_errands e ON e.id=m.errand_id WHERE m.errand_id=? AND m.id=?`), "shell-errand", messages[0].ID))
		require.NoError(t, txErr)
		head, txErr := loadShellOutputHead(ctx, tx, errandShellOutputQueries, messages[0].ID)
		require.NoError(t, txErr)
		oldOutput := output
		verifySnapshot = func() {
			old, readErr := shellOutputStateTx(ctx, tx, errandShellOutputQueries, messages[0].ID, head)
			require.NoError(t, readErr)
			require.Equal(t, oldOutput, old.Output.Stdout)
			require.NoError(t, tx.Commit())
		}
	}
	if injectFailure {
		_, err = repo.db.Exec(`CREATE TABLE errand_shell_write_measurement (bytes INTEGER NOT NULL);
INSERT INTO errand_shell_write_measurement VALUES (0);
CREATE TRIGGER measure_errand_shell_insert AFTER INSERT ON errand_message_shell_output_chunks BEGIN
 UPDATE errand_shell_write_measurement SET bytes=bytes+length(NEW.content); END;
CREATE TRIGGER measure_errand_shell_update AFTER UPDATE ON errand_message_shell_output_chunks BEGIN
 UPDATE errand_shell_write_measurement SET bytes=bytes+length(NEW.content); END;`)
		require.NoError(t, err)
	}
	state, err := streams.ApplyShellOutputPatch(nil, patch)
	require.NoError(t, err)
	output += " and more"
	patch, err = streams.EncodeShellOutputPatch(state, streams.ShellExecOutput{Stdout: output}, "epoch", false)
	require.NoError(t, err)
	agent.ShellOutput = patch
	agent.TurnID, agent.PromptGeneration = "", 0
	payload, err = json.Marshal(agent)
	require.NoError(t, err)
	event.Sequence, event.SubmissionID, event.Payload = 3, "", payload
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 3)
	require.NoError(t, err)
	projected, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	require.True(t, projected)
	if verifySnapshot != nil {
		verifySnapshot()
	}
	if injectFailure {
		var written int
		require.NoError(t, repo.db.GetContext(ctx, &written, `SELECT bytes FROM errand_shell_write_measurement`))
		require.LessOrEqual(t, written, shellOutputChunkBytes)
	}
	listed, _, err := repo.ListErrandMessages(ctx, "shell-errand", "", 10)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	encodedSummary, err := json.Marshal(listed[0])
	require.NoError(t, err)
	require.Less(t, len(encodedSummary), len(output))
	require.NotContains(t, string(encodedSummary), output)
	detail, err := repo.GetErrandMessageDetail(ctx, "shell-errand", messages[0].ID)
	require.NoError(t, err)
	snapshot, ok := models.ExtractShellExecOutput(detail.Metadata)
	require.True(t, ok)
	require.Equal(t, output, snapshot.Stdout)
	_, err = repo.GetErrandMessageDetail(ctx, "other-errand", messages[0].ID)
	require.Error(t, err)
	agent.ParentToolCallID = "other-parent"
	payload, err = json.Marshal(agent)
	require.NoError(t, err)
	event.Sequence, event.Payload = 4, payload
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 4)
	require.Error(t, err)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, "shell-stream")
	require.NoError(t, err)
	require.EqualValues(t, 3, cursor.ProjectedSequence)
	detail.Metadata = shellOutputMetadata("replacement", "")
	detail.Metadata["tool_call_id"] = "tool"
	require.NoError(t, repo.UpdateErrandMessage(ctx, "shell-errand", detail))
	var heads int
	require.NoError(t, repo.db.GetContext(ctx, &heads, repo.db.Rebind(`SELECT COUNT(*) FROM errand_message_shell_output WHERE message_id=?`), detail.ID))
	require.Zero(t, heads)
	detail, err = repo.GetErrandMessageDetail(ctx, "shell-errand", detail.ID)
	require.NoError(t, err)
	snapshot, ok = models.ExtractShellExecOutput(detail.Metadata)
	require.True(t, ok)
	require.Equal(t, "replacement", snapshot.Stdout)
	agent.ParentToolCallID = ""
	retainedOutput := streams.ShellExecOutput{Stdout: "sensitive shell output"}
	checkpoint, err := streams.EncodeShellOutputPatch(nil, retainedOutput, "after-replacement", true)
	require.NoError(t, err)
	agent.ShellOutput = checkpoint
	event.Payload, err = json.Marshal(agent)
	require.NoError(t, err)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 4)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	removed, err := repo.GetErrandMessage(ctx, "shell-errand", detail.ID)
	require.NoError(t, err)
	metadata, err := json.Marshal(removed.Metadata)
	require.NoError(t, err)
	reduction, err := models.ReduceToolPayload(string(removed.Type), metadata, repo.nowUTC())
	require.NoError(t, err)
	require.Empty(t, reduction.Reason)
	require.NoError(t, json.Unmarshal(reduction.Metadata, &removed.Metadata))
	require.NoError(t, repo.UpdateErrandMessage(ctx, "shell-errand", removed))
	removedAt := removed.UpdatedAt
	retainedState, err := streams.ApplyShellOutputPatch(nil, checkpoint)
	require.NoError(t, err)
	late, err := streams.EncodeShellOutputPatch(retainedState, streams.ShellExecOutput{Stdout: "sensitive shell output and late bytes"}, "after-replacement", false)
	require.NoError(t, err)
	agent.ShellOutput = late
	event.Sequence = 5
	event.Payload, err = json.Marshal(agent)
	require.NoError(t, err)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 5)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	removed, err = repo.GetErrandMessageDetail(ctx, "shell-errand", detail.ID)
	require.NoError(t, err)
	require.True(t, models.ToolPayloadRemoved(removed.Metadata))
	require.True(t, removedAt.Equal(removed.UpdatedAt))
	serialized, err := json.Marshal(removed.Metadata)
	require.NoError(t, err)
	require.NotContains(t, string(serialized), "sensitive shell output")
	require.NoError(t, repo.db.GetContext(ctx, &heads, repo.db.Rebind(`SELECT COUNT(*) FROM errand_message_shell_output WHERE message_id=?`), detail.ID))
	require.Zero(t, heads)
	cursor, err = repo.GetAgentDeliveryCursor(ctx, "shell-stream")
	require.NoError(t, err)
	require.EqualValues(t, 5, cursor.ProjectedSequence)
}
