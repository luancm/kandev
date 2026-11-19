package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type collaborationErrandLookup struct {
	*fakeCollaborationLookup
	stale bool
}

func (l *collaborationErrandLookup) GetErrand(context.Context, string) (*models.Errand, error) {
	return &models.Errand{ID: "child", TaskID: "caller-task"}, nil
}
func (l *collaborationErrandLookup) ErrandHarnessBindingStale(context.Context, string, *models.ErrandHarnessBinding) (bool, error) {
	return l.stale, nil
}
func TestWorkspaceCollaborationErrandUsesOwnedTask(t *testing.T) {
	lookup := &collaborationErrandLookup{fakeCollaborationLookup: &fakeCollaborationLookup{task: &models.Task{ID: "caller-task", WorkspaceID: "source-ws"}, workspace: &models.Workspace{ID: "source-ws"}}}
	boundary := &workspaceCollaborationBoundary{enabled: true, store: lookup}
	execution := streams.MCPExecutionContext{TaskID: "caller-task", WorkspaceID: "source-ws", ErrandID: "child", ErrandGeneration: 1, ExecutionID: "execution", ParentIncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream"}
	ctx := streams.WithMCPExecutionContext(context.Background(), execution)
	ctx = mcpscope.WithPrincipal(ctx, mcpscope.Principal{CallerTaskID: "caller-task", WorkspaceID: "source-ws", ErrandID: "child", Surface: mcpprofile.SurfaceErrand})
	require.NoError(t, boundary.Authorize(ctx, "source-ws", models.CollaborationRead))
	require.Error(t, boundary.Authorize(ctx, "foreign", models.CollaborationRead))
	lookup.stale = true
	require.Error(t, boundary.Authorize(ctx, "source-ws", models.CollaborationRead))
}
