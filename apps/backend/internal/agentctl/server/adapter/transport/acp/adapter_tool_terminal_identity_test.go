package acp

import (
	"testing"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-003.7
func TestPromptEndPreservesInitiallyTerminalTool(t *testing.T) {
	for _, status := range []acp.ToolCallStatus{"completed", "failed", "in_progress"} {
		t.Run(string(status), func(t *testing.T) {
			a := newTestAdapter()
			t.Cleanup(func() { _ = a.Close() })
			event := a.convertToolCallUpdateForGeneration("native-A", &acp.SessionUpdateToolCall{
				ToolCallId: "exec-A", Kind: "execute", Title: "command", Status: status,
				RawInput: map[string]any{"command": "true"},
			}, 7)
			require.NotNil(t, event)
			require.Equal(t, string(status), event.ToolStatus)
			a.cancelPromptEndToolCalls("native-A")
			events := drainEvents(a)
			if status == "in_progress" {
				require.Len(t, events, 1)
				require.Equal(t, "cancelled", events[0].ToolStatus)
			} else {
				require.Empty(t, events, "a terminal start must not receive a synthetic cancellation")
			}
		})
	}
}

func TestPromptEndCancellationRetainsToolOwner(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "prompt_end", true: "explicit_cancel"}[explicit], func(t *testing.T) {
			a := newTestAdapter()
			t.Cleanup(func() { _ = a.Close() })
			require.NotNil(t, a.convertToolCallUpdateForGeneration("native-A", &acp.SessionUpdateToolCall{
				ToolCallId: "exec-A", Kind: "execute", Title: "command", Status: "in_progress",
				Meta: map[string]any{"claudeCode": map[string]any{"parentToolUseId": "parent-A"}},
			}, 7))
			// Prompt-end cleanup may run after the owning prompt has been retired.
			a.promptTurn = &promptTurnState{promptGeneration: 8}
			if explicit {
				a.cancelActiveToolCalls("native-A")
			} else {
				a.cancelPromptEndToolCalls("native-A")
			}
			events := drainEvents(a)
			require.Len(t, events, 1)
			require.Equal(t, streams.EventTypeToolUpdate, events[0].Type)
			require.Equal(t, uint64(7), events[0].PromptGeneration)
			require.Equal(t, "native-A", events[0].SessionID)
			require.Equal(t, "parent-A", events[0].ParentToolCallID)
			a.cancelPromptEndToolCalls("native-A")
			require.Empty(t, drainEvents(a), "cleanup is emitted once")
		})
	}
}
