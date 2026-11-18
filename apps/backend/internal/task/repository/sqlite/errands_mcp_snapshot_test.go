package sqlite

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func TestErrandMCPSnapshotRoundTripsOneFormat(t *testing.T) {
	for _, row := range []models.Errand{
		{},
		{MCPServerIDs: []string{"added"}},
		{MCPExcludedServerIDs: []string{"blocked"}},
		{MCPServerIDs: []string{"added"}, MCPExcludedServerIDs: []string{"blocked"}},
	} {
		stored, err := encodeErrandMCPSnapshot(&row)
		require.NoError(t, err)
		require.Contains(t, stored, `"server_ids"`)
		require.Contains(t, stored, `"excluded_server_ids"`)
		var restored models.Errand
		require.NoError(t, decodeErrandMCPSnapshot(stored, &restored))
		require.Equal(t, row, restored)
	}
	require.Error(t, decodeErrandMCPSnapshot(`["old"]`, &models.Errand{}))
}
