package scope

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

// Principal is the server-derived identity of an in-session MCP caller.
// Request payloads must never be used to construct or replace these fields.
// Automation handlers use the principal as their workspace and self-target
// boundary in addition to the normal owner identity attached to the context.
type Principal struct {
	AutomationID    string
	WorkspaceID     string
	CallerTaskID    string
	CallerSessionID string
	Surface         mcpprofile.Surface
	CoordinatorID   string
	ErrandID        string
}

func (p Principal) IsAutomation() bool {
	return p.AutomationID != "" && p.Surface == mcpprofile.SurfaceAutomation
}

// IsCoordinator reports whether this principal is a coordinator's own
// attended conversation session (docs/specs/coordinator/system-design/
// copilot.md#principal-and-mode).
func (p Principal) IsCoordinator() bool {
	return p.CoordinatorID != "" && p.Surface == mcpprofile.SurfaceCoordinator
}

type principalContextKey struct{}

// WithPrincipal attaches a trusted principal to a dispatch context.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// PrincipalFromContext returns the server-derived caller principal.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	if principal.Surface == mcpprofile.SurfaceErrand {
		return principal, ok && principal.CallerTaskID != "" && principal.CallerSessionID == "" && principal.ErrandID != "" && principal.WorkspaceID != "" && principal.AutomationID == "" && principal.CoordinatorID == ""
	}
	return principal, ok && principal.CallerTaskID != "" && principal.CallerSessionID != "" && principal.ErrandID == ""
}

// ScopePrincipal derives the MCP principal from the execution's own task and
// session. It intentionally does not read any identity or surface fields from
// an agent payload. Non-automation tasks still receive their normal surface so
// downstream code has one consistent trusted caller shape.
func (r *Resolver) ScopePrincipal(ctx context.Context, execution streams.MCPExecutionContext) (context.Context, error) {
	taskID, sessionID := execution.TaskID, execution.SessionID
	if hasErrandIdentity(execution) {
		return r.scopeErrandPrincipal(ctx, execution)
	}
	if r == nil || taskID == "" || sessionID == "" || r.tasks == nil {
		return nil, fmt.Errorf("resolve MCP principal: task and session are required")
	}
	task, err := r.resolvePrincipalTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	session, err := r.principalSession(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	if session != nil {
		ctx = models.WithExecutorActor(ctx, session.ExecutorActorUserID)
	}

	workspaceID, err := r.resolvePrincipalWorkspace(ctx, task)
	if err != nil {
		return nil, err
	}

	automationID, coordinatorID, surface, err := r.principalSurface(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("resolve MCP principal task %s: %w", taskID, err)
	}
	if surface == mcpprofile.SurfaceKanbanTask || surface == mcpprofile.SurfaceOfficeTask {
		surface, err = configurationPrincipalSurface(task, session, surface)
		if err != nil {
			return nil, fmt.Errorf("resolve MCP principal task %s: %w", taskID, err)
		}
	}
	return WithPrincipal(ctx, Principal{
		AutomationID:    automationID,
		WorkspaceID:     workspaceID,
		CallerTaskID:    taskID,
		CallerSessionID: sessionID,
		Surface:         surface,
		CoordinatorID:   coordinatorID,
	}), nil
}

func hasErrandIdentity(execution streams.MCPExecutionContext) bool {
	return execution.ErrandID != "" || execution.ErrandGeneration != 0 || execution.WorkspaceID != "" || execution.StreamID != ""
}

func (r *Resolver) resolvePrincipalTask(ctx context.Context, taskID string) (*models.Task, error) {
	task, err := r.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("resolve MCP principal task %s: %w", taskID, err)
	}
	if task == nil {
		return nil, fmt.Errorf("resolve MCP principal task %s: task not found", taskID)
	}
	return task, nil
}

func (r *Resolver) principalSession(ctx context.Context, taskID, sessionID string) (*models.TaskSession, error) {
	lookup, ok := r.tasks.(interface {
		GetTaskSession(context.Context, string) (*models.TaskSession, error)
	})
	if !ok {
		return nil, nil
	}
	session, err := lookup.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("resolve MCP principal session %s: %w", sessionID, err)
	}
	if session == nil || session.ID != sessionID || session.TaskID != taskID {
		return nil, fmt.Errorf("resolve MCP principal: session %s does not belong to task %s", sessionID, taskID)
	}
	return session, nil
}

func configurationPrincipalSurface(task *models.Task, session *models.TaskSession, surface mcpprofile.Surface) (mcpprofile.Surface, error) {
	if session == nil {
		return surface, nil
	}
	configMode, _ := session.Metadata["config_mode"].(bool)
	if !configMode {
		return surface, nil
	}
	_, managed, err := models.ManagedToolPolicyFromTask(task)
	if err != nil {
		return surface, err
	}
	if managed {
		return mcpprofile.SurfaceManagedConversation, nil
	}
	return mcpprofile.SurfaceConfiguration, nil
}

func (r *Resolver) resolvePrincipalWorkspace(ctx context.Context, task *models.Task) (string, error) {
	if task.WorkspaceID == "" {
		return "", fmt.Errorf("resolve MCP principal task %s: workspace is required", task.ID)
	}
	workspace, err := r.tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("resolve MCP principal workspace %s: %w", task.WorkspaceID, err)
	}
	if workspace == nil {
		return "", fmt.Errorf("resolve MCP principal workspace %s: workspace not found", task.WorkspaceID)
	}
	return task.WorkspaceID, nil
}

// principalSurface derives the MCP surface (and, for automation/coordinator
// tasks, the owning automation/coordinator id) from task. The coordinator
// branch runs first: a coordinator-origin task must resolve to its
// coordinator or be refused, never fall through to the Kanban/office default
// (docs/specs/coordinator/system-design/copilot.md#principal-and-mode).
func (r *Resolver) principalSurface(ctx context.Context, task *models.Task) (automationID, coordinatorID string, surface mcpprofile.Surface, err error) {
	if task.Origin == models.TaskOriginCoordinator {
		if r.coordinators == nil {
			return "", "", mcpprofile.SurfaceCoordinator, fmt.Errorf("coordinator lookup is not configured")
		}
		id, ok, err := r.coordinators.CoordinatorForConversationTask(ctx, task.ID)
		if err != nil {
			return "", "", mcpprofile.SurfaceCoordinator, fmt.Errorf("resolve coordinator for task %s: %w", task.ID, err)
		}
		if !ok {
			return "", "", mcpprofile.SurfaceCoordinator, fmt.Errorf("task %s is not a live coordinator conversation task", task.ID)
		}
		return "", id, mcpprofile.SurfaceCoordinator, nil
	}
	if task.Origin != models.TaskOriginAutomationRun {
		if task.IsFromOffice {
			return "", "", mcpprofile.SurfaceOfficeTask, nil
		}
		return "", "", mcpprofile.SurfaceKanbanTask, nil
	}
	automationID = models.StringFromAny(task.Metadata["automation_id"])
	if automationID == "" {
		return "", "", mcpprofile.SurfaceAutomation, fmt.Errorf("automation ID is missing")
	}
	return automationID, "", mcpprofile.SurfaceAutomation, nil
}

func (r *Resolver) scopeErrandPrincipal(ctx context.Context, execution streams.MCPExecutionContext) (context.Context, error) {
	if r == nil || r.tasks == nil {
		return nil, fmt.Errorf("resolve MCP errand: lookup unavailable")
	}
	if _, ok := streams.MCPExecutionContextFromContext(streams.WithMCPExecutionContext(ctx, execution)); !ok {
		return nil, fmt.Errorf("resolve MCP errand: incomplete identity")
	}
	lookup, ok := r.tasks.(interface {
		GetErrand(context.Context, string) (*models.Errand, error)
		ErrandHarnessBindingStale(context.Context, string, *models.ErrandHarnessBinding) (bool, error)
	})
	if !ok {
		return nil, fmt.Errorf("resolve MCP errand: lookup unavailable")
	}
	task, err := r.resolvePrincipalTask(ctx, execution.TaskID)
	if err != nil {
		return nil, err
	}
	workspace, err := r.resolvePrincipalWorkspace(ctx, task)
	if err != nil {
		return nil, err
	}
	if workspace != execution.WorkspaceID {
		return nil, fmt.Errorf("resolve MCP errand: workspace mismatch")
	}
	row, err := lookup.GetErrand(ctx, execution.ErrandID)
	if err != nil {
		return nil, err
	}
	if row == nil || row.ID != execution.ErrandID || row.TaskID != execution.TaskID {
		return nil, fmt.Errorf("resolve MCP errand: task mismatch")
	}
	binding := &models.ErrandHarnessBinding{ErrandID: execution.ErrandID, Generation: execution.ErrandGeneration, ExecutionID: execution.ExecutionID, IncarnationID: execution.ParentIncarnationID, HarnessGeneration: execution.HarnessGeneration, StreamID: execution.StreamID}
	stale, err := lookup.ErrandHarnessBindingStale(ctx, execution.TaskID, binding)
	if err != nil {
		return nil, err
	}
	if stale {
		return nil, fmt.Errorf("resolve MCP errand: stale binding")
	}
	ctx, err = r.errandActorContext(ctx, execution.TaskID, row.ParentSessionID)
	if err != nil {
		return nil, err
	}
	return WithPrincipal(ctx, Principal{WorkspaceID: workspace, CallerTaskID: execution.TaskID, ErrandID: execution.ErrandID, Surface: mcpprofile.SurfaceErrand}), nil
}

func (r *Resolver) errandActorContext(ctx context.Context, taskID, parentSessionID string) (context.Context, error) {
	if parentSessionID == "" {
		return ctx, nil
	}
	session, err := r.principalSession(ctx, taskID, parentSessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("resolve MCP errand: parent lookup unavailable")
	}
	return models.WithExecutorActor(ctx, session.ExecutorActorUserID), nil
}
