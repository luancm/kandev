package errands

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

type compactParentStateStore struct {
	Store
	has, outstanding bool
	err              error
	reads, lists     int
	parent           string
}

func (s *compactParentStateStore) GetParentErrandState(_ context.Context, parent string) (bool, bool, error) {
	s.reads++
	s.parent = parent
	return s.has, s.outstanding, s.err
}

func (s *compactParentStateStore) ListErrandsByParent(context.Context, string) ([]*models.Errand, error) {
	s.lists++
	return nil, errors.New("parent state must not materialize children")
}

func TestParentErrandStateUsesCompactRead(t *testing.T) {
	for _, tc := range []struct {
		name             string
		has, outstanding bool
		err              error
	}{
		{name: "empty"}, {name: "finished", has: true}, {name: "outstanding", has: true, outstanding: true},
		{name: "read error", err: errors.New("unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			store := &compactParentStateStore{Store: h.store, has: tc.has, outstanding: tc.outstanding, err: tc.err}
			h.service.store = store
			has, outstanding, err := h.service.ParentErrandState(t.Context(), testParent)
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, tc.has, has)
			require.Equal(t, tc.outstanding, outstanding)
			require.Equal(t, 1, store.reads)
			require.Equal(t, testParent, store.parent)
			require.Zero(t, store.lists)
		})
	}
}

func TestParentErrandStateSkipsDisabledAndEmptyReads(t *testing.T) {
	h := newHarness(t)
	store := &compactParentStateStore{Store: h.store}
	h.service.store = store
	for _, enabled := range []bool{false, true} {
		h.service.enabled = func() bool { return enabled }
		parent := ""
		if !enabled {
			parent = testParent
		}
		has, outstanding, err := h.service.ParentErrandState(t.Context(), parent)
		require.NoError(t, err)
		require.False(t, has)
		require.False(t, outstanding)
	}
	require.Zero(t, store.reads)
	require.Zero(t, store.lists)
}
