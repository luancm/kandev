package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestWorkspaceConflictMappingKeepsEverySpawnOutput pins the typed workspace
// failures that spawn_session_kandev and start_errand_kandev share: code,
// message, and the reason/recoverable/retry details.
func TestWorkspaceConflictMappingKeepsEverySpawnOutput(t *testing.T) {
	unsafeMessage := "the task workspace cannot be safely reused; retry after restoring its existing workspace"
	cases := []struct {
		name    string
		err     error
		message string
		reason  string
		retry   string
	}{
		{"unsupported", models.ErrWorkspaceReuseUnsupported, models.WorkspaceReuseUnsupportedMessage,
			"workspace_reuse_unsupported", models.WorkspaceReuseUnsupportedRetry},
		{"preparing", models.ErrWorkspacePreparing,
			"the task workspace is still being prepared; retry after its initial launch completes",
			"workspace_preparing", "retry after the initial workspace launch completes"},
		{"reset required", models.ErrTaskEnvironmentResetRequired,
			"the task workspace failed and keeps workspace resources; reset the task environment, then retry",
			"workspace_reuse_unsafe", "reset the task environment, then retry"},
		{"unsafe", models.ErrWorkspaceReuseUnsafe, unsafeMessage, "workspace_reuse_unsafe", "restore the task workspace, then retry"},
		{"worktree unavailable", worktree.ErrReuseWorktreeUnavailable, unsafeMessage,
			"workspace_reuse_unsafe", "restore the task workspace, then retry"},
		{"wrapped", fmt.Errorf("launch: %w", models.ErrWorkspacePreparing),
			"the task workspace is still being prepared; retry after its initial launch completes",
			"workspace_preparing", "retry after the initial workspace launch completes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, handled := workspaceConflictError("req-1", "mcp.spawn_session", tc.err)
			require.True(t, handled)
			var payload ws.ErrorPayload
			require.NoError(t, json.Unmarshal(msg.Payload, &payload))
			require.Equal(t, ws.ErrorCodeConflict, payload.Code)
			require.Equal(t, tc.message, payload.Message)
			require.Equal(t, tc.reason, payload.Details["reason"])
			require.Equal(t, true, payload.Details["recoverable"])
			require.Equal(t, tc.retry, payload.Details["retry"])
		})
	}
}

func TestWorkspaceConflictMappingIgnoresOtherErrors(t *testing.T) {
	msg, handled := workspaceConflictError("req-1", "mcp.spawn_session", errors.New("boom"))
	require.False(t, handled)
	require.Nil(t, msg)
	_, handled = workspaceConflictError("req-1", "mcp.spawn_session", nil)
	require.False(t, handled)
}
