package lifecycle

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestErrandRecoveryStopRequiresVerifiedControlAbsence(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			observed := make(chan agentctltypes.ErrandMCPOwner, 1)
			control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/instances/execution/stop-errand" ||
					r.Header.Get("Authorization") != "Bearer test-token" {
					http.NotFound(w, r)
					return
				}
				var got agentctltypes.ErrandMCPOwner
				if json.NewDecoder(r.Body).Decode(&got) != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				observed <- got
				w.WriteHeader(status)
			}))
			t.Cleanup(control.Close)
			ctx := t.Context()
			repo, _ := runOwnerTestRepository(t)
			mgr := newTestManager(t)
			mgr.SetExecutorRunningWriter(repo)
			host, port := parseHTTPURL(t, control.URL)
			ctl := agentctl.NewControlClient(host, port, mgr.logger, agentctl.WithControlAuthToken("test-token"))
			t.Cleanup(ctl.Close)
			backend := NewStandaloneExecutor(ctl, host, port, mgr.logger)
			mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
			mgr.executorRegistry.Register(backend)
			owner := ExecutionOwner{Kind: ExecutionOwnerErrand, WorkspaceID: "ws", TaskID: "parent", ErrandID: "child", Generation: 1}
			execution := &AgentExecution{ID: "execution", WorkspaceID: "ws", Owner: owner, OwnerAdmission: allowRunOwner{}, Status: v1.AgentStatusRunning, RuntimeName: backend.Name()}
			require.NoError(t, mgr.registerAndPublishExecution(ctx, execution, backend, &ExecutorInstance{InstanceID: execution.ID}, ""))
			mgr.RemoveExecution(execution.ID)

			err := mgr.StopErrandOwnerForRecovery(ctx, owner)
			row, readErr := repo.GetExecutorRunningBySessionID(ctx, "errand:child")
			require.NoError(t, readErr)
			if status == http.StatusNoContent {
				require.NoError(t, err)
				require.Equal(t, models.ExecutorRunningStatusStopped, row.Status)
			} else {
				require.Error(t, err)
				require.NotEqual(t, models.ExecutorRunningStatusStopped, row.Status)
			}
			select {
			case got := <-observed:
				require.Equal(t, *errandMCPOwner(owner), got)
			default:
				t.Fatal("exact owner-checked control request was never made")
			}
		})
	}
}
