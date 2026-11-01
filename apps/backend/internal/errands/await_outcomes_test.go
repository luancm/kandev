package errands

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-004.4
func TestAwaitResponseCarriesOnlyCommittedRevisionReceipts(t *testing.T) {
	exerciseAwaitResponseIdentity(t, false)
}
func TestPostgresAwaitResponseCarriesOnlyCommittedRevisionReceipts(t *testing.T) {
	exerciseAwaitResponseIdentity(t, true)
}
func exerciseAwaitResponseIdentity(t *testing.T, postgres bool) {
	t.Helper()
	ctx := t.Context()
	conn := serviceClaimDB(t, postgres)
	repo, err := sqlite.NewWithDB(conn, conn, nil)
	require.NoError(t, err)
	seedServiceClaimRows(t, repo)
	h := newHarness(t)
	h.service.store = repo
	h.service.deliverer = nil
	b, err := repo.GetErrand(ctx, "claim-b")
	require.NoError(t, err)
	all, err := repo.ListErrandsByParent(ctx, testParent)
	require.NoError(t, err)
	_, err = repo.ClaimResults(ctx, testTask, testParent, models.ErrandDeliveryTurnContext, "already", []models.ErrandClaimCandidate{{ID: b.ID, Version: b.Version}}, h.service.clock.Now(), models.ErrandInventory(all))
	require.NoError(t, err)
	id := models.ErrandAwaitIdentity{TaskID: testTask, ParentSessionID: testParent, Token: "response", ParentIncarnationID: "launched", HarnessGeneration: 3, SubmissionID: "original"}
	result, err := h.service.Await(ctx, AwaitRequest{Owner: &id, ParentSessionID: testParent, Token: id.Token, ErrandIDs: []string{"claim-b", "claim-a"}, Mode: "all"})
	require.NoError(t, err)
	require.Equal(t, 1, result.Delivery.Version)
	require.Equal(t, id.Token, result.Delivery.Token)
	require.Len(t, result.Delivery.Items, 1)
	item := result.Delivery.Items[0]
	require.Equal(t, "claim-a", item.ErrandID)
	require.NotEmpty(t, item.ResultRevision)
	require.Empty(t, result.Errands[0].Result)
	require.Equal(t, "full result claim-a", result.Errands[1].Result)
	require.NoError(t, h.service.OfferAwait(ctx, id, result.Delivery))
	var state string
	require.NoError(t, conn.Get(&state, "SELECT state FROM errand_await_attempt WHERE token='response'"))
	require.Equal(t, "offered", state, "success is not receipt")
	foreign := id
	foreign.HarnessGeneration++
	_, err = h.service.Await(ctx, AwaitRequest{Owner: &foreign, ParentSessionID: testParent, Token: id.Token, ErrandIDs: []string{"claim-a"}, Mode: "all"})
	require.Error(t, err)
	require.NoError(t, conn.Get(&state, "SELECT state FROM errand_await_attempt WHERE token='response'"))
	require.Equal(t, "offered", state, "foreign begin error cannot abort original attempt")
	next := id
	next.Token = "status-only"
	status, err := h.service.Await(ctx, AwaitRequest{Owner: &next, ParentSessionID: testParent, Token: next.Token, ErrandIDs: []string{"claim-a", "claim-b"}, Mode: "all"})
	require.NoError(t, err)
	require.Empty(t, status.Delivery.Items)
	require.NoError(t, h.service.OfferAwait(ctx, next, status.Delivery))
	row, err := repo.GetErrand(ctx, "claim-a")
	require.NoError(t, err)
	row.ResultTurnID = "distinct-turn"
	row.ResultState = models.ErrandResultPending
	row.DeliveredRoute = ""
	row.DeliveryToken = ""
	row.DeliveredAt = nil
	require.NoError(t, repo.UpdateErrand(ctx, row))
	require.NotEqual(t, item.ResultRevision, row.ResultRevision)
	successor := id
	successor.Token = "equal-text-distinct-revision"
	later, err := h.service.Await(ctx, AwaitRequest{Owner: &successor, ParentSessionID: testParent, Token: successor.Token, ErrandIDs: []string{"claim-a"}, Mode: "all"})
	require.NoError(t, err)
	require.Equal(t, result.Errands[1].Result, later.Errands[0].Result)
	require.Equal(t, row.ResultRevision, later.Delivery.Items[0].ResultRevision)
	require.NoError(t, h.service.AbortOwnedAwait(ctx, id))
	current, err := repo.GetErrand(ctx, "claim-a")
	require.NoError(t, err)
	require.Equal(t, successor.Token, current.DeliveryToken)
}
