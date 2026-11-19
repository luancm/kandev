package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func TestParentErrandStateProjection(t *testing.T) {
	exerciseParentErrandStateProjection(t, newRepoForSessionTests(t))
}
func TestPostgresParentErrandStateProjection(t *testing.T) {
	exerciseParentErrandStateProjection(t, newPostgresErrandRepo(t))
}
func exerciseParentErrandStateProjection(t *testing.T, r *Repository) {
	task := newErrandTask(t, r, "parent-state")
	assertParentState(t, r, "absent", false, false)
	for _, tc := range []struct {
		name      string
		lifecycle models.ErrandLifecycle
		awaiting  bool
		result    models.ErrandResultState
		want      bool
	}{
		{"finished", models.ErrandLifecycleLaunched, false, models.ErrandResultDelivered, false},
		{"queued", models.ErrandLifecycleQueued, false, models.ErrandResultNone, true},
		{"awaiting", models.ErrandLifecycleLaunched, true, models.ErrandResultDelivered, true},
		{"pending", models.ErrandLifecycleLaunched, false, models.ErrandResultPending, true},
		{"cancelled", models.ErrandLifecycleCancelled, false, models.ErrandResultDelivered, false},
		{"cancelled pending", models.ErrandLifecycleCancelled, false, models.ErrandResultPending, true},
	} {
		row := sampleErrand("child-"+tc.name, task, tc.name, errandScenarioBase)
		markErrandFinished(row)
		row.Lifecycle, row.AwaitingResult, row.ResultState = tc.lifecycle, tc.awaiting, tc.result
		if row.Lifecycle == models.ErrandLifecycleCancelled {
			row.CancelSource = models.ErrandCancelSourceUser
		}
		mustCreateErrand(t, r, row)
		// Missing child sessions must not erase reservations or pending results.
		assertParentState(t, r, tc.name, true, tc.want)
	}
	assertParentState(t, r, "absent", false, false)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err := r.GetParentErrandState(ctx, "finished")
	require.ErrorIs(t, err, context.Canceled)
}

func assertParentState(t *testing.T, r *Repository, parent string, wantHas, wantOutstanding bool) {
	t.Helper()
	has, outstanding, err := r.GetParentErrandState(t.Context(), parent)
	require.NoError(t, err)
	require.Equal(t, wantHas, has, parent)
	require.Equal(t, wantOutstanding, outstanding, parent)
}

func TestParentErrandStateLargeHistoryUsesScopedScalarQuery(t *testing.T) {
	r := newRepoForSessionTests(t)
	task := newErrandTask(t, r, "parent-state-history")
	for i := range 1000 {
		row := sampleErrand(fmt.Sprintf("history-%04d", i), task, "parent", errandScenarioBase)
		markErrandFinished(row)
		mustCreateErrand(t, r, row)
	}
	assertParentState(t, r, "parent", true, false)
	rows, err := r.ro.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+parentErrandStateQuery(r.ro.DriverName()), "parent", "parent")
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	var parentIndexUses int
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		if strings.Contains(detail, "idx_task_session_errands_parent") {
			parentIndexUses++
		}
	}
	require.NoError(t, rows.Err())
	require.Equal(t, 2, parentIndexUses, "both existence checks must scope by parent before examining children")
}
