package config

import (
	"testing"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestErrandModeIgnoresInheritedManagedProfile(t *testing.T) {
	inherited := mcpprofile.New(mcpprofile.SurfaceConfiguration, nil, nil)
	cfg, err := (&Config{}).NewInstanceConfig(12345, &InstanceOverrides{TaskID: "", SessionID: "errand:child", McpMode: "errand", McpProfile: &inherited})
	require.NoError(t, err)
	require.NotNil(t, cfg.McpProfile)
	require.Equal(t, mcpprofile.SurfaceErrand, cfg.McpProfile.Surface)
	require.Equal(t, mcpprofile.SurfaceConfiguration, inherited.Surface)
}

func TestErrandProjectionPreservesTransportAndSelectsLogicalIdentity(t *testing.T) {
	owner := &agentctltypes.ErrandMCPOwner{TaskID: "task", ErrandID: "child", Generation: 2}
	inherited := mcpprofile.New(mcpprofile.SurfaceConfiguration, nil, nil)
	cfg, err := (&Config{}).NewInstanceConfig(12345, &InstanceOverrides{TaskID: "", SessionID: "errand:child", ErrandMCPOwner: owner, McpProfile: &inherited})
	require.NoError(t, err)
	require.Equal(t, "errand:child", cfg.SessionID)
	task, session := cfg.MCPIdentity()
	require.Equal(t, "task", task)
	require.Empty(t, session)
	require.Equal(t, mcpprofile.SurfaceErrand, cfg.McpProfile.Surface)
	owner.TaskID = "changed"
	require.Equal(t, "task", cfg.ErrandMCPOwner.TaskID)
	require.Equal(t, mcpprofile.SurfaceConfiguration, inherited.Surface)
}
