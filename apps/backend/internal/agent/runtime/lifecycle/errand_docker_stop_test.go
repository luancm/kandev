package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestDockerErrandTeardownContactsOnlyOwnedAgentctlInstance(t *testing.T) {
	for _, runtimeName := range []string{"docker", "remote-docker"} {
		for _, status := range []int{http.StatusNoContent, http.StatusConflict, http.StatusServiceUnavailable} {
			t.Run(runtimeName+"/"+http.StatusText(status), func(t *testing.T) {
				testDockerErrandStop(t, runtimeName, status)
			})
		}
	}
}

func testDockerErrandStop(t *testing.T, runtimeName string, status int) {
	t.Helper()
	engine, dockerClient := newRetainedContainerEngine(t)
	expected := agentctltypes.ErrandMCPOwner{TaskID: "task", ErrandID: "child", Generation: 3}
	requests := make(chan agentctltypes.ErrandMCPOwner, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/instances/actual-agentctl-id/stop-errand" ||
			r.Header.Get("Authorization") != "Bearer auth-token" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var owner agentctltypes.ErrandMCPOwner
		if json.NewDecoder(r.Body).Decode(&owner) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- owner
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	_, port := parseHTTPURL(t, server.URL)
	endpoints := &remoteDockerRecoveryEndpoints{AgentCtlPort: uint16(port)}
	var backend ExecutorBackend
	switch runtimeName {
	case "docker":
		local := NewDockerExecutor(config.DockerConfig{}, "", newTestDockerLogger())
		local.docker = dockerClient
		local.initialized = true
		local.containerMgr = NewContainerManager(dockerClient, "", newTestDockerLogger())
		local.endpoints = endpoints
		backend = local
	case "remote-docker":
		remote := NewRemoteDockerExecutor(newTestDockerLogger())
		remote.connect = func(context.Context, *ExecutorCreateRequest) (*remoteDockerSession, error) {
			return &remoteDockerSession{dockerClient: dockerClient, endpoints: endpoints}, nil
		}
		backend = remote
	default:
		t.Fatalf("unexpected runtime %s", runtimeName)
	}
	row := &models.ExecutorRunning{
		AgentExecutionID: "execution", SessionID: "errand:child", Runtime: backend.Name(),
		ContainerID: "container-retained", TransientAuthToken: "auth-token",
		Metadata: map[string]interface{}{MetadataKeyRemoteDockerAgentctlInstanceID: "actual-agentctl-id"},
	}
	exact, ok := backend.(errandInstanceStopper)
	require.True(t, ok, "Docker teardown must support exact owned stop")
	err := exact.stopErrandInstance(t.Context(), row, expected)
	if status == http.StatusNoContent {
		require.NoError(t, err)
	} else {
		require.Error(t, err)
	}
	select {
	case owner := <-requests:
		require.Equal(t, expected, owner)
	default:
		t.Fatal("agentctl did not receive an exact stop request")
	}
	for _, request := range engine.snapshot() {
		require.True(t, strings.HasPrefix(request, "GET ") || strings.HasPrefix(request, "HEAD "), request)
	}
}
