package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

// ErrandAdapter satisfies the errands service's launcher, teardown, and
// MCP selection seams on top of the orchestrator.
type ErrandAdapter struct {
	svc                *Service
	runtime            runtimeapi.Runtime
	workspace          func(context.Context, string, string) (*runtimeapi.WorkspaceInfo, error)
	ownedStatusChanged func(context.Context, string, string)
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

// SetOwnedLaunchRuntime binds the runtime and parent workspace reader at setup.
func (a *ErrandAdapter) SetOwnedLaunchRuntime(runtime runtimeapi.Runtime, workspace func(context.Context, string, string) (*runtimeapi.WorkspaceInfo, error)) {
	a.runtime = runtime
	a.workspace = workspace
}

// LaunchErrand admits only independent Errand-owned executions.
func (a *ErrandAdapter) LaunchErrand(ctx context.Context, request errands.LaunchRequest) (errands.LaunchResult, error) {
	blocked := errands.LaunchResult{Disposition: errands.LaunchBlocked}
	if !validErrandLaunchRequest(request) {
		return blocked, models.ErrInvalidErrand
	}
	if err := a.svc.authorizeTask(ctx, request.TaskID); err != nil {
		return blocked, err
	}
	if a.runtime == nil {
		return blocked, errors.New("errand runtime is unavailable")
	}
	return a.launchOwnedErrand(ctx, request)
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
