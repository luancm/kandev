package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestErrandMCPResolutionUsesSavedAdditionsAndExclusions(t *testing.T) {
	assertErrandMCPResolution(t, newTestRepo(t).(*sqliteRepository))
}

func TestPostgresErrandMCPResolutionUsesSavedAdditionsAndExclusions(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(database, database, nil)
	require.NoError(t, err)
	assertErrandMCPResolution(t, repo)
}

func assertErrandMCPResolution(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	ctx := t.Context()
	added := testMCPDefinition()
	excluded := testMCPDefinition()
	excluded.ID, excluded.RuntimeName, excluded.NormalizedRuntimeName = "excluded", "excluded", "excluded"
	for _, definition := range []*mcpconfig.MCPServerDefinition{added, excluded} {
		definition.ExecutionMode = mcpconfig.ExecutionModeExistingExecutable
		definition.Transport = mcpconfig.ServerTypeStdio
		definition.Configuration = mcpconfig.MCPServerConfiguration{Command: "test-mcp"}
		definition.SecretBindings = nil
	}
	require.NoError(t, repo.CreateMCPServerDefinition(ctx, added))
	require.NoError(t, repo.CreateMCPServerDefinition(ctx, excluded))
	require.NoError(t, repo.ReplaceMCPSelections(ctx, mcpconfig.SelectionScopeTask, added.WorkspaceID, "task", []string{excluded.ID}))
	selection := &mcpconfig.ErrandMCPContribution{
		ErrandID: "errand", WorkspaceID: added.WorkspaceID, TaskID: "task",
		Snapshot: mcpconfig.SelectionSnapshot{DefinitionIDs: []string{added.ID}, ExcludedDefinitionIDs: []string{excluded.ID}},
	}
	resolver := mcpconfig.NewResolver(repo, repo)
	rc := mcpconfig.ResolutionContext{WorkspaceID: added.WorkspaceID, TaskID: "task", Errand: selection}
	result, err := resolver.Resolve(ctx, rc, mcpconfig.Policy{AllowStdio: true})
	require.NoError(t, err)
	require.Len(t, result.Servers, 1)
	require.Equal(t, added.ID, result.Servers[0].DefinitionID)
	require.Equal(t, []mcpconfig.SelectionOrigin{{Scope: mcpconfig.SelectionScope("errand"), WorkspaceID: added.WorkspaceID, OwnerID: selection.ErrandID}}, result.Servers[0].Origins)
	require.Len(t, result.Decisions, 1)
	require.Equal(t, excluded.ID, result.Decisions[0].DefinitionID)
	require.Equal(t, "session_excluded", result.Decisions[0].ReasonCode)

	selection.WorkspaceID = "another-workspace"
	_, err = resolver.Resolve(ctx, rc, mcpconfig.Policy{AllowStdio: true})
	require.ErrorIs(t, err, mcpconfig.ErrMCPInvalidSelection)
}

func TestMCPSetupProjectionUsesUnionAndPreservesUnavailableState(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	ctx := context.Background()
	d := testMCPDefinition()
	if err := repo.CreateMCPServerDefinition(ctx, d); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceMCPSelections(ctx, mcpconfig.SelectionScopeProfile, d.WorkspaceID, "profile", []string{d.ID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceMCPSelections(ctx, mcpconfig.SelectionScopeTask, d.WorkspaceID, "task", []string{d.ID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAcceptedMCPSetup(ctx, "session", 0, "initial", &mcpconfig.MCPSetupSnapshot{Version: 1, Servers: []mcpconfig.MCPSetupServer{}}); err != nil {
		t.Fatal(err)
	}
	service := mcpconfig.NewSelectionService(repo, repo)
	service.SetSessionMCPStateRepository(repo)
	service.SetOwnerValidator(func(_ context.Context, _ mcpconfig.SelectionScope, workspaceID, _ string) error {
		if workspaceID != d.WorkspaceID {
			return errors.New("denied")
		}
		return nil
	})
	service.SetSessionResolutionContext(func(_ context.Context, workspaceID, sessionID string) (mcpconfig.ResolutionContext, error) {
		return mcpconfig.ResolutionContext{WorkspaceID: workspaceID, SessionID: sessionID, ProfileID: "profile", TaskID: "task"}, nil
	})
	state, err := service.SessionState(ctx, d.WorkspaceID, "session")
	if err != nil {
		t.Fatal(err)
	}
	if state.AcceptedSetup != nil || state.Comparison == nil || state.Comparison.Status != "known" || len(state.Comparison.Changes) != 1 || state.Comparison.Changes[0].Action != "add" {
		t.Fatalf("projection = %#v", state)
	}
	if _, err := service.SessionState(ctx, "other", "session"); !errors.Is(err, mcpconfig.ErrMCPSelectionOwnerAccess) {
		t.Fatalf("cross workspace = %v", err)
	}
	inputs, err := repo.ReadMCPResolutionInputs(ctx, mcpconfig.ResolutionContext{WorkspaceID: d.WorkspaceID, SessionID: "session", ProfileID: "profile", TaskID: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs.Definitions) != 1 || len(inputs.Origins[d.ID]) != 2 || inputs.DesiredRevision != 0 {
		t.Fatalf("union = %#v", inputs)
	}
	service.SetSessionResolutionContext(func(context.Context, string, string) (mcpconfig.ResolutionContext, error) {
		return mcpconfig.ResolutionContext{}, errors.New("private error")
	})
	state, err = service.SessionState(ctx, d.WorkspaceID, "session")
	if err != nil {
		t.Fatal(err)
	}
	if state.Comparison.Status != "unavailable" || state.Comparison.Reason != "context_unavailable" {
		t.Fatalf("fallback = %#v", state.Comparison)
	}
}

func TestMCPSetupMigrationKeepsLegacyUnknownAndEmptyKnown(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	ctx := context.Background()
	if err := repo.SaveMCPSelectionState(ctx, "legacy", mcpconfig.SessionMCPSelectionState{DesiredRevision: 2, AppliedRevision: 1, ApplyState: mcpconfig.SessionMCPApplyStatePendingIdle}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`ALTER TABLE mcp_task_session_apply_state DROP COLUMN accepted_setup_json`); err != nil {
		t.Fatal(err)
	}
	if err := repo.initSchema(); err != nil {
		t.Fatal(err)
	}
	state, err := repo.GetMCPSelectionState(ctx, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if state.AcceptedSetup != nil || state.DesiredRevision != 2 || state.AppliedRevision != 1 {
		t.Fatalf("legacy migration = %#v", state)
	}
	if err := repo.SaveAcceptedMCPSetup(ctx, "empty", 0, "initial-empty", &mcpconfig.MCPSetupSnapshot{Version: 1, Servers: []mcpconfig.MCPSetupServer{}}); err != nil {
		t.Fatal(err)
	}
	reopened, err := newSQLiteRepository(repo.db, repo.ro, repo.log, false)
	if err != nil {
		t.Fatal(err)
	}
	state, err = reopened.GetMCPSelectionState(ctx, "empty")
	if err != nil {
		t.Fatal(err)
	}
	if state.AcceptedSetup == nil || state.AcceptedSetup.Servers == nil || len(state.AcceptedSetup.Servers) != 0 {
		t.Fatalf("known empty lost: %#v", state)
	}
}

func TestMCPAcceptedSetupSurvivesDatabaseReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.db")
	open := func() (*sqlx.DB, *sqliteRepository) {
		db, err := sqlx.Open("sqlite3", path)
		if err != nil {
			t.Fatal(err)
		}
		repo, err := newSQLiteRepository(db, db, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		return db, repo
	}
	db, repo := open()
	ctx := context.Background()
	setup := &mcpconfig.MCPSetupSnapshot{Version: 1, Servers: []mcpconfig.MCPSetupServer{{DefinitionID: "deleted", Revision: 3, Enabled: true, DisplayName: "Accepted name", RuntimeName: "accepted"}}}
	if err := repo.SaveAcceptedMCPSetup(ctx, "session", 7, "attempt", setup); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, repo = open()
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	state, err := repo.GetMCPSelectionState(ctx, "session")
	if err != nil {
		t.Fatal(err)
	}
	if state.AcceptedSetup == nil || state.AppliedRevision != 7 || state.AcceptedSetup.Servers[0].DisplayName != "Accepted name" {
		t.Fatalf("reopened = %#v", state)
	}
	if _, err := repo.GetMCPSelectionState(ctx, "sibling"); !errors.Is(err, mcpconfig.ErrMCPSelectionStateNotFound) {
		t.Fatalf("sibling = %v", err)
	}
}
