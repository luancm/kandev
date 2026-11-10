package lifecycle

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestErrandRetainedAdmissionReachesLifecycleConsumer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.db")
	repo, database := openRetentionLifecycleRepository(t, path)
	seedProjectionResumeOwner(t, repo)
	require.NoError(t, repo.CreateWorkspace(t.Context(), &models.Workspace{ID: "workspace", Name: "Ownership"}))
	_, workspaceErr := repo.DB().Exec(`UPDATE tasks SET workspace_id='workspace' WHERE id='task'`)
	require.NoError(t, workspaceErr)
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "parent", TaskID: "task"}))
	_, err := repo.DB().Exec(`UPDATE task_sessions SET role='errand',parent_session_id='parent' WHERE id='session'`)
	require.NoError(t, err)
	require.NoError(t, repo.CreateTurn(t.Context(), &models.Turn{ID: "original", TaskID: "task", TaskSessionID: "session", StartedAt: time.Now()}))
	_, err = repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{ID: "prompt:original", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: "hash", Payload: []byte(`{}`)})
	require.NoError(t, err)
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", DeliveryMode: DurableDeliveryV1, DeliveryStreamID: "stream", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2, ACPSessionID: "native-conversation", promptTurnID: "original", promptGeneration: 7}
	sm := &SessionManager{persistPromptTurnBinding: func(ctx context.Context, e *AgentExecution) error {
		return repo.UpsertExecutorRunning(ctx, buildRunningFromExecution(e, nil))
	}}
	require.NoError(t, sm.persistDurablePromptTurnBinding(t.Context(), execution, "original"))
	require.NoError(t, repo.CompleteTurn(t.Context(), "original"))
	require.NoError(t, repo.DeleteExecutorRunningBySessionID(t.Context(), "session"))
	require.NoError(t, database.Close())
	repo, _ = openRetentionLifecycleRepository(t, path)
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "session", SessionID: "session", TaskID: "task", AgentExecutionID: "execution"}))
	execution.promptTurnID = "successor"
	execution.promptGeneration = 7
	execution.setDeliverySubmissionID("prompt:successor")
	retained := openRetentionLifecycleJournal(t, filepath.Join(t.TempDir(), "journal.db"))
	payload, _ := json.Marshal(agentctl.AgentEvent{Type: "message_chunk", SessionID: "native-conversation", Text: "late original", PromptGeneration: 7})
	_, err = retained.Append(t.Context(), journal.Event{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream", SubmissionID: "prompt:original", Type: "message_chunk", Payload: payload})
	require.NoError(t, err)
	page, _, err := retained.Replay(t.Context(), "stream", 0, 10)
	require.NoError(t, err)
	require.Len(t, page, 1)
	var wire agentctl.AgentEvent
	require.NoError(t, json.Unmarshal(page[0].Payload, &wire))
	wire.DeliveryStreamID = "stream"
	wire.DeliverySequence = 1
	wire.DeliverySubmissionID = page[0].SubmissionID
	wire.DeliveryIncarnationID = "incarnation"
	wire.DeliveryHarnessGeneration = 2
	var observedTurn string
	observedCalls := 0
	stream := NewStreamManager(newTestLogger(), StreamCallbacks{OnAgentEvent: func(_ *AgentExecution, e agentctl.AgentEvent) { observedTurn = e.TurnID; observedCalls++ }}, nil, nil)
	prepared, err := stream.prepareAgentEvent(t.Context(), execution, nil, repo, wire)
	require.NoError(t, err)
	require.Equal(t, "original", prepared.event.TurnID, "callback ownership must survive restart and mutable successor state")
	require.Equal(t, "prompt:successor", execution.deliverySubmissionIDSnapshot(), "late output cannot replace active submission")
	require.NoError(t, stream.projectReceivedDeliveryEvent(t.Context(), execution, repo, prepared.durableEvent))
	require.Equal(t, "original", observedTurn)
	messages, err := repo.ListMessages(t.Context(), "session")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "original", messages[0].TurnID)
	terminal := wire
	terminal.Type = "complete"
	terminal.Text = ""
	terminal.DeliverySequence = 2
	terminal.PromptGeneration = 0
	historical, terminalErr := stream.prepareAgentEvent(t.Context(), execution, nil, repo, terminal)
	require.NoError(t, terminalErr)
	require.True(t, historical.skipCallback, "a proven old terminal must not complete the successor lifecycle")
	beforeTerminal := observedCalls
	historical.skipAcknowledgement = true
	require.NoError(t, stream.processPreparedAgentEvent(t.Context(), execution, nil, repo, historical, 0))
	require.Equal(t, beforeTerminal, observedCalls)
	execution.dispatchedPromptPending.Store(true)
	native := wire
	native.DeliverySequence = 3
	native.DeliverySubmissionID = ""
	native.PromptGeneration = 99
	execution.promptTurnIDs = map[uint64]string{99: "successor"}
	native.Text = "native output"
	prepared, err = stream.prepareAgentEvent(t.Context(), execution, nil, repo, native)
	require.NoError(t, err, "native activity during B remains unowned real work")
	require.NoError(t, stream.projectReceivedDeliveryEvent(t.Context(), execution, repo, prepared.durableEvent))
	require.NotEmpty(t, observedTurn)
	require.NotEqual(t, "successor", observedTurn)
	require.NotEqual(t, "original", observedTurn)
	nativeTerminal := native
	nativeTerminal.Type = "complete"
	nativeTerminal.Text = ""
	nativeTerminal.PromptGeneration = execution.promptGenerationSnapshot()
	nativeTerminal.DeliverySequence = 4
	unownedCompletion, completionErr := stream.prepareAgentEvent(t.Context(), execution, nil, repo, nativeTerminal)
	require.NoError(t, completionErr)
	require.True(t, unownedCompletion.skipCallback, "unowned native completion cannot claim an admitted prompt")
	beforeUnownedTerminal := observedCalls
	unownedCompletion.skipAcknowledgement = true
	require.NoError(t, stream.processPreparedAgentEvent(t.Context(), execution, nil, repo, unownedCompletion, 0))
	require.Equal(t, beforeUnownedTerminal, observedCalls)

}

func TestRetainedTerminalCannotClaimSuccessorWithReusedGeneration(t *testing.T) {
	manager, _ := createTestManagerWithTracking()
	execution := createTestExecution("execution", "task", "session")
	execution.promptGeneration = 7
	execution.promptTurnID = "successor"
	execution.setDeliverySubmissionID("prompt:successor")
	execution.dispatchedPromptPending.Store(true)
	require.NoError(t, manager.executionStore.Add(execution))
	claim, accepted := manager.claimPromptCompletion(execution, &agentctl.AgentEvent{Type: "complete", PromptGeneration: 7, TurnID: "original", DeliverySubmissionID: "prompt:original"}, false)
	if claim.locked {
		execution.promptLifecycleMu.Unlock()
	}
	require.False(t, accepted, "the completion claim must check immutable submission under its lock")
	require.Zero(t, execution.promptCompletionGeneration)
	claim, accepted = manager.claimPromptCompletion(execution, &agentctl.AgentEvent{Type: "complete", PromptGeneration: 7, TurnID: "original", DeliverySubmissionID: "prompt:successor"}, false)
	if claim.locked {
		execution.promptLifecycleMu.Unlock()
	}
	require.False(t, accepted, "a different turn cannot claim the current submission")
	require.Zero(t, execution.promptCompletionGeneration)

	claim, accepted = manager.claimPromptCompletion(execution, &agentctl.AgentEvent{Type: "complete", PromptGeneration: 7, TurnID: "successor", DeliverySubmissionID: "prompt:successor"}, false)
	if claim.locked {
		execution.promptLifecycleMu.Unlock()
	}
	require.True(t, accepted)
}
