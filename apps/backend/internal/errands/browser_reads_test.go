package errands

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type browserReadMemoryStore struct {
	Store
	summary       models.ErrandBrowserSummary
	page          models.ErrandHistoryPage
	inspection    models.ErrandBrowserInspection
	pageRequest   models.ErrandHistoryPageRequest
	summaryTaskID string
	inspectIDs    [2]string
	err           error
}

func (s *browserReadMemoryStore) GetErrandBrowserSummary(_ context.Context, taskID string) (models.ErrandBrowserSummary, error) {
	s.summaryTaskID = taskID
	return s.summary, s.err
}

func (s *browserReadMemoryStore) GetErrandHistoryPage(_ context.Context, request models.ErrandHistoryPageRequest) (models.ErrandHistoryPage, error) {
	s.pageRequest = request
	return s.page, s.err
}

func (s *browserReadMemoryStore) GetErrandInspection(_ context.Context, taskID, childID string) (models.ErrandBrowserInspection, error) {
	s.inspectIDs = [2]string{taskID, childID}
	return s.inspection, s.err
}

func browserReadService(t *testing.T) (*harness, *browserReadMemoryStore) {
	t.Helper()
	h := newHarness(t)
	store := &browserReadMemoryStore{Store: h.store}
	h.service.store = store
	h.service.SetEnabled(func() bool { return false })
	return h, store
}

func TestErrandBrowserSummaryIsAvailableWithFeatureFlagOffAndUsesConfiguredCapacity(t *testing.T) {
	h, store := browserReadService(t)
	store.summary = models.ErrandBrowserSummary{
		TaskID: testTask,
		Counts: models.ErrandBrowserCounts{Live: 2},
		ParentCounts: []models.ErrandParentBrowserCounts{{
			ParentSessionID: testParent, Counts: models.ErrandBrowserCounts{Live: 1},
		}},
	}

	got, err := h.service.BrowserSummary(t.Context(), "  "+testTask+"  ")
	require.NoError(t, err)
	require.Equal(t, testTask, store.summaryTaskID)
	require.Equal(t, MaxLiveErrands, got.Counts.Capacity)
	require.Equal(t, MaxLiveErrands, got.ParentCounts[0].Counts.Capacity)
}

func TestErrandHistoryPageValidatesBoundsAndScopesOpaqueCursor(t *testing.T) {
	h, store := browserReadService(t)
	position := models.ErrandHistoryCursor{
		Version: 1, TaskID: testTask, ParentSessionID: testParent, Order: errHistoryOrder,
		CreatedAt: time.Date(2026, time.November, 13, 9, 0, 0, 0, time.UTC), ID: "errand-25",
	}
	store.page.NextCursor = &position

	page, token, err := h.service.HistoryPage(t.Context(), models.ErrandHistoryPageRequest{
		TaskID: " " + testTask + " ", ParentSessionID: " " + testParent + " ",
	}, "")
	require.NoError(t, err)
	require.Equal(t, models.ErrandHistoryPageRequest{TaskID: testTask, ParentSessionID: testParent, Limit: ErrandHistoryPageSize}, store.pageRequest)
	require.Equal(t, store.page, page)
	decoded, err := decodeErrandHistoryCursor(token, testTask, testParent)
	require.NoError(t, err)
	require.Equal(t, &position, decoded)

	_, _, err = h.service.HistoryPage(t.Context(), models.ErrandHistoryPageRequest{TaskID: testTask, ParentSessionID: testParent, Limit: 26}, "")
	require.ErrorIs(t, err, ErrInvalidRequest)
	_, _, err = h.service.HistoryPage(t.Context(), models.ErrandHistoryPageRequest{TaskID: testTask, ParentSessionID: testParent, Limit: -1}, "")
	require.ErrorIs(t, err, ErrInvalidRequest)
	_, _, err = h.service.HistoryPage(t.Context(), models.ErrandHistoryPageRequest{TaskID: testTask, ParentSessionID: testParent}, "not-a-cursor")
	require.ErrorIs(t, err, ErrInvalidRequest)

	foreign, err := encodeErrandHistoryCursor(models.ErrandHistoryCursor{
		Version: 1, TaskID: "other-task", ParentSessionID: testParent, Order: errHistoryOrder,
		CreatedAt: position.CreatedAt, ID: "errand-foreign",
	})
	require.NoError(t, err)
	_, _, err = h.service.HistoryPage(t.Context(), models.ErrandHistoryPageRequest{TaskID: testTask, ParentSessionID: testParent}, foreign)
	require.ErrorIs(t, err, ErrInvalidRequest)
	// Invalid cursors are rejected before the optional repository capability runs.
	require.Equal(t, models.ErrandHistoryPageRequest{TaskID: testTask, ParentSessionID: testParent, Limit: ErrandHistoryPageSize}, store.pageRequest)
}

func TestErrandHistoryCursorRoundTripsValidZeroTimestamp(t *testing.T) {
	h, store := browserReadService(t)
	position := models.ErrandHistoryCursor{
		Version: 1, TaskID: testTask, ParentSessionID: testParent, Order: errHistoryOrder,
		CreatedAt: time.Time{}, ID: "errand-year-one-a",
	}
	store.page.NextCursor = &position
	request := models.ErrandHistoryPageRequest{TaskID: testTask, ParentSessionID: testParent, Limit: 1}
	_, token, err := h.service.HistoryPage(t.Context(), request, "")
	require.NoError(t, err)
	require.NotEmpty(t, token)
	store.page.NextCursor = nil
	_, _, err = h.service.HistoryPage(t.Context(), request, token)
	require.NoError(t, err)
	require.Equal(t, &position, store.pageRequest.Cursor)
}

func TestErrandHistoryPageRejectsRepositoryCursorFromAnotherScope(t *testing.T) {
	h, store := browserReadService(t)
	store.page.NextCursor = &models.ErrandHistoryCursor{
		Version: 1, TaskID: "other-task", ParentSessionID: testParent, Order: errHistoryOrder,
		CreatedAt: time.Now().UTC(), ID: "errand-1",
	}

	_, _, err := h.service.HistoryPage(t.Context(), models.ErrandHistoryPageRequest{TaskID: testTask, ParentSessionID: testParent}, "")
	require.ErrorContains(t, err, "out-of-scope")
}

func TestErrandBrowserReadsPropagateRepositoryErrorsAndTrimInspectionIdentity(t *testing.T) {
	h, store := browserReadService(t)
	store.err = errors.New("storage unavailable")
	_, err := h.service.BrowserSummary(t.Context(), testTask)
	require.ErrorContains(t, err, "storage unavailable")
	_, _, err = h.service.HistoryPage(t.Context(), models.ErrandHistoryPageRequest{TaskID: testTask, ParentSessionID: testParent}, "")
	require.ErrorContains(t, err, "storage unavailable")
	_, err = h.service.Inspect(t.Context(), " "+testTask+" ", " child-1 ")
	require.ErrorContains(t, err, "storage unavailable")
	require.Equal(t, [2]string{testTask, "child-1"}, store.inspectIDs)
}
