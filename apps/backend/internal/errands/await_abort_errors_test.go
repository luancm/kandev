package errands

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type awaitLoadFailureStore struct{ Store }

func (s *awaitLoadFailureStore) GetErrand(context.Context, string) (*models.Errand, error) {
	return nil, errors.New("await inventory read failed")
}

func TestAwaitErrorBeforeClaimCommitsAbortTombstone(t *testing.T) {
	exerciseAwaitErrorBeforeClaim(t, false)
}

func TestPostgresAwaitErrorBeforeClaimCommitsAbortTombstone(t *testing.T) {
	exerciseAwaitErrorBeforeClaim(t, true)
}

func exerciseAwaitErrorBeforeClaim(t *testing.T, postgres bool) {
	t.Helper()
	ctx := t.Context()
	conn := serviceClaimDB(t, postgres)
	repo, err := sqlite.NewWithDB(conn, conn, nil)
	require.NoError(t, err)
	seedServiceClaimRows(t, repo)
	h := newHarness(t)
	h.service.store = &awaitLoadFailureStore{Store: repo}
	h.service.deliverer = nil
	owner := testAwaitOwner("load-error")
	_, err = h.service.Await(ctx, AwaitRequest{
		Owner: owner, ErrandIDs: []string{"claim-a"}, Mode: awaitModeAll,
	})
	require.ErrorContains(t, err, "await inventory read failed")
	var state, incarnation string
	var generation int64
	require.NoError(t, conn.QueryRow(`SELECT state,parent_incarnation_id,harness_generation
		FROM errand_await_attempt WHERE token='load-error'`).Scan(&state, &incarnation, &generation))
	require.Equal(t, "aborted", state)
	require.Equal(t, owner.ParentIncarnationID, incarnation)
	require.Equal(t, owner.HarnessGeneration, generation)
	row, err := repo.GetErrand(ctx, "claim-a")
	require.NoError(t, err)
	_, err = repo.ClaimResults(ctx, testTask, testParent, models.ErrandDeliveryAwait, owner.Token,
		[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, time.Now().UTC(), serviceClaimInventory(t, repo, testParent))
	require.ErrorIs(t, err, models.ErrErrandVersionConflict, "known error must fence a delayed claim")
}
