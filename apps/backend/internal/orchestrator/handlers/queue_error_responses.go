package handlers

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/errands"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func isQueueIdentityError(err error) bool {
	return errors.Is(err, messagequeue.ErrSessionIdentityMismatch) ||
		errors.Is(err, messagequeue.ErrTaskInactive)
}

func queueAdmissionErrorResponse(msg *ws.Message) (*ws.Message, error) {
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnavailable,
		"Session queue is busy; retry the operation", nil)
}

func (h *QueueHandlers) errandContinuationRefusal(ctx context.Context, msg *ws.Message, sessionID string) *ws.Message {
	validator, ok := h.queueDrainer.(interface {
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
	response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to verify queued continuation", nil)
	return response
}
