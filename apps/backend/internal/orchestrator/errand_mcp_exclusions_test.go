package orchestrator

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentsettingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/stretchr/testify/require"
)

type errandMCPStore interface {
	mcpconfig.CatalogRepository
	mcpconfig.SelectionRepository
}

func wireErrandMCPSelections(t *testing.T, f *errandLaunchFixture) (errandMCPStore, *mcpconfig.SelectionService) {
	t.Helper()
	db := sqlx.NewDb(f.repo.DB(), "sqlite3")
	settings, cleanup, err := agentsettingsstore.Provide(db, db, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })
	require.NoError(t, settings.CreateAgent(context.Background(), &agentsettingsmodels.Agent{ID: "mcp-agent", Name: "mcp-agent"}))
	for _, id := range []string{"parent-profile", "errand-profile"} {
		require.NoError(t, settings.CreateAgentProfile(context.Background(), &agentsettingsmodels.AgentProfile{
			ID: id, AgentID: "mcp-agent", Name: id, Enabled: true,
		}))
	}
	selections := mcpconfig.NewSelectionService(settings, settings)
	selections.SetSessionMCPStateRepository(settings)
	selections.SetSessionResolutionContext(func(ctx context.Context, workspaceID, sessionID string) (mcpconfig.ResolutionContext, error) {
		session, err := f.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return mcpconfig.ResolutionContext{}, err
		}
		return mcpconfig.ResolutionContext{WorkspaceID: workspaceID, SessionID: sessionID,
			TaskID: session.TaskID, ProfileID: session.AgentProfileID}, nil
	})
	f.svc.SetMCPSelectionReader(selections)
	f.svc.SetMCPSelectionWriter(selections)
	return settings, selections
}

func TestErrandMCPExclusionsSurviveParentEditsAndDifferentChildProfile(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	settings, selections := wireErrandMCPSelections(t, f)
	for _, id := range []string{"shared", "parent-only", "child-only", "added"} {
		require.NoError(t, settings.CreateMCPServerDefinition(ctx, &mcpconfig.MCPServerDefinition{
			ID: id, WorkspaceID: "ws1", RuntimeName: id, NormalizedRuntimeName: id,
			DisplayName: id, Enabled: true, Revision: 1, Source: mcpconfig.DefinitionSourceCustom,
			ExecutionMode: mcpconfig.ExecutionModeRemote, Transport: mcpconfig.ServerTypeHTTP,
			Configuration: mcpconfig.MCPServerConfiguration{URL: "https://mcp.example.test/" + id},
		}))
	}
	require.NoError(t, selections.Replace(ctx, mcpconfig.SelectionScopeProfile, "ws1", "parent-profile", []string{"shared", "parent-only"}))
	require.NoError(t, selections.Replace(ctx, mcpconfig.SelectionScopeProfile, "ws1", "errand-profile", []string{"shared", "child-only"}))
	origin := mcpconfig.SelectionOrigin{Scope: mcpconfig.SelectionScopeTaskSession, WorkspaceID: "ws1", OwnerID: f.parent}
	revision, excluded := int64(0), []string{"shared", "parent-only"}
	_, err := selections.ReplaceConditional(ctx, origin, []string{"added", "parent-only"}, mcpconfig.SelectionCondition{
		ExpectedRevision: &revision, ExcludedDefinitionIDs: &excluded,
	})
	require.NoError(t, err)
	snapshot, err := NewErrandAdapter(f.svc).SessionSnapshot(ctx, f.taskID, f.parent)
	require.NoError(t, err)
	require.ElementsMatch(t, excluded, snapshot.ExcludedDefinitionIDs)
	parent, err := selections.ReadSnapshot(ctx, origin)
	require.NoError(t, err)
	revision, excluded = parent.State.DesiredRevision, []string{}
	_, err = selections.ReplaceConditional(ctx, origin, nil, mcpconfig.SelectionCondition{
		ExpectedRevision: &revision, ExcludedDefinitionIDs: &excluded,
	})
	require.NoError(t, err)
	request := f.request()
	request.MCPServerIDs, request.MCPExcludedServerIDs = snapshot.DefinitionIDs, snapshot.ExcludedDefinitionIDs
	_, err = f.svc.LaunchErrandSession(ctx, request)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"added", "parent-only"}, snapshot.DefinitionIDs, "launch must not mutate the stored snapshot")
	child, err := selections.ReadSnapshot(ctx, mcpconfig.SelectionOrigin{
		Scope: mcpconfig.SelectionScopeTaskSession, WorkspaceID: "ws1", OwnerID: request.ErrandID,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"shared"}, child.ExcludedDefinitionIDs)
	resolved, err := mcpconfig.NewResolver(settings, settings).Resolve(ctx, mcpconfig.ResolutionContext{
		WorkspaceID: "ws1", SessionID: request.ErrandID, TaskID: f.taskID, ProfileID: request.AgentProfileID,
	}, mcpconfig.Policy{AllowHTTP: true})
	require.NoError(t, err)
	var names []string
	for _, server := range resolved.Servers {
		names = append(names, server.Name)
	}
	require.ElementsMatch(t, []string{"child-only", "added"}, names)
}
