package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The errand block is the trusted errand prompt, and an errand's first turn
// does not advertise the parent or question tools that an errand never has.
func TestErrandLaunchPromptUsesErrandPromptAndErrandSessionContext(t *testing.T) {
	errand := &ErrandLaunch{ErrandID: "errand-1", ParentSessionID: "parent-1", Title: "Check logs"}
	got := (&Service{}).applyLaunchPromptContext(context.Background(), launchPromptContext{
		prompt:         "Find the failing test",
		taskID:         "task-1",
		sessionID:      "errand-1",
		configMode:     false,
		errand:         errand,
		includeErrands: true,
	})

	require.Contains(t, got, "errand of session parent-1")
	assert.Contains(t, got, "Find the failing test")
	assert.NotContains(t, got, "start_errand_kandev")
	assert.NotContains(t, got, "ask_user_question_kandev")
	assert.NotContains(t, got, "Optional: session_id, delivery_mode")
}

func TestParentLaunchPromptAdvertisesErrandToolsAndProfiles(t *testing.T) {
	got := (&Service{}).applyLaunchPromptContext(context.Background(), launchPromptContext{
		prompt:         "Plan the release",
		taskID:         "task-1",
		sessionID:      "session-1",
		includeErrands: true,
		errandProfiles: []sysprompt.ErrandProfile{{Name: "Fast", Agent: "claude", Model: "sonnet"}},
	})

	assert.Contains(t, got, "start_errand_kandev")
	assert.Contains(t, got, "Fast (claude, sonnet)")
	assert.Contains(t, got, "Errands are the default way to delegate bounded work")
}

func TestParentErrandPromptOptionsStayOffWithoutExecutor(t *testing.T) {
	enabled, profiles := (&Service{}).parentErrandPromptOptions(context.Background(), "task-1", nil)
	assert.False(t, enabled)
	assert.Nil(t, profiles)
}
