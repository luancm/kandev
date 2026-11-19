package backendapp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/mcp/handlers"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestErrandCatalogListWorkspacesWithCollaboration(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "catalog.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	repo, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	for _, id := range []string{"own", "foreign"} {
		require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: id, Name: id}))
	}
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "own", Title: "task"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "parent", TaskID: "task"}))
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{ID: "child", TaskID: "task", ParentSessionID: "parent", Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone}))
	_, err = repo.ReserveErrandExecutionGeneration(ctx, "child", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "child", 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	binding := &models.ErrandHarnessBinding{ErrandID: "child", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream"}
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	execution := streams.MCPExecutionContext{TaskID: "task", WorkspaceID: "own", ErrandID: "child", ErrandGeneration: 1, ExecutionID: "execution", ParentIncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream"}
	log := logger.Default()
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	svc := taskservice.NewService(taskservice.Repos{Workspaces: repo, Tasks: repo, Sessions: repo}, eventBus, log, taskservice.RepositoryDiscoveryConfig{})
	svc.SetWorkspaceCollaboration(&workspaceCollaborationBoundary{enabled: true, store: repo}, repo)
	h := handlers.NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, log)
	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)
	resolver := mcpscope.NewResolver(repo, nil, func() bool { return false }, log)
	scoped, err := resolver.ScopePrincipal(streams.WithMCPExecutionContext(ctx, execution), execution)
	require.NoError(t, err)
	response, err := dispatcher.Dispatch(scoped, &ws.Message{ID: "1", Action: ws.ActionMCPListWorkspaces})
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	var payload struct {
		Workspaces []struct {
			ID string `json:"id"`
		} `json:"workspaces"`
	}
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.Len(t, payload.Workspaces, 1)
	require.Equal(t, "own", payload.Workspaces[0].ID)
	successor := *binding
	successor.HarnessGeneration = 2
	successor.StreamID = "successor"
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &successor, 1))
	response, err = dispatcher.Dispatch(scoped, &ws.Message{ID: "2", Action: ws.ActionMCPListWorkspaces})
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
}
