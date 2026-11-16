package handlers

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
)

// A follow-up message to an errand must carry the read-only errand context and
// never the ask-question tool, even when the errand is not autopilot. An
// ordinary session keeps its task-mode context with the ask tool.
func TestInjectMessageContextErrandOmitsAskTool(t *testing.T) {
	h := &MessageHandlers{}
	task := &models.Task{ID: "task-1"}
	req := wsAddMessageRequest{TaskID: "task-1", TaskSessionID: "errand-1"}

	errandResp := &dto.GetTaskSessionResponse{Session: dto.TaskSessionDTO{Role: models.SessionRoleErrand}}
	errandContent := h.injectMessageContext(context.Background(), req, errandResp, task,
		false, false, false, false, "follow-up", "")
	assert.Contains(t, errandContent, "list_workspaces_kandev")
	assert.NotContains(t, errandContent, "ask_user_question_kandev")
	assert.NotContains(t, errandContent, "create_task_kandev")

	ordinaryResp := &dto.GetTaskSessionResponse{Session: dto.TaskSessionDTO{}}
	ordinaryContent := h.injectMessageContext(context.Background(), req, ordinaryResp, task,
		false, false, false, false, "follow-up", "")
	assert.Contains(t, ordinaryContent, "ask_user_question_kandev")
}
