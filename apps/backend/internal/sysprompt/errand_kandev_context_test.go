package sysprompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errandReadOnlyCatalog is the fixed errand catalog of AC-TASKS-ERRANDS-009.2.
var errandReadOnlyCatalog = []string{
	"list_workspaces_kandev",
	"list_workflows_kandev",
	"list_workflow_steps_kandev",
	"list_tasks_kandev",
	"list_related_tasks_kandev",
	"list_task_sessions_kandev",
	"get_task_conversation_kandev",
	"get_task_plan_kandev",
}

// errandForbiddenTools are task-mode tools an errand never has. Any of them
// named in an errand's context would invite a call that fails or escapes its
// read-only boundary.
var errandForbiddenTools = []string{
	"create_task_kandev",
	"spawn_session_kandev",
	"message_task_kandev",
	"stop_task_kandev",
	"set_task_title_kandev",
	"create_task_plan_kandev",
	"update_task_plan_kandev",
	"edit_task_plan_kandev",
	"step_complete_kandev",
	"ask_user_question_kandev",
	"ask_parent_question_kandev",
	"show_rich_output_kandev",
	"show_walkthrough_kandev",
	"get_walkthrough_kandev",
	"delete_walkthrough_kandev",
	"create_canvas_kandev",
	"start_errand_kandev",
	"await_errands_kandev",
	"cancel_errand_kandev",
}

func TestErrandKandevContextListsExactlyTheReadOnlyCatalog(t *testing.T) {
	got := FormatErrandKandevContext("task-1", "session-1")

	for _, name := range errandReadOnlyCatalog {
		assert.Contains(t, got, "- "+name+":", "catalog tool %s must be listed", name)
	}
	for _, name := range errandForbiddenTools {
		assert.NotContains(t, got, name, "errand context must not name %s", name)
	}
}

func TestErrandKandevContextKeepsTaskMarkerAndIdentity(t *testing.T) {
	got := FormatErrandKandevContext("task-1", "session-1")

	assert.Contains(t, got, kandevContextMarker)
	assert.Contains(t, got, "Kandev Task ID: task-1")
	assert.Contains(t, got, "Kandev Session ID: session-1")
	assert.NotContains(t, got, "{")
	assert.NotContains(t, got, officeContextMarker)
}

// The first-turn injector keeps the errand context only when it recognizes the
// task marker, so the errand render must still canonicalize as a task block.
func TestInjectedErrandKandevContextIsTheCanonicalTaskBlock(t *testing.T) {
	got := InjectKandevContextWithOptions("task-1", "session-1", "Find the failing test", KandevContextOptions{
		ErrandSession:           true,
		IncludeUserQuestionTool: true,
	})

	require.True(t, strings.HasPrefix(got, Wrap(FormatErrandKandevContext("task-1", "session-1"))))
	assert.True(t, strings.HasSuffix(got, "Find the failing test"))
	assert.Equal(t, 1, strings.Count(got, TagStart))
}

// An errand never gets the ask tool, even when a caller requests it for a
// session that is not autopilot. Every errand follow-up path uses this render.
func TestErrandSessionNeverShowsTheAskToolForNonAutopilot(t *testing.T) {
	for name, options := range map[string]KandevContextOptions{
		"user question requested": {ErrandSession: true, IncludeUserQuestionTool: true},
		"parent question requested": {
			ErrandSession: true, IncludeParentQuestionTool: true,
		},
		"no question flags, not autopilot": {ErrandSession: true},
	} {
		t.Run(name, func(t *testing.T) {
			got := FormatKandevContextWithOptions("task-1", "session-1", options)

			assert.NotContains(t, got, "ask_user_question_kandev")
			assert.NotContains(t, got, "ask_parent_question_kandev")
		})
	}
}
