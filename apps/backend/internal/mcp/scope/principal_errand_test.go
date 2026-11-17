package scope

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type errandPrincipalLookup struct {
	principalLookup
	row   *models.Errand
	stale bool
}

func (l errandPrincipalLookup) GetErrand(context.Context, string) (*models.Errand, error) {
	return l.row, nil
}
func (l errandPrincipalLookup) ErrandHarnessBindingStale(_ context.Context, task string, binding *models.ErrandHarnessBinding) (bool, error) {
	return l.stale || task != "task" || binding.ErrandID != "child" || binding.Generation != 2 || binding.ExecutionID != "execution" || binding.IncarnationID != "incarnation" || binding.HarnessGeneration != 3 || binding.StreamID != "stream", nil
}
func TestErrandPrincipalRequiresCurrentOwnerAndParentMembership(t *testing.T) {
	owner := streams.MCPExecutionContext{TaskID: "task", WorkspaceID: "workspace", ErrandID: "child", ErrandGeneration: 2, ExecutionID: "execution", ParentIncarnationID: "incarnation", HarnessGeneration: 3, StreamID: "stream"}
	lookup := errandPrincipalLookup{principalLookup: principalLookup{task: &models.Task{ID: "task", WorkspaceID: "workspace", Origin: models.TaskOriginCoordinator}, workspace: &models.Workspace{ID: "workspace"}, session: &models.TaskSession{ID: "parent", TaskID: "task", ExecutorActorUserID: "actor"}}, row: &models.Errand{ID: "child", TaskID: "task", ParentSessionID: "parent"}}
	ctx, err := (&Resolver{tasks: lookup}).ScopePrincipal(context.Background(), owner)
	require.NoError(t, err)
	principal, ok := PrincipalFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, mcpprofile.SurfaceErrand, principal.Surface)
	require.Equal(t, "child", principal.ErrandID)
	require.Empty(t, principal.CallerSessionID)
	actor, ok := models.ExecutorActorFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "actor", actor)
	require.False(t, principal.IsCoordinator())
	require.False(t, principal.IsAutomation())
	for _, mutate := range []func(*streams.MCPExecutionContext){func(e *streams.MCPExecutionContext) { e.SessionID = "forged" }, func(e *streams.MCPExecutionContext) { e.StreamID = "foreign" }, func(e *streams.MCPExecutionContext) { e.WorkspaceID = "foreign" }, func(e *streams.MCPExecutionContext) { e.ErrandGeneration = 0 }} {
		invalid := owner
		mutate(&invalid)
		_, err := (&Resolver{tasks: lookup}).ScopePrincipal(context.Background(), invalid)
		require.Error(t, err)
	}
	lookup.stale = true
	_, err = (&Resolver{tasks: lookup}).ScopePrincipal(context.Background(), owner)
	require.Error(t, err)
	lookup.stale = false
	lookup.row = &models.Errand{ID: "child", TaskID: "foreign"}
	_, err = (&Resolver{tasks: lookup}).ScopePrincipal(context.Background(), owner)
	require.Error(t, err)
	lookup.row = &models.Errand{ID: "child", TaskID: "task", ParentSessionID: "parent"}
	lookup.session = &models.TaskSession{ID: "parent", TaskID: "foreign"}
	_, err = (&Resolver{tasks: lookup}).ScopePrincipal(context.Background(), owner)
	require.Error(t, err)
}
