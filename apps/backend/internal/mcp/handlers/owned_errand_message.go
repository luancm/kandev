package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type messageTaskRequest struct {
	TaskID            string `json:"task_id"`
	SessionID         string `json:"session_id"`
	Prompt            string `json:"prompt"`
	SenderTaskID      string `json:"sender_task_id"`
	SenderSessionID   string `json:"sender_session_id"`
	DeliveryMode      string `json:"delivery_mode"`
	ReplyToQuestionID string `json:"reply_to_question_id"`
}

type ownedErrandMessenger interface {
	Lookup(context.Context, string) (*models.Errand, error)
	ContinueFromParent(context.Context, errands.ContinuationRequest) (string, error)
}

// The legacy argument names the exact Errand only after normal pinned session lookup.
func (h *Handlers) tryOwnedErrandMessage(ctx context.Context, msg *ws.Message, req messageTaskRequest, senderTask *models.Task, targetRefusal *ws.Message) (*ws.Message, error) {
	messenger, ok := h.errands.(ownedErrandMessenger)
	if !ok || req.SessionID == "" {
		return nil, nil
	}
	var refusal ws.ErrorPayload
	if targetRefusal.ParsePayload(&refusal) != nil || refusal.Code != ws.ErrorCodeNotFound {
		return nil, nil
	}
	row, err := messenger.Lookup(ctx, req.SessionID)
	if err != nil {
		if reason, known := errands.ReasonOf(err); known && reason == errands.ReasonNotFound {
			return nil, nil
		}
		return h.ownedErrandMessageFailure(msg, err)
	}
	if row == nil || row.TaskID != req.TaskID {
		return nil, nil
	}
	if refusal := h.errandsDisabled(msg); refusal != nil {
		return refusal, nil
	}
	parent := h.verifiedSession(ctx, req.SenderTaskID, req.SenderSessionID)
	if !isOwnedErrandMessageParent(ctx, row, parent) {
		return errandRefusal(msg, errands.ReasonSenderNotParent, "only the errand's parent session can message this errand"), nil
	}
	return h.sendOwnedErrandMessage(ctx, msg, req, senderTask, parent, messenger)
}

func isOwnedErrandMessageParent(ctx context.Context, row *models.Errand, parent *models.TaskSession) bool {
	execution, ok := streams.MCPExecutionContextFromContext(ctx)
	return parent != nil && parent.ID == row.ParentSessionID && parent.TaskID == row.TaskID &&
		ok && execution.TaskID == row.TaskID && execution.SessionID == parent.ID &&
		!parent.IsPassthrough && !models.IsErrandSession(parent) && !isConfigSession(parent)
}

func (h *Handlers) sendOwnedErrandMessage(ctx context.Context, msg *ws.Message, req messageTaskRequest, senderTask *models.Task, parent *models.TaskSession, messenger ownedErrandMessenger) (*ws.Message, error) {
	if req.ReplyToQuestionID != "" || len(req.Prompt) > plancomments.MaxRenderedPromptBytes {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "invalid errand instruction", nil)
	}
	if req.DeliveryMode == deliveryModeInterrupt {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "errand instructions require a settled conversation", nil)
	}
	prompt := h.appendPromptReferenceExpansionContext(ctx, req.Prompt)
	wrapped, _ := wrapErrandParentMessage(prompt, senderTask, parent.ID, parent.Name)
	ctx = errands.WithParentPromptOrigin(ctx, errands.ParentPromptOrigin{SenderSessionID: parent.ID})
	turnID, err := messenger.ContinueFromParent(ctx, errands.ContinuationRequest{
		TaskID: req.TaskID, ErrandID: req.SessionID, ClientMessageID: msg.ID, Content: wrapped,
	})
	if err != nil {
		return h.ownedErrandMessageFailure(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"task_id": req.TaskID, "session_id": req.SessionID, errandIDKey: req.SessionID,
		"turn_id": turnID, stopTaskStatusKey: "started",
	})
}

func (h *Handlers) ownedErrandMessageFailure(msg *ws.Message, err error) (*ws.Message, error) {
	if response, handled := errands.WSError(msg.ID, msg.Action, err); handled {
		return response, nil
	}
	h.logger.Warn("owned errand instruction failed", zap.Error(err))
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "errand instruction failed", nil)
}
