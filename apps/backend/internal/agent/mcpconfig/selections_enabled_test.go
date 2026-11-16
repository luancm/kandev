package mcpconfig

import (
	"context"
	"reflect"
	"testing"
)

func TestSelectionServiceEnabledDefinitionIDsDropsDisabledMissingAndForeign(t *testing.T) {
	catalogRepo := newCatalogRepositoryFake()
	catalog := NewCatalogService(catalogRepo)
	create := func(workspaceID, name string) *MCPServerDefinition {
		definition, err := catalog.Create(context.Background(), CreateDefinitionInput{
			WorkspaceID: workspaceID, RuntimeName: name, DisplayName: name,
			ExecutionMode: ExecutionModeExistingExecutable, Transport: ServerTypeStdio,
			Configuration: MCPServerConfiguration{Command: name},
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return definition
	}
	first := create("workspace-1", "first")
	disabled := create("workspace-1", "disabled")
	foreign := create("workspace-2", "foreign")
	last := create("workspace-1", "last")
	disabled.Enabled = false
	catalogRepo.definitions[disabled.ID] = disabled

	service := NewSelectionService(newSelectionRepositoryFake(), catalogRepo)
	got, err := service.EnabledDefinitionIDs(context.Background(), "workspace-1",
		[]string{last.ID, disabled.ID, "missing", foreign.ID, first.ID, last.ID})
	if err != nil {
		t.Fatalf("EnabledDefinitionIDs: %v", err)
	}
	if want := []string{last.ID, first.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("enabled IDs = %v, want %v in the caller's order without duplicates", got, want)
	}
}

func TestSelectionServiceEnabledDefinitionIDsAuthorizesTheWorkspace(t *testing.T) {
	service := NewSelectionService(newSelectionRepositoryFake(), newCatalogRepositoryFake())
	service.SetWorkspaceAuthorizer(func(context.Context, string) error { return context.Canceled })
	if _, err := service.EnabledDefinitionIDs(context.Background(), "workspace-1", []string{"definition-1"}); err == nil {
		t.Fatal("EnabledDefinitionIDs succeeded for a workspace the caller cannot access")
	}
}
