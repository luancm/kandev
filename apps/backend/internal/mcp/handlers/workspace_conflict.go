package handlers

import (
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	workspacePreparingMessage = "the task workspace is still being prepared; retry after its initial launch completes"
	workspacePreparingRetry   = "retry after the initial workspace launch completes"
	workspacePreparingReason  = "workspace_preparing"
	workspaceUnsafeReason     = "workspace_reuse_unsafe"
	workspaceRestoreRetry     = "restore the task workspace, then retry"
)

// workspaceConflictError maps the typed failures of attaching a new session to
// a task's existing workspace to a CONFLICT response with the reason,
// recoverable, and retry details. It reports false for any other error. Both
// spawn_session_kandev and start_errand_kandev use it, so the two surfaces
// refuse a preparing, unsafe, or unsupported workspace identically.
func workspaceConflictError(msgID, action string, err error) (*ws.Message, bool) {
	var message, reason, retry string
	switch {
	case errors.Is(err, models.ErrWorkspaceReuseUnsupported):
		message, reason, retry = models.WorkspaceReuseUnsupportedMessage, "workspace_reuse_unsupported",
			models.WorkspaceReuseUnsupportedRetry
	case errors.Is(err, models.ErrWorkspacePreparing):
		message, reason, retry = workspacePreparingMessage,
			workspacePreparingReason, workspacePreparingRetry
	case errors.Is(err, models.ErrTaskEnvironmentResetRequired):
		message, reason, retry = "the task workspace failed and keeps workspace resources; reset the task environment, then retry",
			workspaceUnsafeReason, "reset the task environment, then retry"
	case errors.Is(err, models.ErrWorkspaceReuseUnsafe), errors.Is(err, worktree.ErrReuseWorktreeUnavailable):
		message, reason, retry = "the task workspace cannot be safely reused; retry after restoring its existing workspace",
			workspaceUnsafeReason, workspaceRestoreRetry
	default:
		return nil, false
	}
	msg, marshalErr := ws.NewError(msgID, action, ws.ErrorCodeConflict, message, map[string]interface{}{
		mcpReasonKey:  reason,
		"recoverable": true,
		"retry":       retry,
	})
	return msg, marshalErr == nil
}
