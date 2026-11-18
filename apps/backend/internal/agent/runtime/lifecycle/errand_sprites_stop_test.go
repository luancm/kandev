package lifecycle

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	sprites "github.com/superfly/sprites-go"
)

func TestSpritesErrandTeardownUsesRecordedSpriteAndOwner(t *testing.T) {
	owner := agentctltypes.ErrandMCPOwner{TaskID: "task", ErrandID: "child", Generation: 2}
	calls := make(chan agentctltypes.ErrandMCPOwner, 4)
	var controlStatus atomic.Int32
	controlStatus.Store(http.StatusNoContent)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/instances/execution/stop-errand" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var got agentctltypes.ErrandMCPOwner
		if json.NewDecoder(r.Body).Decode(&got) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		calls <- got
		w.WriteHeader(int(controlStatus.Load()))
	}))
	t.Cleanup(target.Close)
	var destructiveCalls atomic.Int64
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			destructiveCalls.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer sprite-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/sprites/shared":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"shared","id":"sprite-shared","status":"running","created_at":"2026-02-03T01:00:00Z"}`))
		case "/v1/sprites/shared/proxy":
			proxyAgentctlWebSocket(w, r, upgrader, target.Listener.Addr().String(), spritesAgentctlPort, 41001)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(provider.Close)
	exec := newTestSpritesExecutor(&mockSecretStore{store: map[string]string{"provider-secret": "sprite-token"}})
	exec.agentctlPort = spritesAgentctlPort
	exec.setSpritesClientFactory(func(token string) *sprites.Client {
		return sprites.New(token, sprites.WithBaseURL(provider.URL), sprites.WithDisableControl())
	})
	row := &models.ExecutorRunning{AgentExecutionID: "execution", SessionID: "errand:child", Metadata: map[string]interface{}{
		MetadataKeySpriteName: "shared", MetadataKeySpriteCreatedAt: "2026-02-03T01:00:00Z",
		"env_secret_id_SPRITES_API_TOKEN": "provider-secret",
	}}
	exact, ok := any(exec).(errandInstanceStopper)
	require.True(t, ok, "Sprites teardown must support the exact owned instance")
	require.NoError(t, exact.stopErrandInstance(t.Context(), row, owner))
	select {
	case got := <-calls:
		require.Equal(t, owner, got)
	default:
		t.Fatal("the expected instance stop request was not sent")
	}

	controlStatus.Store(http.StatusNotFound)
	require.Error(t, exact.stopErrandInstance(t.Context(), row, owner))
	select {
	case got := <-calls:
		require.Equal(t, owner, got)
	default:
		t.Fatal("the expected instance stop request was not sent")
	}
	row.Metadata[MetadataKeySpriteCreatedAt] = time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	require.Error(t, exact.stopErrandInstance(t.Context(), row, owner))
	require.Empty(t, calls)
	require.Zero(t, destructiveCalls.Load())
}
