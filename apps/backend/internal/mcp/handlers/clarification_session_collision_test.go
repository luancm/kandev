package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/clarification"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestAskUserQuestionRejectsForeignWaiterCollision(t *testing.T) {
	store := clarification.NewStore(time.Second)
	questions := []clarification.Question{{ID: "question", Prompt: "Which?", Options: []clarification.Option{{ID: "one", Label: "One"}, {ID: "two", Label: "Two"}}}}
	pendingID := clarification.PendingIDForRequest("session", "retry", questions, "")
	_, created, err := store.CreateRequest(&clarification.Request{PendingID: pendingID, SessionID: "other-session", Questions: questions})
	require.NoError(t, err)
	require.True(t, created)
	h := &Handlers{clarificationSvc: store, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPAskUserQuestion, map[string]interface{}{"session_id": "session", "task_id": "task", "retry_key": "retry", "questions": questions})
	response, err := h.handleAskUserQuestion(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.True(t, store.HasWaiter(pendingID))
	require.Equal(t, []string{pendingID}, store.CancelSession("other-session"))
}
