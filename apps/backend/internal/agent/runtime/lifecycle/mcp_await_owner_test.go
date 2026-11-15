package lifecycle

import (
	"context"
	"encoding/json"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestMCPHandlerForAwaitCapturesLaunchedOwner(t *testing.T) {
	inner := &recordingMCPHandler{}
	sm := newMCPStreamManager(t, inner, nil)
	execution := &AgentExecution{ID: "exec", TaskID: "task", SessionID: "parent", DeliveryIncarnationID: "launched", DeliveryHarnessGeneration: 2}
	handler := sm.mcpHandlerFor(execution)
	execution.DeliveryIncarnationID = "replacement"
	execution.DeliveryHarnessGeneration = 9
	invocation := models.ErrandAwaitIdentity{TaskID: "task", ParentSessionID: "parent", Token: "token", ParentIncarnationID: "launched", HarnessGeneration: 2, TurnID: "origin-turn", SubmissionID: "origin-submission"}
	message := mcpRequest(t, map[string]any{"parent_incarnation_id": "forged", "harness_generation": 99})
	mcporigin.AttachAwaitInvocation(mcporigin.WithAwaitInvocation(context.Background(), invocation), message)
	raw, err := json.Marshal(message)
	require.NoError(t, err)
	var decoded ws.Message
	require.NoError(t, json.Unmarshal(raw, &decoded))
	_, err = handler.Dispatch(context.Background(), &decoded)
	require.NoError(t, err)
	owner, ok := streams.MCPExecutionContextFromContext(inner.gotCtx)
	require.True(t, ok)
	require.Equal(t, "launched", owner.ParentIncarnationID)
	require.EqualValues(t, 2, owner.HarnessGeneration)
	original, ok := mcporigin.AwaitInvocation(inner.gotCtx)
	require.True(t, ok)
	require.Equal(t, invocation, original)
}
