package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/errands"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func (h *MessageHandlers) errandContinuationRefusal(ctx context.Context, msg *ws.Message, sessionID string) *ws.Message {
	validator, ok := h.orchestrator.(interface {
		ValidateErrandContinuation(context.Context, string) error
	})
	if !ok {
		return nil
	}
	err := validator.ValidateErrandContinuation(ctx, sessionID)
	if err == nil {
		return nil
	}
	if response, handled := errands.WSError(msg.ID, msg.Action, err); handled {
		return response
	}
	response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to verify errand continuation", nil)
	return response
}
