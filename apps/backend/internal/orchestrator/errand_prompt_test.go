package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/stretchr/testify/assert"
)

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
