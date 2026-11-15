//go:build linux

package backendapp

import (
	"encoding/json"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator"
	orchhandlers "github.com/kandev/kandev/internal/orchestrator/handlers"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-002.8
func assertRecoveryTransportRefusals(t *testing.T, h bootStateTestHarness, svc *orchestrator.Service, workspace string, child *models.TaskSession) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, h.taskSvc.AbandonOpenTurns(ctx, child.ID))
	require.NoError(t, h.taskRepo.UpdateTaskSessionState(ctx, child.ID, models.TaskSessionStateWaitingForInput, ""))
	revision, err := h.taskRepo.ReadConversationRevision(ctx, child.ID)
	require.NoError(t, err)
	turns, err := h.taskRepo.ListTurnsBySession(ctx, child.ID)
	require.NoError(t, err)
	dispatcher := ws.NewDispatcher()
	taskhandlers.RegisterMessageRoutes(gin.New(), dispatcher, h.taskSvc, &orchestratorWrapper{svc: svc}, logger.Default())
	mcp := mcphandlers.NewHandlers(h.taskSvc, nil, nil, nil, nil, h.taskRepo, h.taskRepo, nil, nil, nil, svc, svc.GetMessageQueue(), logger.Default())
	mcp.RegisterHandlers(dispatcher)
	queueHandlers := orchhandlers.NewQueueHandlers(svc.GetMessageQueue(), nil, logger.Default(), svc, h.taskSvc, nil)
	t.Cleanup(queueHandlers.Stop)
	queueHandlers.RegisterHandlers(dispatcher)
	for _, action := range []string{ws.ActionMessageAdd, ws.ActionMCPMessageTask, ws.ActionMessageQueueAdd, ws.ActionMessageQueueDrain} {
		for range 2 {
			request, err := ws.NewRequest("refused", action, map[string]interface{}{"task_id": child.TaskID, "session_id": child.ID, "session_incarnation_id": child.QueueIncarnationID, "content": "refused new work", "prompt": "refused new work", "sender_task_id": child.TaskID, "sender_session_id": child.ParentSessionID})
			require.NoError(t, err)
			callCtx := ctx
			if action == ws.ActionMCPMessageTask {
				callCtx = mcpscope.WithPrincipal(ctx, mcpscope.Principal{WorkspaceID: workspace, CallerTaskID: child.TaskID, CallerSessionID: child.ParentSessionID})
			}
			response, err := dispatcher.Dispatch(callCtx, request)
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeError, response.Type)
			var payload struct {
				Details map[string]interface{} `json:"details"`
			}
			require.NoError(t, json.Unmarshal(response.Payload, &payload))
			require.Equal(t, "errand_recovery_unresolved", payload.Details["reason"])
		}
	}
	after, err := h.taskRepo.ReadConversationRevision(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, revision, after)
	afterTurns, err := h.taskRepo.ListTurnsBySession(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, turns, afterTurns)
}
