package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type unresolvedErrandLauncher struct{ *fakeOrchestrator }

func (*unresolvedErrandLauncher) ValidateErrandContinuation(context.Context, string) error {
	return db.ErrErrandSettlementPending
}

// @covers AC-TASKS-ERRANDS-002.8
func TestParentErrandContinuationRefusalPrecedesMessage(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, task, parent := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	child := &models.TaskSession{ID: "errand-child", TaskID: task.ID, ParentSessionID: parent.ID, Role: models.SessionRoleErrand, State: models.TaskSessionStateWaitingForInput}
	require.NoError(t, repo.CreateTaskSession(t.Context(), child))
	h, orch := newMessageTaskHandler(t, svc, repo)
	h.sessionLauncher = &unresolvedErrandLauncher{orch}
	revision, err := repo.ReadConversationRevision(t.Context(), child.ID)
	require.NoError(t, err)
	for range 2 {
		request := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{"task_id": task.ID, "session_id": child.ID, "sender_task_id": task.ID, "sender_session_id": parent.ID, "prompt": "new instruction"})
		response, err := h.handleMessageTask(t.Context(), request)
		require.NoError(t, err)
		var payload struct {
			Details map[string]interface{} `json:"details"`
		}
		require.NoError(t, json.Unmarshal(response.Payload, &payload))
		require.Equal(t, "errand_recovery_unresolved", payload.Details["reason"])
	}
	after, err := repo.ReadConversationRevision(t.Context(), child.ID)
	require.NoError(t, err)
	require.Equal(t, revision, after)
	require.Empty(t, orch.promptCalls)
	require.Empty(t, orch.startCreatedCalls)
	require.Empty(t, orch.launchCalls)
}
