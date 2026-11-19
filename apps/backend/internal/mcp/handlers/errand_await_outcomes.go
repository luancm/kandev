package handlers

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/errands"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func awaitCallerIdentity(ctx context.Context, parent *models.TaskSession, token string, abort bool) (models.ErrandAwaitIdentity, error) {
	execution, ok := streams.MCPExecutionContextFromContext(ctx)
	id := models.ErrandAwaitIdentity{TaskID: parent.TaskID, ParentSessionID: parent.ID, Token: token,
		ParentIncarnationID: execution.ParentIncarnationID, HarnessGeneration: execution.HarnessGeneration}
	if !ok || execution.TaskID != id.TaskID || execution.SessionID != id.ParentSessionID || !id.Valid() {
		return id, models.ErrInvalidErrand
	}
	original, present := mcporigin.AwaitInvocation(ctx)
	if !present {
		return id, nil
	}
	return bindAwaitInvocation(id, original, abort && mcporigin.IsTrustedInternalCall(ctx))
}

func bindAwaitInvocation(id, original models.ErrandAwaitIdentity, trustedAbort bool) (models.ErrandAwaitIdentity, error) {
	if original.TaskID != id.TaskID || original.ParentSessionID != id.ParentSessionID || original.Token != id.Token {
		return id, models.ErrInvalidErrand
	}
	if trustedAbort {
		// A journalled pre-success error may outlive its harness generation.
		// Its old identity still has to match the retained attempt exactly.
		if !original.Valid() {
			return id, models.ErrInvalidErrand
		}
		return original, nil
	}
	if original.ParentIncarnationID != id.ParentIncarnationID || original.HarnessGeneration != id.HarnessGeneration {
		return id, models.ErrInvalidErrand
	}
	id.TurnID, id.SubmissionID = original.TurnID, original.SubmissionID
	return id, nil
}

func (h *Handlers) offerAwaitResponse(ctx context.Context, msg *ws.Message, id models.ErrandAwaitIdentity, result errands.AwaitResult) (*ws.Message, error) {
	response, err := ws.NewResponse(msg.ID, msg.Action, awaitErrandsView(result))
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = h.errands.OfferAwait(ctx, id, result.Delivery)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		return response, nil
	}
	abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if abortErr := h.errands.AbortOwnedAwait(abortCtx, id); abortErr != nil {
		h.logger.Warn("await error abort not confirmed")
	}
	return errandServiceError(msg, err), nil
}
