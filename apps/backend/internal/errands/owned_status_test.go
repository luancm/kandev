package errands

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type missingOwnedStatusStore struct {
	Store
	sessionReads int
}

func (*missingOwnedStatusStore) ReadErrandStatuses(context.Context, string, string) (map[string]models.ErrandBrowserObservation, error) {
	return nil, nil
}

func (s *missingOwnedStatusStore) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	s.sessionReads++
	return s.Store.GetTaskSession(ctx, id)
}

func TestErrandSnapshotRequiresAnOwnedObservation(t *testing.T) {
	h := newHarness(t)
	row := h.mustStart("disappearing Errand").Errand
	store := &missingOwnedStatusStore{Store: h.store}
	h.service.store = store

	_, err := h.service.ProjectSnapshot(t.Context(), row)
	require.ErrorIs(t, err, models.ErrErrandNotFound)
	require.Zero(t, store.sessionReads)
}
