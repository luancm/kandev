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

func addOwnedErrand(t *testing.T, repo *sqliterepo.Repository, taskID, errandID, parentID string, awaitingResult bool, age sessionAge) {
	t.Helper()
	row := &models.Errand{
		ID: errandID, TaskID: taskID, ParentSessionID: parentID,
		Title: "Inspect", Objective: "Inspect the workspace", AgentProfileID: "agent-profile-2",
		FirstTurnID: errandID + "-turn", SubmissionID: errandID + "-submission",
		Lifecycle: models.ErrandLifecycleLaunched, ResultState: models.ErrandResultNone,
		CreatedAt: age.startedAt(), AwaitingResult: awaitingResult,
	}
	if !awaitingResult {
		row.LastOutcome = models.ErrandOutcomeDone
		row.LatestSettledTurnID = row.FirstTurnID
	}
	require.NoError(t, repo.CreateErrand(t.Context(), row))
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
	addOwnedErrand(t, repo, target.ID, "owned-errand", live.ID, true, 2)
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
	addOwnedErrand(t, repo, target.ID, "owned-errand", primary.ID, true, 1)
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
	addOwnedErrand(t, repo, targetResult.Task.ID, "owned-errand-completed", ordinary.ID, false, 2)
	h, _ := newMessageTaskHandler(t, svc, repo)

	session, pinned := resolveTargetWithoutSessionID(t, h, targetResult.Task.ID)
	assert.Equal(t, ordinary.ID, session.ID)
	assert.True(t, pinned)
}

// @covers AC-TASKS-ERRANDS-005.3
func TestResolveMessageTargetSession_ExplicitOwnedErrandUsesSeparateRoute(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	const errandID = "owned-errand"
	addOwnedErrand(t, repo, target.ID, errandID, primary.ID, false, 1)
	h, _ := newMessageTaskHandler(t, svc, repo)
	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{
		"task_id": target.ID, "session_id": errandID, "prompt": "follow up",
	})

	session, _, refusal := h.resolveMessageTargetSession(t.Context(), msg, target.ID, errandID)
	require.Nil(t, session)
	assertWSError(t, refusal, ws.ErrorCodeNotFound)
}
