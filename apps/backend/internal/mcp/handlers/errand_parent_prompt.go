package handlers

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

// withErrandParentPrompt carries verified provenance to provider admission.
// Enqueueing the message never decides whether an old result was superseded.
func (h *Handlers) withErrandParentPrompt(ctx context.Context, senderTaskID, senderSessionID string, metadata map[string]interface{}) context.Context {
	origin := errands.ParentPromptOrigin{}
	delete(metadata, errands.ParentPromptSenderMetadataKey)
	if sender := h.verifiedSession(ctx, senderTaskID, senderSessionID); sender != nil {
		origin = errands.ParentPromptOrigin{SenderSessionID: sender.ID, SubmissionID: "peer:" + uuid.NewString()}
		if metadata != nil {
			metadata[errands.ParentPromptSenderMetadataKey] = sender.ID
		}
	}
	return errands.WithParentPromptOrigin(ctx, origin)
}

func retainErrandParentPrompt(ctx context.Context, session *models.TaskSession, err error) bool {
	if !models.IsErrandSession(session) || errands.ParentPromptOriginFromContext(ctx).SenderSessionID != session.ParentSessionID {
		return false
	}
	var accepted interface{ DetachedResumeAccepted() bool }
	if errors.As(err, &accepted) && accepted.DetachedResumeAccepted() {
		return true
	}
	var uncertain interface{ ErrandParentPromptUncertain() bool }
	return errors.As(err, &uncertain) && uncertain.ErrandParentPromptUncertain()
}
