package orchestrator

import (
	"context"
	"fmt"
	"slices"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

func (s *Service) applyErrandMCPExclusions(ctx context.Context, workspaceID, sessionID string, captured []string) error {
	if len(captured) == 0 {
		return nil
	}
	writer, ok := s.mcpSelectionWriter.(interface {
		ReplaceConditional(context.Context, mcpconfig.SelectionOrigin, []string, mcpconfig.SelectionCondition) (mcpconfig.SelectionSnapshot, error)
	})
	if !ok || s.mcpSelectionReader == nil {
		return fmt.Errorf("conditional MCP selections are required for errand exclusions")
	}
	inherited, err := s.mcpSelectionReader.InheritedDefinitionIDs(ctx, workspaceID, sessionID)
	if err != nil {
		return fmt.Errorf("read inherited errand MCP definitions: %w", err)
	}
	excluded := make([]string, 0, len(captured))
	for _, id := range captured {
		if slices.Contains(inherited, id) {
			excluded = append(excluded, id)
		}
	}
	if len(excluded) == 0 {
		return nil
	}
	origin := mcpconfig.SelectionOrigin{Scope: mcpconfig.SelectionScopeTaskSession, WorkspaceID: workspaceID, OwnerID: sessionID}
	snapshot, err := s.mcpSelectionReader.ReadSnapshot(ctx, origin)
	if err != nil {
		return fmt.Errorf("read child MCP selection revision: %w", err)
	}
	revision := int64(0)
	if snapshot.State != nil {
		revision = snapshot.State.DesiredRevision
	}
	_, err = writer.ReplaceConditional(ctx, origin, snapshot.DefinitionIDs, mcpconfig.SelectionCondition{
		ExpectedRevision: &revision, ExcludedDefinitionIDs: &excluded,
	})
	if err != nil {
		return fmt.Errorf("apply errand MCP exclusions: %w", err)
	}
	return nil
}
