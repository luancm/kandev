package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

// ErrandAdapter satisfies the errands service's launcher, session stopper, and
// MCP selection seams on top of the orchestrator.
type ErrandAdapter struct {
	svc *Service
}

var (
	_ errands.Launcher       = (*ErrandAdapter)(nil)
	_ errands.SessionStopper = (*ErrandAdapter)(nil)
	_ errands.MCPSelections  = (*ErrandAdapter)(nil)
)

// NewErrandAdapter wraps the orchestrator for the errands service.
func NewErrandAdapter(svc *Service) *ErrandAdapter {
	return &ErrandAdapter{svc: svc}
}

// LaunchErrand starts the errand session and reports a ceiling refusal as
// errands.ErrCeilingRefused.
func (a *ErrandAdapter) LaunchErrand(ctx context.Context, request errands.LaunchRequest) error {
	_, err := a.svc.LaunchErrandSession(ctx, request)
	if errors.Is(err, ErrSessionCeilingRefused) {
		return fmt.Errorf("%w: %w", errands.ErrCeilingRefused, err)
	}
	return err
}

// StopErrandSession stops an errand's runtime. A session that does not exist
// yet, such as a cancelled queued errand, or whose runtime is already gone has
// nothing to stop.
func (a *ErrandAdapter) StopErrandSession(ctx context.Context, sessionID, reason string) error {
	if _, err := a.svc.repo.GetTaskSession(ctx, sessionID); err != nil {
		if errors.Is(err, models.ErrTaskSessionNotFound) {
			return nil
		}
		return err
	}
	err := a.svc.StopSession(ctx, sessionID, reason, false)
	if errors.Is(err, agentruntime.ErrNoExecutionForSession) {
		return nil
	}
	return err
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
