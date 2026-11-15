package api

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestAwaitAbortReplaysOnAuthenticatedAgentStream(t *testing.T) {
	ctx := t.Context()
	log := newTestLogger()
	cfg := &config.InstanceConfig{TaskID: "task", SessionID: "parent", InstanceID: "instance", WorkDir: t.TempDir(), DurableJournalPath: filepath.Join(t.TempDir(), "journal.bbolt"), DeliveryIncarnationID: "launched", DeliveryHarnessGeneration: 3, DeliveryStreamID: "delivery"}
	manager := process.NewManager(cfg, log)
	j, err := manager.DeliveryJournal()
	require.NoError(t, err)
	t.Cleanup(func() { _ = j.Close() })
	id, err := manager.CaptureAwaitInvocation(ctx, "pre-success-error")
	require.NoError(t, err)
	require.NoError(t, manager.RecordAwaitAbort(ctx, id))
	backend := mcpserver.NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	mcp := mcpserver.New(backend, "parent", "task", 0, log, "", false, mcpserver.ModeTask)
	server := httptest.NewServer(NewServer(cfg, manager, mcp, backend, log).router)
	t.Cleanup(server.Close)
	// Lose the first abort response; a new authenticated stream must retry the same intent.
	first := dialTestWS(t, server)
	require.NoError(t, first.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, data, err := first.ReadMessage()
	require.NoError(t, err)
	var request ws.Message
	require.NoError(t, json.Unmarshal(data, &request))
	require.Equal(t, ws.ActionMCPAwaitErrandsRelease, request.Action)
	require.NoError(t, first.Close())
	second := dialTestWS(t, server)
	t.Cleanup(func() { _ = second.Close() })
	require.NoError(t, second.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, data, err = second.ReadMessage()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &request))
	require.Equal(t, ws.ActionMCPAwaitErrandsRelease, request.Action)
	original, ok := mcporigin.AwaitInvocationFromMessage(&request)
	require.True(t, ok)
	require.Equal(t, id, original)
	require.True(t, mcporigin.MessageCarriesTrustedInternalCall(&request))
	response, err := ws.NewResponse(request.ID, request.Action, map[string]any{"released": true})
	require.NoError(t, err)
	require.NoError(t, second.WriteJSON(response))
	require.Eventually(t, func() bool { pending, err := j.PendingAwaitAborts(ctx); return err == nil && len(pending) == 0 }, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, second.Close())
}
