package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type uncertainErrandPromptTestError struct{}

func (uncertainErrandPromptTestError) Error() string                     { return "parent prompt admission is uncertain" }
func (uncertainErrandPromptTestError) ErrandParentPromptUncertain() bool { return true }

type acceptedErrandPromptTestError struct{}

func (acceptedErrandPromptTestError) Error() string                { return "accepted parent prompt decision failed" }
func (acceptedErrandPromptTestError) DetachedResumeAccepted() bool { return true }

func TestErrandParentPromptErrorRetainsAcceptedOrUncertainEvidence(t *testing.T) {
	for _, test := range []struct {
		name     string
		err      error
		messages int
	}{
		{"refused", errors.New("not dispatched"), 0},
		{"accepted", acceptedErrandPromptTestError{}, 1},
		{"uncertain", uncertainErrandPromptTestError{}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, repo := newTestTaskService(t)
			_, task, parent := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
			child := &models.TaskSession{ID: "admission-child", TaskID: task.ID, Role: models.SessionRoleErrand,
				ParentSessionID: parent.ID, State: models.TaskSessionStateWaitingForInput}
			require.NoError(t, repo.CreateTaskSession(t.Context(), child))
			h, orch := newMessageTaskHandler(t, svc, repo)
			orch.promptErrFirst = test.err
			metadata := map[string]interface{}{}
			ctx := h.withErrandParentPrompt(t.Context(), task.ID, parent.ID, metadata)
			_, err := h.promptPreparedTaskMessage(ctx, task.ID, child, "next", metadata, nil)
			require.Error(t, err)
			messages, err := svc.ListMessages(t.Context(), child.ID)
			require.NoError(t, err)
			require.Len(t, messages, test.messages, "accepted or unknown dispatch must keep its transcript evidence")
		})
	}
}

func TestErrandParentQueueIsNotProviderAdmission(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, task, parent := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	child := &models.TaskSession{ID: "admission-child", TaskID: task.ID, Role: models.SessionRoleErrand,
		ParentSessionID: parent.ID, State: models.TaskSessionStateRunning}
	require.NoError(t, repo.CreateTaskSession(t.Context(), child))
	h, orch := newMessageTaskHandler(t, svc, repo)
	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{
		"task_id": task.ID, "session_id": child.ID, "prompt": "next",
		"sender_task_id": task.ID, "sender_session_id": parent.ID,
	})
	response, err := h.handleMessageTask(t.Context(), msg)
	require.NoError(t, err)
	var view map[string]interface{}
	require.NoError(t, json.Unmarshal(response.Payload, &view))
	require.Equal(t, "queued", view["status"])
	status := orch.queue.GetStatus(t.Context(), child.ID)
	require.Len(t, status.Entries, 1)
	require.Equal(t, parent.ID, status.Entries[0].Metadata["errand_parent_sender_session_id"], "retain verified sender for later dispatch")
}

func TestErrandParentPromptTrustsOnlySenderSessionOfSenderTask(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, task, parent := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	h, _ := newMessageTaskHandler(t, svc, repo)
	for _, test := range []struct {
		task, sender string
		trusted      bool
	}{
		{task.ID, parent.ID, true}, {"other-task", parent.ID, false},
		{task.ID, "unknown", false}, {task.ID, "", false},
	} {
		metadata := map[string]interface{}{errands.ParentPromptSenderMetadataKey: "forged"}
		ctx := h.withErrandParentPrompt(context.Background(), test.task, test.sender, metadata)
		origin := errands.ParentPromptOriginFromContext(ctx)
		if test.trusted {
			require.Equal(t, parent.ID, origin.SenderSessionID)
			require.NotEmpty(t, origin.SubmissionID)
			require.Equal(t, parent.ID, metadata[errands.ParentPromptSenderMetadataKey])
		} else {
			require.Empty(t, origin)
			require.NotContains(t, metadata, errands.ParentPromptSenderMetadataKey)
		}
	}
}
