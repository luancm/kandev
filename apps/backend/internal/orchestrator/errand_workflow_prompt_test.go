package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An errand that is not autopilot must not receive the ask-question tool when a
// workflow step prompt is injected. The ordinary control session still gets it.
func TestInjectAutoStartRuntimeContext_ErrandOmitsAskTool(t *testing.T) {
	svc := &Service{}
	newState := func(role models.SessionRole) *autoStartStepPromptState {
		return &autoStartStepPromptState{
			session: &models.TaskSession{
				ID:    "session-1",
				State: models.TaskSessionStateCreated,
				Role:  role,
			},
			sessionID:   "session-1",
			taskID:      "task-1",
			step:        &wfmodels.WorkflowStep{},
			agentPrompt: "step prompt",
		}
	}

	errandState := newState(models.SessionRoleErrand)
	_, errandPrompt := svc.injectAutoStartRuntimeContext(
		errandState, "recorded", "dispatched", false, &models.Task{ID: "task-1"}, false, false, "",
	)
	require.Contains(t, errandPrompt, "list_workspaces_kandev")
	assert.NotContains(t, errandPrompt, "ask_user_question_kandev")
	assert.NotContains(t, errandPrompt, "create_task_kandev")

	ordinaryState := newState("")
	_, ordinaryPrompt := svc.injectAutoStartRuntimeContext(
		ordinaryState, "recorded", "dispatched", false, &models.Task{ID: "task-1"}, false, false, "",
	)
	assert.Contains(t, ordinaryPrompt, "ask_user_question_kandev")
}
