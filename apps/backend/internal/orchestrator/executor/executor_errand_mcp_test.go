package executor

import (
	"context"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestResolveTaskSessionMCPProfile_ErrandsCapabilityFollowsFlagAndEligibility(t *testing.T) {
	tests := []struct {
		name      string
		task      *models.Task
		session   *models.TaskSession
		flag      bool
		wantGrant bool
	}{
		{
			name:      "ordinary kanban session with flag on",
			task:      &models.Task{ID: "task"},
			session:   &models.TaskSession{ID: "session", TaskID: "task"},
			flag:      true,
			wantGrant: true,
		},
		{
			name:      "ordinary kanban session with flag off",
			task:      &models.Task{ID: "task"},
			session:   &models.TaskSession{ID: "session", TaskID: "task"},
			flag:      false,
			wantGrant: false,
		},
		{
			name:      "passthrough session",
			task:      &models.Task{ID: "task"},
			session:   &models.TaskSession{ID: "session", TaskID: "task", IsPassthrough: true},
			flag:      true,
			wantGrant: false,
		},
		{
			name:      "ephemeral task",
			task:      &models.Task{ID: "task", IsEphemeral: true},
			session:   &models.TaskSession{ID: "session", TaskID: "task"},
			flag:      true,
			wantGrant: false,
		},
		{
			name:      "office task",
			task:      &models.Task{ID: "task", IsFromOffice: true},
			session:   &models.TaskSession{ID: "session", TaskID: "task"},
			flag:      true,
			wantGrant: false,
		},
		{
			name:      "automation task",
			task:      &models.Task{ID: "task", Origin: models.TaskOriginAutomationRun},
			session:   &models.TaskSession{ID: "session", TaskID: "task"},
			flag:      true,
			wantGrant: false,
		},
		{
			name:      "configuration session",
			task:      &models.Task{ID: "task"},
			session:   &models.TaskSession{ID: "session", TaskID: "task", Metadata: map[string]interface{}{"config_mode": true}},
			flag:      true,
			wantGrant: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepository()
			repo.tasks[tt.task.ID] = tt.task
			repo.sessions[tt.session.ID] = tt.session
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			exec.SetErrandsEnabled(func() bool { return tt.flag })

			profile, err := exec.ResolveTaskSessionMCPProfile(context.Background(), tt.task.ID, tt.session, true)
			require.NoError(t, err)
			require.Equal(t, tt.wantGrant, profile.HasCapability(mcpprofile.CapabilityErrands))
		})
	}
}

func TestResolveTaskSessionMCPProfile_ErrandsCapabilityReadsFlagPerResolution(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["task"] = &models.Task{ID: "task"}
	session := &models.TaskSession{ID: "session", TaskID: "task"}
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	enabled := false
	exec.SetErrandsEnabled(func() bool { return enabled })

	profile, err := exec.ResolveTaskSessionMCPProfile(context.Background(), "task", session, true)
	require.NoError(t, err)
	require.False(t, profile.HasCapability(mcpprofile.CapabilityErrands))

	enabled = true
	profile, err = exec.ResolveTaskSessionMCPProfile(context.Background(), "task", session, true)
	require.NoError(t, err)
	require.True(t, profile.HasCapability(mcpprofile.CapabilityErrands))
}
