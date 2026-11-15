package executor

import (
	"context"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestResolveTaskSessionMCPMode_ErrandSessionIsErrandMode(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	repo.tasks["task-errand"] = &models.Task{ID: "task-errand"}
	session := &models.TaskSession{ID: "session-errand", TaskID: "task-errand", Role: models.SessionRoleErrand}
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	mode, err := exec.resolveTaskSessionMCPMode(ctx, "task-errand", session, true)
	require.NoError(t, err)
	require.Equal(t, McpModeErrand, mode)
}

func TestResolveTaskSessionMCPMode_ErrandSessionWinsOverTaskState(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	// The errand's task is the parent's task. It may carry title-pending
	// metadata, but the errand catalog still applies to the errand session.
	repo.tasks["task-pending"] = &models.Task{
		ID:       "task-pending",
		Metadata: map[string]interface{}{models.MetaKeyAgentTitlePending: true, models.MetaKeyAgentTitleOwnerSessionID: "errand-session"},
	}
	session := &models.TaskSession{ID: "errand-session", TaskID: "task-pending", Role: models.SessionRoleErrand}
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	mode, err := exec.resolveTaskSessionMCPMode(ctx, "task-pending", session, true)
	require.NoError(t, err)
	require.Equal(t, McpModeErrand, mode)
}

func TestResolveTaskSessionMCPProfile_ErrandSessionUsesErrandSurface(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["task-errand"] = &models.Task{ID: "task-errand"}
	session := &models.TaskSession{ID: "session-errand", TaskID: "task-errand", Role: models.SessionRoleErrand}
	repo.sessions[session.ID] = session
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetErrandsEnabled(func() bool { return true })
	exec.SetCanvasesEnabled(true)

	profile, err := exec.ResolveTaskSessionMCPProfile(context.Background(), "task-errand", session, true)
	require.NoError(t, err)
	require.Equal(t, mcpprofile.SurfaceErrand, profile.Surface)
	require.False(t, profile.HasCapability(mcpprofile.CapabilityErrands))
	require.False(t, profile.HasCapability(mcpprofile.CapabilityUserQuestion))
	require.False(t, profile.HasCapability(mcpprofile.CapabilityCanvas))
}

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
