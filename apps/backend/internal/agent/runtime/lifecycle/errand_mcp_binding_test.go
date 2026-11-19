package lifecycle

import (
	"context"
	"testing"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestErrandMCPDispatchRejectsSupersededPersistedBinding(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "workspace"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "workspace", Title: "task"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "parent", TaskID: "task"}))
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{ID: "child", TaskID: "task", ParentSessionID: "parent", Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone}))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "child", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "child", 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	binding := &models.ErrandHarnessBinding{ErrandID: "child", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream"}
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	inner := &recordingMCPHandler{}
	sm := newMCPStreamManager(t, inner, nil)
	resolver := mcpscope.NewResolver(repo, nil, func() bool { return false }, sm.logger)
	sm.setMCPPrincipalScoper(resolver.ScopePrincipal)
	execution := &AgentExecution{ID: "execution", Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", WorkspaceID: "workspace", ErrandID: "child", Generation: 1}, DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1, DeliveryStreamID: "stream"}
	handler := sm.mcpHandlerFor(execution)
	response, err := handler.Dispatch(ctx, mcpRequest(t, nil))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	successor := *binding
	successor.HarnessGeneration = 2
	successor.StreamID = "successor"
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &successor, 1))
	response, err = handler.Dispatch(ctx, mcpRequest(t, nil))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Equal(t, 1, inner.calls)
}
