package errands

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// This fixture has no queue; durable prepared ownership uses the SQL fixture.
func (*memoryStore) HasErrandPreparedOwnership(context.Context, string, string, models.ErrandResultClaim) (bool, error) {
	return false, nil
}

// @covers AC-TASKS-ERRANDS-003.5
func TestPreserveCancellationWithoutDelivererPreventsDeletion(t *testing.T) {
	h := newHarness(t)
	row := h.mustStart("audit").Errand
	h.service.SetDeliverer(nil)
	require.Error(t, h.service.PreserveCancellationNotice(t.Context(), row.ID), "missing durable admission authority cannot authorize deletion")
	retained, err := h.store.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultPending, retained.ResultState)
	require.Equal(t, cancellationNotice, retained.ResultText)
}
