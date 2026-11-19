package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

func TestControlErrandStopRequiresConfirmation(t *testing.T) {
	owner := agentctltypes.ErrandMCPOwner{TaskID: "task", ErrandID: "child", Generation: 2}
	for _, status := range []int{http.StatusNoContent, http.StatusNotFound, http.StatusConflict, http.StatusUnauthorized, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			requests := make(chan agentctltypes.ErrandMCPOwner, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/instances/execution/stop-errand" ||
					r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer control-token" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var got agentctltypes.ErrandMCPOwner
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				requests <- got
				w.WriteHeader(status)
			}))
			t.Cleanup(srv.Close)
			ctl := newTestControlClient(t, srv, WithControlAuthToken("control-token"))
			t.Cleanup(ctl.Close)
			err := ctl.StopErrandInstance(t.Context(), "execution", owner)
			select {
			case got := <-requests:
				require.Equal(t, owner, got)
			default:
				t.Fatal("no owner-checked request reached agentctl")
			}
			if status == http.StatusNoContent {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
