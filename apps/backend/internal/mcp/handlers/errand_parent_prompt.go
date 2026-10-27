package handlers

import (
	"context"

	"go.uber.org/zap"
)

// ErrandParentPromptRecorder records a prompt admitted into a session that may
// be an errand. The orchestrator implements it and forwards to the errands
// service, which ignores senders that are not the errand's parent.
type ErrandParentPromptRecorder interface {
	OnErrandParentPromptAdmitted(ctx context.Context, errandSessionID, senderSessionID string) error
}

// SetErrandParentPromptRecorder wires the errand parent-prompt hook.
func (h *Handlers) SetErrandParentPromptRecorder(recorder ErrandParentPromptRecorder) {
	h.errandPrompts = recorder
}

// recordErrandParentPrompt reports an admitted message to its target session.
// The sender is trusted only when the sender session belongs to the sender
// task named by the caller; the payload's session ID alone is never enough.
func (h *Handlers) recordErrandParentPrompt(ctx context.Context, senderTaskID, senderSessionID, targetSessionID string) {
	if h.errandPrompts == nil || h.taskSvc == nil || senderSessionID == "" || targetSessionID == "" {
		return
	}
	sender, err := h.taskSvc.GetTaskSession(ctx, senderSessionID)
	if err != nil || sender == nil || sender.TaskID != senderTaskID {
		return
	}
	if err := h.errandPrompts.OnErrandParentPromptAdmitted(ctx, targetSessionID, sender.ID); err != nil {
		h.logger.Warn("failed to record errand parent prompt",
			zap.String("errand_session_id", targetSessionID), zap.Error(err))
	}
}
