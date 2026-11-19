package handlers

import (
	"context"
	"errors"
	"slices"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func (h *Handlers) handleOwnedErrandConversation(ctx context.Context, msg *ws.Message, req *taskConversationRequest) (*ws.Message, error) {
	notFound := func() (*ws.Message, error) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "errand not found", nil)
	}
	if h.errands == nil || h.taskSvc.AuthorizeTaskScope(ctx, req.TaskID, authz.ScopeWorkspaceRead) != nil {
		return notFound()
	}
	messages, more, err := h.errands.ReadErrandMessages(ctx, req.TaskID, req.ErrandID, req.Before, conversationLimit(req.Limit))
	if err != nil {
		reason, _ := errands.ReasonOf(err)
		if reason == errands.ReasonNotFound || errors.Is(err, models.ErrErrandNotFound) {
			return notFound()
		}
		h.logger.Error("failed to list errand conversation", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list errand conversation", nil)
	}
	slices.Reverse(messages)
	result := filterAndConvertMessages(messages, req.Types)
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"task_id": req.TaskID, "errand_id": req.ErrandID, "messages": result,
		"total": len(result), "has_more": more, "cursor": conversationCursor(messages),
	})
}
