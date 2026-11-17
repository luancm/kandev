package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

func TestErrandInstanceRequiresOwnerProjectionCapability(t *testing.T) {
	for _, tc := range []struct {
		name      string
		owner     bool
		supported bool
		want      bool
	}{{"old", true, false, false}, {"current", true, true, true}, {"ordinary", false, false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			created, identities := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity" {
					identities++
					caps := []string{}
					if tc.supported {
						caps = append(caps, "errand-mcp-owner.v1")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": caps})
					return
				}
				created++
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":"instance","port":12345}`))
			}))
			defer server.Close()
			req := &CreateInstanceRequest{}
			if tc.owner {
				req.ErrandMCPOwner = &agentctltypes.ErrandMCPOwner{TaskID: "task", ErrandID: "child", Generation: 1}
			}
			_, err := newTestControlClient(t, server).CreateInstance(context.Background(), req)
			if tc.want {
				require.NoError(t, err)
				require.Equal(t, 1, created)
			} else {
				require.Error(t, err)
				require.Zero(t, created)
			}
			if !tc.owner {
				require.Zero(t, identities)
			}
		})
	}
}
