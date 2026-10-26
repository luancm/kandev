package backendapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// The composition root joins four packages through narrow interfaces; these
// assertions fail the build when one side drifts.
var (
	_ errands.Store                   = (*sqliterepo.Repository)(nil)
	_ errands.Launcher                = (*orchestrator.ErrandAdapter)(nil)
	_ errands.SessionStopper          = (*orchestrator.ErrandAdapter)(nil)
	_ errands.MCPSelections           = (*orchestrator.ErrandAdapter)(nil)
	_ orchestrator.ErrandController   = (*errands.Service)(nil)
	_ taskhandlers.ErrandService      = (*errands.Service)(nil)
	_ orchestrator.MCPSelectionReader = (*mcpconfig.SelectionService)(nil)
	_ taskservice.TaskErrandCanceller = (*orchestrator.Service)(nil)
	_ errandExecutorValidator         = (*taskservice.Service)(nil)
)

type fakeErrandProfiles struct {
	agents   []*agentsettingsmodels.Agent
	profiles map[string][]*agentsettingsmodels.AgentProfile
	err      error
}

func (f fakeErrandProfiles) ListAgents(context.Context) ([]*agentsettingsmodels.Agent, error) {
	return f.agents, f.err
}

func (f fakeErrandProfiles) ListAgentProfiles(_ context.Context, agentID string) ([]*agentsettingsmodels.AgentProfile, error) {
	return f.profiles[agentID], nil
}

type fakeErrandTasks struct{ workspaceID string }

func (f fakeErrandTasks) GetTask(_ context.Context, id string) (*models.Task, error) {
	if id == "missing" {
		return nil, errors.New("not found")
	}
	return &models.Task{ID: id, WorkspaceID: f.workspaceID}, nil
}

type fakeErrandValidator struct{ refuse map[string]bool }

func (f fakeErrandValidator) ValidateAgentProfileForTaskExecutor(_ context.Context, _, profileID string) error {
	if f.refuse[profileID] {
		return errors.New("incompatible")
	}
	return nil
}

func TestErrandProfileCatalogListsOnlyEnabledProfilesOfTheTasksWorkspace(t *testing.T) {
	deleted := time.Now()
	catalog := errandProfileCatalog{
		profiles: fakeErrandProfiles{
			agents: []*agentsettingsmodels.Agent{{ID: "agent-1"}, {ID: "agent-2"}},
			profiles: map[string][]*agentsettingsmodels.AgentProfile{
				"agent-1": {
					{ID: "global", Name: "Global", Enabled: true},
					{ID: "mine", Name: "Mine", Enabled: true, WorkspaceID: "ws-1"},
					{ID: "foreign", Name: "Foreign", Enabled: true, WorkspaceID: "ws-2"},
					{ID: "off", Name: "Off", Enabled: false},
				},
				"agent-2": {
					{ID: "pty", Name: "Terminal", Enabled: true, CLIPassthrough: true},
					{ID: "gone", Name: "Gone", Enabled: true, DeletedAt: &deleted},
				},
			},
		},
		tasks:     fakeErrandTasks{workspaceID: "ws-1"},
		validator: fakeErrandValidator{},
	}
	infos, err := catalog.ListTaskProfiles(context.Background(), "task-1")
	require.NoError(t, err)
	require.Equal(t, []errands.ProfileInfo{
		{ID: "global", Name: "Global"},
		{ID: "mine", Name: "Mine"},
		{ID: "pty", Name: "Terminal", Passthrough: true},
	}, infos)

	_, err = catalog.ListTaskProfiles(context.Background(), "missing")
	require.Error(t, err)
}

func TestErrandProfileResolutionRefusesPassthroughAndExecutorIncompatibleProfiles(t *testing.T) {
	catalog := errandProfileCatalog{
		profiles: fakeErrandProfiles{
			agents: []*agentsettingsmodels.Agent{{ID: "agent-1"}},
			profiles: map[string][]*agentsettingsmodels.AgentProfile{"agent-1": {
				{ID: "ok", Name: "Reviewer", Enabled: true},
				{ID: "pty", Name: "Terminal", Enabled: true, CLIPassthrough: true},
				{ID: "remote", Name: "Remote only", Enabled: true},
			}},
		},
		tasks:     fakeErrandTasks{workspaceID: "ws-1"},
		validator: fakeErrandValidator{refuse: map[string]bool{"remote": true}},
	}
	resolver := errands.NewProfileResolver(catalog)
	ctx := context.Background()

	resolved, err := resolver.ResolveProfile(ctx, "task-1", "Reviewer", "")
	require.NoError(t, err)
	require.Equal(t, errands.ResolvedProfile{ID: "ok", Name: "Reviewer"}, resolved)
	for _, selector := range []string{"pty", "Terminal", "remote"} {
		_, err = resolver.ResolveProfile(ctx, "task-1", selector, "")
		reason, typed := errands.ReasonOf(err)
		require.True(t, typed, selector)
		require.Equal(t, errands.ReasonProfileIncompatible, reason, selector)
	}
}
