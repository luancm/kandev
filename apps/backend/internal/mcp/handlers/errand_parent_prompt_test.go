package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kandev/kandev/internal/task/models"
)

type recordingErrandPromptRecorder struct {
	calls [][2]string
}

func (r *recordingErrandPromptRecorder) OnErrandParentPromptAdmitted(_ context.Context, errandSessionID, senderSessionID string) error {
	r.calls = append(r.calls, [2]string{errandSessionID, senderSessionID})
	return nil
}

func TestRecordErrandParentPromptTrustsOnlySenderSessionOfSenderTask(t *testing.T) {
	svc, repo := newTestTaskService(t)
	_, target, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	h, _ := newMessageTaskHandler(t, svc, repo)
	recorder := &recordingErrandPromptRecorder{}
	h.SetErrandParentPromptRecorder(recorder)
	ctx := context.Background()

	h.recordErrandParentPrompt(ctx, target.ID, primary.ID, "errand-session")
	assert.Equal(t, [][2]string{{"errand-session", primary.ID}}, recorder.calls)

	h.recordErrandParentPrompt(ctx, "some-other-task", primary.ID, "errand-session")
	h.recordErrandParentPrompt(ctx, target.ID, "unknown-session", "errand-session")
	h.recordErrandParentPrompt(ctx, target.ID, "", "errand-session")
	assert.Len(t, recorder.calls, 1, "a sender session not owned by the sender task must not be trusted")
}

func TestRecordErrandParentPromptWithoutRecorderIsNoOp(t *testing.T) {
	svc, repo := newTestTaskService(t)
	h, _ := newMessageTaskHandler(t, svc, repo)
	assert.NotPanics(t, func() {
		h.recordErrandParentPrompt(context.Background(), "task", "session", "errand-session")
	})
}
