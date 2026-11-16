package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestModelSelectionWarningUsesSeparateLifecycleTurn(t *testing.T) {
	f := newResumeTodoFixture(t)
	work, err := f.taskService.StartTurn(t.Context(), f.workerID)
	require.NoError(t, err)
	f.svc.activeTurns.Store(f.workerID, work.ID)
	f.svc.handleAgentStreamEvent(t.Context(), warningStreamPayload(f))
	messages, err := f.repo.ListMessages(t.Context(), f.workerID)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	warningTurn, err := f.repo.GetTurn(t.Context(), messages[1].TurnID)
	require.NoError(t, err)
	require.NotEqual(t, work.ID, warningTurn.ID)
	require.Equal(t, true, warningTurn.Metadata[models.TurnMetaKeyLifecycleOnly])
	require.NotNil(t, warningTurn.CompletedAt)
	current, err := f.repo.GetTurn(t.Context(), work.ID)
	require.NoError(t, err)
	require.Nil(t, current.CompletedAt)
	require.NotEqual(t, true, current.Metadata[models.TurnMetaKeyLifecycleOnly])
	require.Equal(t, work.ID, f.svc.getActiveTurnID(f.workerID))
}

// @covers AC-TASKS-ERRANDS-003.7
// Identical words do not grant ordinary work the typed warning's exemption.
func TestModelSelectionWarningTextDoesNotExemptOrdinaryWork(t *testing.T) {
	for _, active := range []bool{false, true} {
		for _, kind := range []string{"message_streaming", "thinking_streaming", "tool_call"} {
			name := "without_active_turn/" + kind
			if active {
				name = "with_original_turn/" + kind
			}
			t.Run(name, func(t *testing.T) {
				f := newResumeTodoFixture(t)
				f.svc.messageCreator = &warningStreamMessageCreator{serviceBackedMessageCreator: f.messageCreator}
				originalID := ""
				if active {
					work, err := f.taskService.StartTurn(t.Context(), f.workerID)
					require.NoError(t, err)
					originalID = work.ID
					f.svc.activeTurns.Store(f.workerID, originalID)
				}
				warningEvent := warningStreamPayload(f)
				f.svc.handleAgentStreamEvent(t.Context(), warningEvent)
				messages, err := f.repo.ListMessages(t.Context(), f.workerID)
				require.NoError(t, err)
				require.Len(t, messages, 2)
				warning := messages[1]
				// A repeated typed warning must not create another turn or take ownership.
				f.svc.handleAgentStreamEvent(t.Context(), warningEvent)
				data := &lifecycle.AgentStreamEventData{
					Type: kind, MessageID: "real-warning-looking-work",
					ToolCallID: "warning-looking-tool", ToolTitle: warning.Content, ToolStatus: "running",
				}
				if kind != "tool_call" {
					data.Text = warning.Content
				}
				f.svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
					TaskID: f.taskID, SessionID: f.workerID, Data: data,
				})
				messages, err = f.repo.ListMessages(t.Context(), f.workerID)
				require.NoError(t, err)
				require.Len(t, messages, 3)
				ordinary := messages[2]
				if kind == "thinking_streaming" {
					require.Equal(t, warning.Content, ordinary.Metadata["thinking"])
					require.Equal(t, models.MessageType("thinking"), ordinary.Type)
				} else {
					require.Equal(t, warning.Content, ordinary.Content)
				}
				if active {
					require.Equal(t, originalID, ordinary.TurnID)
				}
				require.NotEqual(t, warning.TurnID, ordinary.TurnID)
				current, err := f.repo.GetTurn(t.Context(), ordinary.TurnID)
				require.NoError(t, err)
				require.Nil(t, current.CompletedAt)
				require.NotEqual(t, true, current.Metadata[models.TurnMetaKeyLifecycleOnly])
				require.Equal(t, current.ID, f.svc.getActiveTurnID(f.workerID))
				warningTurn, err := f.repo.GetTurn(t.Context(), warning.TurnID)
				require.NoError(t, err)
				require.NotNil(t, warningTurn.CompletedAt)
				require.Equal(t, true, warningTurn.Metadata[models.TurnMetaKeyLifecycleOnly])
				warningMessages, err := f.repo.ListMessagesByTurnID(t.Context(), warning.TurnID)
				require.NoError(t, err)
				require.Len(t, warningMessages, 1)
				require.Equal(t, warning.ID, warningMessages[0].ID)
			})
		}
	}
}

func warningStreamPayload(f *resumeTodoFixture) *lifecycle.AgentStreamEventPayload {
	return &lifecycle.AgentStreamEventPayload{
		TaskID: f.taskID, SessionID: f.workerID,
		Data: &lifecycle.AgentStreamEventData{
			Type: streams.EventTypeSessionModelSelectionWarning,
			ModelSelectionWarning: &streams.ModelSelectionWarning{
				Kind: "model_selection_warning", DecisionID: "warning-stream-decision",
				Reason: "requested_not_advertised", RequestedModel: "saved-model", EffectiveModel: "executor-model",
			},
		},
	}
}
