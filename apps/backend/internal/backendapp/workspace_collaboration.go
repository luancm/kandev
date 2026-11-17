package backendapp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type workspaceCollaborationLookup interface {
	GetTask(context.Context, string) (*models.Task, error)
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
	GetWorkspace(context.Context, string) (*models.Workspace, error)
	GetWorkspaceCollaborationGrant(context.Context, string, string, string) (*models.WorkspaceCollaborationGrant, error)
}

// workspaceCollaborationBoundary interprets only server-installed principal and
// execution contexts. Human/native authorization remains in the task service.
type workspaceCollaborationBoundary struct {
	enabled bool
	store   workspaceCollaborationLookup
}

func (b *workspaceCollaborationBoundary) Enabled() bool { return b.enabled }

func (b *workspaceCollaborationBoundary) validatePluginCall(ctx context.Context, agentTool bool) error {
	if !b.enabled || (!agentTool && !b.IsConversation(ctx)) {
		return nil
	}
	_, err := b.caller(ctx)
	return err
}

func (b *workspaceCollaborationBoundary) IsConversation(ctx context.Context) bool {
	if _, ok := mcpscope.PrincipalFromContext(ctx); ok {
		return true
	}
	execution, ok := streams.MCPExecutionContextFromContext(ctx)
	return ok && execution.TaskID != ""
}

func (b *workspaceCollaborationBoundary) CallerTaskID(ctx context.Context) string {
	principal, _ := mcpscope.PrincipalFromContext(ctx)
	return principal.CallerTaskID
}

func (b *workspaceCollaborationBoundary) Authorize(ctx context.Context, targetID string, action models.WorkspaceCollaborationAction) error {
	if !b.enabled || !b.IsConversation(ctx) {
		return nil
	}
	if targetID == "" {
		return taskservice.ErrWorkspaceCollaborationUnavailable
	}
	task, err := b.caller(ctx)
	if err != nil {
		return err
	}
	if task.WorkspaceID == targetID {
		return nil
	}
	grant, err := b.store.GetWorkspaceCollaborationGrant(ctx, task.ID, task.WorkspaceID, targetID)
	if err != nil {
		return fmt.Errorf("%w: grants", taskservice.ErrWorkspaceCollaborationUnavailable)
	}
	if grant == nil || !grant.Allows(action) {
		return taskservice.ErrForbidden
	}
	return nil
}

func (b *workspaceCollaborationBoundary) caller(ctx context.Context) (*models.Task, error) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || principal.WorkspaceID == "" || b.store == nil {
		return nil, taskservice.ErrWorkspaceCollaborationUnavailable
	}
	task, err := b.store.GetTask(ctx, principal.CallerTaskID)
	if err != nil {
		return nil, fmt.Errorf("%w: caller task", taskservice.ErrWorkspaceCollaborationUnavailable)
	}
	if task == nil || task.ID != principal.CallerTaskID || task.WorkspaceID != principal.WorkspaceID {
		return nil, taskservice.ErrWorkspaceCollaborationUnavailable
	}
	if principal.Surface == mcpprofile.SurfaceErrand {
		if err := b.validateErrandCaller(ctx, principal); err != nil {
			return nil, err
		}
	} else {
		session, err := b.store.GetTaskSession(ctx, principal.CallerSessionID)
		if err != nil || session == nil || session.TaskID != task.ID || session.ID != principal.CallerSessionID {
			return nil, taskservice.ErrWorkspaceCollaborationUnavailable
		}
	}
	workspace, err := b.store.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil || workspace == nil {
		return nil, fmt.Errorf("%w: caller workspace", taskservice.ErrWorkspaceCollaborationUnavailable)
	}
	return task, nil
}

func (b *workspaceCollaborationBoundary) validateErrandCaller(ctx context.Context, principal mcpscope.Principal) error {
	execution, ok := streams.MCPExecutionContextFromContext(ctx)
	if !ok || execution.ErrandID != principal.ErrandID || execution.TaskID != principal.CallerTaskID || execution.WorkspaceID != principal.WorkspaceID {
		return taskservice.ErrWorkspaceCollaborationUnavailable
	}
	lookup, ok := b.store.(interface {
		GetErrand(context.Context, string) (*models.Errand, error)
		ErrandHarnessBindingStale(context.Context, string, *models.ErrandHarnessBinding) (bool, error)
	})
	if !ok {
		return taskservice.ErrWorkspaceCollaborationUnavailable
	}
	row, err := lookup.GetErrand(ctx, execution.ErrandID)
	if err != nil || row == nil || row.ID != execution.ErrandID || row.TaskID != execution.TaskID {
		return taskservice.ErrWorkspaceCollaborationUnavailable
	}
	binding := &models.ErrandHarnessBinding{ErrandID: execution.ErrandID, Generation: execution.ErrandGeneration, ExecutionID: execution.ExecutionID, IncarnationID: execution.ParentIncarnationID, HarnessGeneration: execution.HarnessGeneration, StreamID: execution.StreamID}
	stale, err := lookup.ErrandHarnessBindingStale(ctx, execution.TaskID, binding)
	if err != nil || stale {
		return taskservice.ErrWorkspaceCollaborationUnavailable
	}
	return nil
}
