package handlers

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type ownedErrandMessageFixture struct {
	*fakeErrandService
	row    *models.Errand
	origin errands.ParentPromptOrigin
	calls  int
	err    error
}

func (f *ownedErrandMessageFixture) Lookup(context.Context, string) (*models.Errand, error) {
	return f.row, nil
}

func (f *ownedErrandMessageFixture) ContinueFromParent(ctx context.Context, _ errands.ContinuationRequest) (string, error) {
	f.origin = errands.ParentPromptOriginFromContext(ctx)
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return "turn", nil
}

func TestOwnedErrandMessageRequiresAuthenticatedParent(t *testing.T) {
	h, fake, task, parent, _ := newErrandMCPFixture(t, true)
	messenger := &ownedErrandMessageFixture{
		fakeErrandService: fake,
		row:               &models.Errand{ID: "owned-errand", TaskID: task.ID, ParentSessionID: parent.ID},
	}
	h.SetErrands(messenger, true)
	req := messageTaskRequest{
		TaskID: task.ID, SessionID: messenger.row.ID, Prompt: "Follow up",
		SenderTaskID: task.ID, SenderSessionID: parent.ID,
	}
	msg := makeWSMessage(t, ws.ActionMCPMessageTask, req)
	for _, ctx := range []context.Context{t.Context(), awaitTestContext(task.ID, "different")} {
		response, err := h.handleMessageTask(ctx, msg)
		require.NoError(t, err)
		var payload ws.ErrorPayload
		require.NoError(t, response.ParsePayload(&payload))
		require.Equal(t, ws.ErrorCodeForbidden, payload.Code)
		require.Zero(t, messenger.calls)
	}

	response, err := h.handleMessageTask(awaitTestContext(task.ID, parent.ID), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, 1, messenger.calls)
	require.Equal(t, parent.ID, messenger.origin.SenderSessionID)
}

// @covers AC-TASKS-ERRANDS-002.8
func TestOwnedErrandMessageReportsUnresolvedRecovery(t *testing.T) {
	h, fake, task, parent, _ := newErrandMCPFixture(t, true)
	messenger := &ownedErrandMessageFixture{
		fakeErrandService: fake,
		row:               &models.Errand{ID: "owned-errand", TaskID: task.ID, ParentSessionID: parent.ID},
		err:               db.ErrErrandSettlementPending,
	}
	h.SetErrands(messenger, true)
	msg := makeWSMessage(t, ws.ActionMCPMessageTask, messageTaskRequest{
		TaskID: task.ID, SessionID: messenger.row.ID, Prompt: "Follow up",
		SenderTaskID: task.ID, SenderSessionID: parent.ID,
	})
	response, err := h.handleMessageTask(awaitTestContext(task.ID, parent.ID), msg)
	require.NoError(t, err)
	var payload ws.ErrorPayload
	require.NoError(t, response.ParsePayload(&payload))
	require.Equal(t, ws.ErrorCodeConflict, payload.Code)
	require.Equal(t, string(errands.ReasonRecoveryUnresolved), payload.Details["reason"])
	require.Equal(t, 1, messenger.calls)
}
