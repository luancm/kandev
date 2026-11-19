package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/stretchr/testify/require"
)

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
