package orchestrator

import (
	"encoding/json"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-002.7, AC-TASKS-ERRANDS-003.1
func TestErrandLateToolReplayDeliversOneExactParentResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := errandCompletionFixtureWithoutQueuedSuccessor(t)
		f.errands.StopWorkers()
		const turnID, submission, execution, stream = "settlement-A", "prompt:errand:settlement-A", "settlement-execution", "tool-replay-stream"
		f.manager.currentPromptExecutionID = execution
		f.manager.currentPromptGeneration.Store(7)
		require.NoError(t, f.repo.CreateHarnessSessionGeneration(t.Context(), &models.HarnessSessionGeneration{
			SessionID: f.child.ID, IncarnationID: f.child.QueueIncarnationID, Generation: 1,
			NativeSessionID: "native-A", CreationReason: "initial",
		}))
		_, err := f.repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{
			ID: submission, SessionID: f.child.ID, IncarnationID: f.child.QueueIncarnationID,
			HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "original-prompt", Payload: []byte("original prompt"),
		})
		require.NoError(t, err)
		require.NoError(t, f.repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{
			ID: f.child.ID, SessionID: f.child.ID, TaskID: f.taskID, AgentExecutionID: execution,
			Metadata: map[string]any{"delivery_prompt_turn_binding": map[string]any{
				"task_id": f.taskID, "session_id": f.child.ID, "execution_id": execution,
				"incarnation_id": f.child.QueueIncarnationID, "harness_generation": 1, "prompt_generation": 7,
				"stream_id": stream, "submission_id": submission, "turn_id": turnID, "native_session_id": "native-A",
			}},
		}))
		require.NoError(t, f.svc.reserveErrandTerminalIntent(t.Context(), f.child,
			errands.ParentPromptOrigin{SenderSessionID: f.parent}, turnID, submission, execution))
		project := func(sequence int64, sub string, agent streams.AgentEvent) *models.AgentDeliveryEvent {
			t.Helper()
			raw, err := json.Marshal(agent)
			require.NoError(t, err)
			event := &models.AgentDeliveryEvent{SessionID: f.child.ID, IncarnationID: f.child.QueueIncarnationID,
				HarnessGeneration: 1, StreamID: stream, Sequence: sequence, SubmissionID: sub,
				EventType: agent.Type, Terminal: agent.Type == streams.EventTypeComplete, Payload: raw}
			_, err = f.repo.ReceiveAgentDeliveryEvent(t.Context(), event, sequence)
			require.NoError(t, err)
			_, err = f.repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), event, nil)
			require.NoError(t, err)
			return event
		}
		project(1, submission, streams.AgentEvent{Type: streams.EventTypeToolCall, SessionID: "native-A",
			PromptGeneration: 7, ToolCallID: "exec-A", ToolStatus: "completed", NormalizedPayload: streams.NewGeneric("finished check", nil)})
		project(2, "", streams.AgentEvent{Type: streams.EventTypeToolUpdate, SessionID: "native-A", ToolCallID: "exec-A", ToolStatus: "cancelled"})
		before, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.True(t, before.AwaitingResult, "tool ownership alone cannot settle the Errand")
		require.Empty(t, before.ResultText)
		terminal := project(3, submission, streams.AgentEvent{Type: streams.EventTypeComplete,
			SessionID: "native-A", PromptGeneration: 7, Data: map[string]any{"stop_reason": "end_turn"}})
		turn, child, err := f.repo.ResolveAgentDeliveryEventTurn(t.Context(), terminal)
		require.NoError(t, err)
		require.True(t, child)
		require.Equal(t, turnID, turn)
		f.svc.handleAgentReady(t.Context(), watcher.AgentEventData{TaskID: f.taskID, SessionID: f.child.ID,
			AgentExecutionID: execution, TurnID: turn, PromptGeneration: 7,
			DeliveryStreamID: stream, DeliverySequence: 3, DeliveryIncarnationID: f.child.QueueIncarnationID,
			DeliveryHarnessGeneration: 1, DeliverySubmissionID: submission})
		synctest.Wait()
		settled, err := f.repo.GetErrand(t.Context(), f.child.ID)
		require.NoError(t, err)
		requireSuccessfulErrandSettlementRow(t, settled)
		composed, err := f.errands.ComposeParentTurn(t.Context(), f.parent, models.ErrandDeliveryTurnContext)
		require.NoError(t, err)
		require.Len(t, composed.Items, 1)
		require.Contains(t, composed.Block, "canonical result A")
		_, err = f.repo.ReceiveAgentDeliveryEvent(t.Context(), terminal, 3)
		require.NoError(t, err)
		_, err = f.repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), terminal, nil)
		require.NoError(t, err)
		require.NoError(t, f.errands.ReconcileTerminalDecisions(t.Context(), f.child.ID))
		repeated, err := f.errands.ComposeParentTurn(t.Context(), f.parent, models.ErrandDeliveryTurnContext)
		require.NoError(t, err)
		require.Empty(t, repeated.Items, "replay cannot redeliver the result")
		pending, err := f.repo.PendingErrandTurnSettlements(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Empty(t, pending, "the exact decision is committed, not an injected receipt")
	})
}
