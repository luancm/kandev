package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func addErrandSession(
	t *testing.T, repo *sqliterepo.Repository, taskID, sessionID, parentID string,
	state models.TaskSessionState, age sessionAge,
) *models.TaskSession {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, Role: models.SessionRoleErrand, ParentSessionID: parentID,
		AgentProfileID: "agent-profile-2", State: models.TaskSessionStateCreated, StartedAt: age.startedAt(),
	}))
	if state != models.TaskSessionStateCreated {
		require.NoError(t, repo.UpdateTaskSessionState(ctx, sessionID, state, ""))
	}
	loaded, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	return loaded
}

func resolveTargetWithoutSessionID(t *testing.T, h *Handlers, taskID string) (*models.TaskSession, bool) {
	t.Helper()
	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{"task_id": taskID, "prompt": "hello"})
	session, pinned, errResp := h.resolveMessageTargetSession(context.Background(), msg, taskID, "")
	require.Nil(t, errResp)
	require.NotNil(t, session)
	return session, pinned
}

// @covers AC-TASKS-ERRANDS-005.3
func TestResolveMessageTargetSession_ErrandIsNeverTheLiveFallback(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	live := addSession(t, repo, target.ID, "sess-live", models.TaskSessionStateRunning, 1)
	addErrandSession(t, repo, target.ID, "sess-errand", live.ID, models.TaskSessionStateRunning, 2)
	cancelSession(t, repo, primary.ID)
	h, _ := newMessageTaskHandler(t, svc, repo)

	session, pinned := resolveTargetWithoutSessionID(t, h, target.ID)
	assert.Equal(t, live.ID, session.ID, "the fallback must skip the newer errand")
	assert.True(t, pinned)
}

// @covers AC-TASKS-ERRANDS-005.3
func TestResolveMessageTargetSession_ErrandAloneLeavesTheTerminalPrimary(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	addErrandSession(t, repo, target.ID, "sess-errand", primary.ID, models.TaskSessionStateRunning, 1)
	cancelSession(t, repo, primary.ID)
	h, _ := newMessageTaskHandler(t, svc, repo)

	session, _ := resolveTargetWithoutSessionID(t, h, target.ID)
	assert.Equal(t, primary.ID, session.ID, "an errand must not take over a task's messages")
}

// @covers AC-TASKS-ERRANDS-005.3
func TestResolveMessageTargetSession_CompletedFallbackSkipsErrand(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-errand-fallback", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-errand-fallback", WorkspaceID: "ws-errand-fallback", Name: "Board"}))
	targetResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-errand-fallback", WorkflowID: "wf-errand-fallback", Title: "Target",
	})
	require.NoError(t, err)
	ordinary := addSession(t, repo, targetResult.Task.ID, "sess-completed", models.TaskSessionStateCompleted, 1)
	addErrandSession(t, repo, targetResult.Task.ID, "sess-errand-completed", ordinary.ID, models.TaskSessionStateCompleted, 2)
	h, _ := newMessageTaskHandler(t, svc, repo)

	session, pinned := resolveTargetWithoutSessionID(t, h, targetResult.Task.ID)
	assert.Equal(t, ordinary.ID, session.ID)
	assert.True(t, pinned)
}

// @covers AC-TASKS-ERRANDS-005.3
func TestResolveMessageTargetSession_ExplicitErrandSessionIDStillResolves(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	errand := addErrandSession(t, repo, target.ID, "sess-errand", primary.ID, models.TaskSessionStateWaitingForInput, 1)
	h, _ := newMessageTaskHandler(t, svc, repo)
	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{
		"task_id": target.ID, "session_id": errand.ID, "prompt": "follow up",
	})

	session, pinned, errResp := h.resolveMessageTargetSession(context.Background(), msg, target.ID, errand.ID)
	require.Nil(t, errResp)
	assert.Equal(t, errand.ID, session.ID)
	assert.True(t, pinned)
}
