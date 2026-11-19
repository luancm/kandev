package scope

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestErrandPrincipalProvesPersistedCurrentBinding(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "principal.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	repo, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "workspace"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "workspace", Title: "task"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "parent", TaskID: "task", ExecutorActorUserID: "actor"}))
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{ID: "child", TaskID: "task", ParentSessionID: "parent", Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone}))
	_, err = repo.ReserveErrandExecutionGeneration(ctx, "child", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "child", 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	binding := &models.ErrandHarnessBinding{ErrandID: "child", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 3, StreamID: "stream"}
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	owner := streams.MCPExecutionContext{TaskID: "task", WorkspaceID: "workspace", ErrandID: "child", ErrandGeneration: 1, ExecutionID: "execution", ParentIncarnationID: "incarnation", HarnessGeneration: 3, StreamID: "stream"}
	resolver := &Resolver{tasks: repo}
	scoped, err := resolver.ScopePrincipal(ctx, owner)
	require.NoError(t, err)
	actor, ok := models.ExecutorActorFromContext(scoped)
	require.True(t, ok)
	require.Equal(t, "actor", actor)
	foreign := owner
	foreign.StreamID = "foreign"
	_, err = resolver.ScopePrincipal(ctx, foreign)
	require.Error(t, err)
	binding2 := *binding
	binding2.StreamID = "successor"
	binding2.HarnessGeneration = 4
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &binding2, 1))
	_, err = resolver.ScopePrincipal(ctx, owner)
	require.Error(t, err)
	owner.StreamID = "successor"
	owner.HarnessGeneration = 4
	_, err = resolver.ScopePrincipal(ctx, owner)
	require.NoError(t, err)
}
