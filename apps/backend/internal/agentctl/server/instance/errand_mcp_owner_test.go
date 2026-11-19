package instance

import (
	"context"
	"encoding/json"
	"testing"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

func TestInstanceInfoRetainsErrandMCPOwner(t *testing.T) {
	mgr := newConcurrencyTestManager(t, 1)
	owner := &agentctltypes.ErrandMCPOwner{TaskID: "task", ErrandID: "child", Generation: 2}
	_, err := mgr.CreateInstance(context.Background(), &CreateRequest{ID: "instance", SessionID: "errand:child", WorkspacePath: t.TempDir(), ErrandMCPOwner: owner})
	require.NoError(t, err)
	defer func() { require.NoError(t, mgr.StopInstance(context.Background(), "instance")) }()
	inst, ok := mgr.GetInstance("instance")
	require.True(t, ok)
	data, err := json.Marshal(inst.Info())
	require.NoError(t, err)
	var payload struct {
		Owner *agentctltypes.ErrandMCPOwner `json:"errand_mcp_owner"`
	}
	require.NoError(t, json.Unmarshal(data, &payload))
	require.Equal(t, owner, payload.Owner)
	info := inst.Info()
	info.ErrandMCPOwner.TaskID = "changed"
	require.Equal(t, "task", inst.Info().ErrandMCPOwner.TaskID)
}
