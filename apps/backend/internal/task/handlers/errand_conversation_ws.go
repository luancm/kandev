package handlers

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/authz"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type errandMessagePage struct {
	TaskID   string        `json:"task_id"`
	ErrandID string        `json:"errand_id"`
	Messages []*v1.Message `json:"messages"`
	HasMore  bool          `json:"has_more"`
}

func (h *TaskHandlers) wsErrandMessages(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if refused := h.browserErrandGuard(msg); refused != nil {
		return refused, nil
	}
	var req struct {
		TaskID          string `json:"task_id"`
		ErrandID        string `json:"errand_id"`
		BeforeMessageID string `json:"before_message_id"`
		Limit           int    `json:"limit"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	req.TaskID, req.ErrandID = strings.TrimSpace(req.TaskID), strings.TrimSpace(req.ErrandID)
	if req.TaskID == "" || req.ErrandID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id and errand_id are required", nil)
	}
	if err := h.authorizeErrandTask(ctx, req.TaskID, authz.ScopeWorkspaceRead); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, errandTaskNotFound, nil)
	}
	messages, more, err := h.errands.ReadErrandMessages(ctx, req.TaskID, req.ErrandID, req.BeforeMessageID, req.Limit)
	if err != nil {
		return h.errandFailure(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, errandMessagePage{
		TaskID: req.TaskID, ErrandID: req.ErrandID, Messages: messagesToAPI(messages), HasMore: more,
	})
}

func (h *TaskHandlers) wsMarkErrandRead(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if refused := h.errandGuard(msg); refused != nil {
		return refused, nil
	}
	var req struct {
		TaskID    string `json:"task_id"`
		ErrandID  string `json:"errand_id"`
		MessageID string `json:"message_id"`
	}
	if err := msg.ParsePayload(&req); err != nil || req.TaskID == "" || req.ErrandID == "" || req.MessageID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id, errand_id and message_id are required", nil)
	}
	if err := h.authorizeErrandTask(ctx, req.TaskID, authz.ScopeTaskWrite); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, errandTaskNotFound, nil)
	}
	if err := h.errands.MarkErrandRead(ctx, req.TaskID, req.ErrandID, req.MessageID); err != nil {
		return h.errandFailure(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, struct {
		Success bool `json:"success"`
	}{Success: true})
}
