package lifecycle

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/docker"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestDockerErrandTeardownConfirmsContainerAbsence(t *testing.T) {
	for _, tc := range []struct {
		name, foundID, state string
		status               int
		absent               bool
	}{
		{name: "unverified daemon reports missing", status: http.StatusNotFound},
		{name: "exited", status: http.StatusOK, foundID: "container-recorded", state: containerStateExited, absent: true},
		{name: "unavailable", status: http.StatusServiceUnavailable},
		{name: "restarting", status: http.StatusOK, foundID: "container-recorded", state: "restarting"},
		{name: "mismatched container", status: http.StatusOK, foundID: "container-other", state: containerStateExited},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newDockerErrandAbsenceProbe(t, tc.status, tc.foundID, tc.state)
			executor := NewDockerExecutor(config.DockerConfig{}, "", newTestDockerLogger())
			owner := agentctltypes.ErrandMCPOwner{TaskID: "task", ErrandID: "child", Generation: 3}
			row := &models.ExecutorRunning{
				AgentExecutionID: "execution", SessionID: models.ErrandExecutionIdentity(owner.ErrandID),
				ContainerID: "container-recorded", TransientAuthToken: "auth-token",
				Metadata: map[string]interface{}{
					MetadataKeyContainerID: "container-recorded", MetadataKeyRemoteDockerAgentctlInstanceID: "actual-instance",
				},
			}
			err := executor.stopDockerErrandInstance(t.Context(), client, row, owner)
			if tc.absent {
				require.NoError(t, err, "the exact recorded container cannot host a running Errand")
			} else {
				require.Error(t, err)
			}
		})
	}
}

func newDockerErrandAbsenceProbe(t *testing.T, status int, id, state string) *docker.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodHead && strings.HasSuffix(request.URL.Path, "/_ping"):
			w.Header().Set("API-Version", "1.41")
			w.WriteHeader(http.StatusOK)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/version"):
			_ = json.NewEncoder(w).Encode(map[string]string{"ApiVersion": "1.41", "Arch": "amd64"})
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/containers/container-recorded/json"):
			w.WriteHeader(status)
			if status == http.StatusOK {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"Id": id, "State": map[string]string{"Status": state}})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]string{"message": http.StatusText(status)})
			}
		default:
			t.Errorf("unexpected Docker request %s %s", request.Method, request.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	address := server.Listener.Addr().String()
	client, err := docker.NewRemoteClient(docker.RemoteTransport{
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", address)
		},
	}, newTestDockerLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

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
