package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
)

func TestStopErrandInstanceChecksOwnerAndCredential(t *testing.T) {
	mgr, router := errandStopFixture(t)
	mgr.SetServerFactory(func(*config.InstanceConfig, *process.Manager, *logger.Logger) http.Handler {
		return http.NotFoundHandler()
	})
	owner := agentctltypes.ErrandMCPOwner{TaskID: "task", ErrandID: "child", Generation: 2}
	_, err := mgr.CreateInstance(t.Context(), &instance.CreateRequest{
		ID: "child-execution", SessionID: "errand:child", WorkspacePath: t.TempDir(), ErrandMCPOwner: &owner,
	})
	require.NoError(t, err)
	_, err = mgr.CreateInstance(t.Context(), &instance.CreateRequest{
		ID: "parent-execution", SessionID: "parent", WorkspacePath: t.TempDir(),
	})
	require.NoError(t, err)

	require.Equal(t, http.StatusUnauthorized, requestErrandStop(t, router, "child-execution", owner, ""))
	foreign := owner
	foreign.Generation++
	require.Equal(t, http.StatusConflict, requestErrandStop(t, router, "child-execution", foreign, "current-token"))
	require.Equal(t, http.StatusConflict, requestErrandStop(t, router, "parent-execution", owner, "current-token"))
	_, childPresent := mgr.GetInstance("child-execution")
	require.True(t, childPresent)

	require.Equal(t, http.StatusNoContent, requestErrandStop(t, router, "child-execution", owner, "current-token"))
	_, childPresent = mgr.GetInstance("child-execution")
	require.False(t, childPresent)
	require.Equal(t, http.StatusNoContent, requestErrandStop(t, router, "child-execution", owner, "current-token"))
	_, parentPresent := mgr.GetInstance("parent-execution")
	require.True(t, parentPresent)
}

func TestStopErrandInstanceDoesNotConfirmProvisionalAbsence(t *testing.T) {
	mgr, router := errandStopFixture(t)
	reached, publish := make(chan struct{}), make(chan struct{})
	publication := publish
	defer func() {
		if publish != nil {
			close(publish)
		}
	}()
	mgr.SetServerFactory(func(*config.InstanceConfig, *process.Manager, *logger.Logger) http.Handler {
		close(reached)
		<-publication
		return http.NotFoundHandler()
	})
	owner := agentctltypes.ErrandMCPOwner{TaskID: "task", ErrandID: "child", Generation: 1}
	workspace := t.TempDir()
	created := make(chan error, 1)
	go func() {
		_, err := mgr.CreateInstance(t.Context(), &instance.CreateRequest{
			ID: "child-execution", SessionID: "errand:child", WorkspacePath: workspace, ErrandMCPOwner: &owner,
		})
		created <- err
	}()
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("creation did not reach provisional startup")
	}

	_, published := mgr.GetInstance("child-execution")
	require.False(t, published)
	status := requestErrandStop(t, router, "child-execution", owner, "current-token")
	close(publish)
	publish = nil
	select {
	case err := <-created:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("creation did not finish")
	}
	require.Equal(t, http.StatusConflict, status)
}

func errandStopFixture(t *testing.T) (*instance.Manager, http.Handler) {
	t.Helper()
	cfg := &config.Config{AuthToken: "current-token", Ports: config.PortConfig{Base: 39000, Max: 39500}, Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP}}
	mgr := instance.NewManager(cfg, logger.Default())
	t.Cleanup(func() { require.NoError(t, mgr.Shutdown(context.Background())) })
	return mgr, NewControlServer(cfg, mgr, logger.Default()).Router()
}

func requestErrandStop(t *testing.T, router http.Handler, id string, owner agentctltypes.ErrandMCPOwner, token string) int {
	t.Helper()
	body, err := json.Marshal(owner)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/"+id+"/stop-errand", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp.Code
}
