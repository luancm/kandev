package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ownedConversationReader struct {
	ErrandMCPService
	taskID string
	reads  int
}

func (r *ownedConversationReader) ReadErrandMessages(_ context.Context, taskID, errandID, before string, limit int) ([]*models.Message, bool, error) {
	r.reads++
	if taskID != r.taskID || errandID != "owned-errand" {
		return nil, false, models.ErrErrandNotFound
	}
	if before == "older-command" {
		return []*models.Message{{ID: "first", Type: models.MessageTypeMessage, Content: "first prompt"}}, false, nil
	}
	return []*models.Message{
		{ID: "older-command", Type: models.MessageTypeToolExecute},
		{ID: "full-result", Type: models.MessageTypeMessage, Content: "the full owned result"},
	}, true, nil
}

func TestHandleGetTaskConversation_ReadsOwnedErrandUnderTaskScope(t *testing.T) {
	svc, repo := newTestTaskService(t)
	owner := seedOwnedTask(t, svc, repo, "owned-reader", "user-owned")
	foreign := seedOwnedTask(t, svc, repo, "foreign-reader", "user-foreign")
	reader := &ownedConversationReader{taskID: owner.taskID}
	h := &Handlers{taskSvc: svc, errands: reader, logger: testLogger(t).WithFields()}
	resolver := mcpscope.NewResolver(repo, stubIdentityLookup{
		owner.userID:   authn.Identity{UserID: owner.userID, Role: authn.RoleMember},
		foreign.userID: authn.Identity{UserID: foreign.userID, Role: authn.RoleMember},
	}, func() bool { return true }, testLogger(t))
	ctx, err := resolver.Scope(t.Context(), owner.taskID)
	require.NoError(t, err)
	foreignCtx, err := resolver.Scope(t.Context(), foreign.taskID)
	require.NoError(t, err)

	payload := map[string]any{"task_id": owner.taskID, "errand_id": "owned-errand", "message_types": []string{"message"}}
	msg := makeWSMessage(t, ws.ActionMCPGetTaskConversation, payload)
	response, err := h.handleGetTaskConversation(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	var page map[string]any
	require.NoError(t, response.ParsePayload(&page))
	assert.Equal(t, "owned-errand", page["errand_id"])
	assert.NotContains(t, page, "session_id")
	assert.Equal(t, "older-command", page["cursor"], "cursor must include filtered messages")
	assert.Equal(t, true, page["has_more"])
	assert.Contains(t, string(response.Payload), "the full owned result")

	response, err = h.handleGetTaskConversation(foreignCtx, msg)
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeNotFound)
	require.Equal(t, 1, reader.reads, "foreign task is refused before reading the Errand")

	payload["before"] = "older-command"
	response, err = h.handleGetTaskConversation(ctx, makeWSMessage(t, ws.ActionMCPGetTaskConversation, payload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	assert.Contains(t, string(response.Payload), "first prompt")

	payload["task_id"] = foreign.taskID
	response, err = h.handleGetTaskConversation(foreignCtx, makeWSMessage(t, ws.ActionMCPGetTaskConversation, payload))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeNotFound)
}

func TestHandleGetTaskConversation_MissingTaskID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPGetTaskConversation, map[string]interface{}{})
	resp, err := h.handleGetTaskConversation(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleGetTaskConversation_UsesPrimarySession(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, task, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	_, err := svc.CreateMessage(context.Background(), &service.CreateMessageRequest{
		TaskSessionID: sess.ID,
		TaskID:        task.ID,
		AuthorType:    "user",
		Content:       "hello from task",
	})
	require.NoError(t, err)

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPGetTaskConversation, map[string]interface{}{
		"task_id": task.ID,
		"limit":   10,
	})

	resp, err := h.handleGetTaskConversation(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, task.ID, payload["task_id"])
	assert.Equal(t, sess.ID, payload["session_id"])
	assert.Equal(t, float64(1), payload["total"])
}

func TestHandleGetTaskConversation_FallsBackToLatestTaskSession(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	taskResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Office task",
	})
	task := taskResult.Task
	require.NoError(t, err)
	older := &models.TaskSession{
		ID:             "older-office-session",
		TaskID:         task.ID,
		AgentProfileID: "agent-profile-1",
		IsPrimary:      false,
		State:          models.TaskSessionStateWaitingForInput,
		StartedAt:      time.Now().Add(-time.Hour),
	}
	require.NoError(t, repo.CreateTaskSession(ctx, older))
	newer := &models.TaskSession{
		ID:             "newer-office-session",
		TaskID:         task.ID,
		AgentProfileID: "agent-profile-1",
		IsPrimary:      false,
		State:          models.TaskSessionStateWaitingForInput,
		StartedAt:      time.Now(),
	}
	require.NoError(t, repo.CreateTaskSession(ctx, newer))
	_, err = svc.CreateMessage(ctx, &service.CreateMessageRequest{
		TaskSessionID: newer.ID,
		TaskID:        task.ID,
		AuthorType:    "agent",
		Content:       "office message",
	})
	require.NoError(t, err)

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPGetTaskConversation, map[string]interface{}{
		"task_id": task.ID,
		"limit":   10,
	})

	resp, err := h.handleGetTaskConversation(ctx, msg)
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, newer.ID, payload["session_id"])
	assert.Equal(t, float64(1), payload["total"])
}

func TestHandleGetTaskConversation_SessionMustBelongToTask(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, taskA, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	// Create another task/session in the same workflow to validate cross-task mismatch.
	taskBResult, err := svc.CreateTask(context.Background(), &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Other task",
	})
	taskB := taskBResult.Task
	require.NoError(t, err)
	sessB := &models.TaskSession{
		ID:             "sess-2",
		TaskID:         taskB.ID,
		AgentProfileID: "agent-profile-1",
		IsPrimary:      true,
		State:          models.TaskSessionStateWaitingForInput,
	}
	require.NoError(t, repo.CreateTaskSession(context.Background(), sessB))

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPGetTaskConversation, map[string]interface{}{
		"task_id":    taskA.ID,
		"session_id": "sess-2",
	})

	resp, err := h.handleGetTaskConversation(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleGetTaskConversation_NegativeLimit(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, task, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPGetTaskConversation, map[string]interface{}{
		"task_id": task.ID,
		"limit":   -1,
	})

	resp, err := h.handleGetTaskConversation(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleGetTaskConversation_FilteredPageStillReturnsCursor(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, task, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	_, err := svc.CreateMessage(context.Background(), &service.CreateMessageRequest{
		TaskSessionID: sess.ID,
		TaskID:        task.ID,
		AuthorType:    "agent",
		Type:          "tool_call",
		Content:       "tool call 1",
	})
	require.NoError(t, err)
	_, err = svc.CreateMessage(context.Background(), &service.CreateMessageRequest{
		TaskSessionID: sess.ID,
		TaskID:        task.ID,
		AuthorType:    "agent",
		Type:          "tool_call",
		Content:       "tool call 2",
	})
	require.NoError(t, err)

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPGetTaskConversation, map[string]interface{}{
		"task_id":       task.ID,
		"limit":         1,
		"message_types": []string{"message"},
	})

	resp, err := h.handleGetTaskConversation(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, float64(0), payload["total"])
	assert.Equal(t, true, payload["has_more"])
	assert.NotEmpty(t, payload["cursor"])
}
