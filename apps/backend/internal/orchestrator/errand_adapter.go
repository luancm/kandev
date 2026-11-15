package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/errands"
)

// ErrandAdapter satisfies the errands service's launcher, teardown, and
// MCP selection seams on top of the orchestrator.
type ErrandAdapter struct {
	svc *Service
}

var (
	_ errands.Launcher       = (*ErrandAdapter)(nil)
	_ errands.ErrandTeardown = (*ErrandAdapter)(nil)
	_ errands.MCPSelections  = (*ErrandAdapter)(nil)
)

// NewErrandAdapter wraps the orchestrator for the errands service.
func NewErrandAdapter(svc *Service) *ErrandAdapter {
	return &ErrandAdapter{svc: svc}
}

// LaunchErrand shares guarded classification and admission with direct callers.
func (a *ErrandAdapter) LaunchErrand(ctx context.Context, request errands.LaunchRequest) (errands.LaunchResult, error) {
	result, _, err := a.svc.launchErrand(ctx, request)
	return result, err
}

// SessionSnapshot returns the session-level MCP additions and exclusions for a
// session. Without a selection reader there is nothing to inherit.
func (a *ErrandAdapter) SessionSnapshot(ctx context.Context, taskID, sessionID string) (errands.MCPSnapshot, error) {
	if a.svc.mcpSelectionReader == nil {
		return errands.MCPSnapshot{}, nil
	}
	task, err := a.svc.repo.GetTask(ctx, taskID)
	if err != nil {
		return errands.MCPSnapshot{}, fmt.Errorf("load task for parent MCP selections: %w", err)
	}
	snapshot, err := a.svc.mcpSelectionReader.ReadSnapshot(ctx, mcpconfig.SelectionOrigin{
		Scope: mcpconfig.SelectionScopeTaskSession, WorkspaceID: task.WorkspaceID, OwnerID: sessionID,
	})
	return errands.MCPSnapshot{DefinitionIDs: snapshot.DefinitionIDs, ExcludedDefinitionIDs: snapshot.ExcludedDefinitionIDs}, err
}
