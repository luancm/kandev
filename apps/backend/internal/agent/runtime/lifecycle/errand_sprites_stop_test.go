package lifecycle

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentruntime"
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
		if r.Method == http.MethodGet {
			switch r.URL.Path {
			case "/identity":
				_ = json.NewEncoder(w).Encode(agentctl.IdentityInfo{Capabilities: []string{agentctltypes.ErrandMCPOwnerCapability}})
			case "/api/v1/instances/execution":
				_ = json.NewEncoder(w).Encode(agentctl.InstanceInfo{
					ID: "execution", Port: 41001, Status: "running", WorkspacePath: spritesWorkspacePath,
					SessionID: "errand:child", ErrandMCPOwner: &owner, ProviderSessionID: "saved-native",
				})
			default:
				http.NotFound(w, r)
			}
			return
		}
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
	t.Run("saved secret", func(t *testing.T) {
		require.NoError(t, exact.stopErrandInstance(t.Context(), row, owner))
		require.NotEmpty(t, calls)
		require.Equal(t, owner, <-calls)
	})
	t.Run("live literal credential and proxy", func(t *testing.T) {
		delete(row.Metadata, "env_secret_id_SPRITES_API_TOKEN")
		t.Cleanup(func() { row.Metadata["env_secret_id_SPRITES_API_TOKEN"] = "provider-secret" })
		exec.tokens[row.AgentExecutionID] = "sprite-token"
		exec.tokens["sibling-execution"] = "sprite-token"
		closed := false
		exec.proxies[row.AgentExecutionID] = &SpritesProxySession{cancel: func() { closed = true }}
		sibling := &SpritesProxySession{}
		exec.proxies["sibling-execution"] = sibling
		require.NoError(t, exact.stopErrandInstance(t.Context(), row, owner))
		require.NotEmpty(t, calls)
		require.Equal(t, owner, <-calls)
		require.True(t, closed)
		require.NotContains(t, exec.proxies, row.AgentExecutionID)
		require.NotContains(t, exec.tokens, row.AgentExecutionID)
		require.Same(t, sibling, exec.proxies["sibling-execution"])
		require.Contains(t, exec.tokens, "sibling-execution")
	})

	t.Run("cold recovery and exact stop use the saved literal profile", func(t *testing.T) {
		manager, _ := task08ManagerWithBackend(t, exec)
		manager.SetExecutorProfileReader(&fakeExecutorProfileReader{profiles: map[string]*models.ExecutorProfile{
			"sprite-profile": {
				ID: "sprite-profile", ExecutorID: "sprite-executor",
				EnvVars: []models.ProfileEnvVar{{Key: spritesAPITokenEnvKey, Value: "sprite-token"}},
			},
		}})
		owned := ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: owner.TaskID, ErrandID: owner.ErrandID,
			Generation: owner.Generation, WorkspaceID: "workspace", AgentProfileID: "agent-profile"}
		saved := &models.ExecutorRunning{
			AgentExecutionID: row.AgentExecutionID, SessionID: row.SessionID, Runtime: agentruntime.RuntimeSprites,
			ExecutorID: "sprite-executor", ExecutionProfileID: owned.AgentProfileID, WorktreePath: spritesWorkspacePath,
			Metadata: map[string]interface{}{
				MetadataKeySpriteName: "shared", MetadataKeySpriteCreatedAt: "2026-02-03T01:00:00Z",
				MetadataKeyExecutorProfileID: "sprite-profile", runExecutionOwnerMetadataKey: owned,
			},
		}
		stale := *saved
		stale.Metadata = cloneSpriteRecoveryMetadata(saved.Metadata)
		stale.Metadata[MetadataKeySpriteCreatedAt] = "2026-02-03T00:00:00Z"
		instances, outcomes, err := exec.RecoverInstancesDetailed(t.Context(), []*models.ExecutorRunning{&stale})
		require.NoError(t, err)
		require.Empty(t, instances)
		require.Equal(t, RecoveryOutcomeUnknown, outcomes[saved.SessionID])
		instances, outcomes, err = exec.RecoverInstancesDetailed(t.Context(), []*models.ExecutorRunning{saved})
		require.NoError(t, err)
		require.Empty(t, outcomes)
		require.Len(t, instances, 1)
		require.Equal(t, owned, instances[0].recoveredErrandOwner)
		require.Empty(t, instances[0].TaskID)
		require.Equal(t, "saved-native", instances[0].ProviderSessionID)
		instances[0].DiscardRecovery()
		instances[0].Client.Close()
		exec.mu.Lock()
		delete(exec.tokens, saved.AgentExecutionID)
		exec.mu.Unlock()
		foreignProfile := *saved
		foreignProfile.ExecutorID = "foreign-executor"
		require.ErrorIs(t, exact.stopErrandInstance(t.Context(), &foreignProfile, owner), models.ErrWorkspaceReuseUnsafe)
		require.Empty(t, calls)
		require.NoError(t, exact.stopErrandInstance(t.Context(), saved, owner))
		require.NotEmpty(t, calls)
		require.Equal(t, owner, <-calls)

		manager.SetExecutorProfileReader(&fakeExecutorProfileReader{profiles: map[string]*models.ExecutorProfile{
			"sprite-profile": {
				ID: "sprite-profile", ExecutorID: "sprite-executor",
				EnvVars: []models.ProfileEnvVar{
					{Key: spritesAPITokenEnvKey, Value: "sprite-token"},
					{Key: spritesAPITokenEnvKey, Value: "different-token"},
				},
			},
		}})
		require.Error(t, exact.stopErrandInstance(t.Context(), saved, owner))
		require.Empty(t, calls)
	})

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
