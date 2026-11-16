package lifecycle

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// errandsAwaitBrowserFixture mutates only the isolated browser database through
// the same durable recovery and inbox boundaries used by the running backend.
func errandsAwaitBrowserFixture(t *testing.T, repo *tasksqlite.Repository, sessionID, action string) {
	t.Helper()
	ctx := t.Context()
	var id models.ErrandAwaitIdentity
	var state string
	require.NoError(t, repo.DB().QueryRowContext(ctx, `SELECT token,task_id,parent_session_id,parent_incarnation_id,harness_generation,turn_id,submission_id,state FROM errand_await_attempt WHERE parent_session_id=? AND EXISTS (SELECT 1 FROM errand_await_attempt_item i WHERE i.token=errand_await_attempt.token) ORDER BY created_at DESC LIMIT 1`, sessionID).Scan(&id.Token, &id.TaskID, &id.ParentSessionID, &id.ParentIncarnationID, &id.HarnessGeneration, &id.TurnID, &id.SubmissionID, &state))
	if action == "errand_offered" {
		require.Equal(t, "offered", state)
		fmt.Println("KANDEV_E2E_ERRAND_FIXTURE:offered")
		return
	}
	if action == "errand_recover" {
		changed, err := repo.RecoverErrandAwait(ctx, id)
		require.NoError(t, err)
		require.True(t, changed)
		fmt.Println("KANDEV_E2E_ERRAND_FIXTURE:recovered")
		return
	}
	rows, err := repo.DB().QueryContext(ctx, `SELECT errand_id,result_revision FROM errand_await_attempt_item WHERE token=? ORDER BY errand_id`, id.Token)
	require.NoError(t, err)
	items := make([]models.ErrandAwaitDeliveryItem, 0)
	for rows.Next() {
		var item models.ErrandAwaitDeliveryItem
		require.NoError(t, rows.Scan(&item.ErrandID, &item.ResultRevision))
		items = append(items, item)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	if action == "errand_zero" {
		id.Token += "-empty"
		require.NoError(t, repo.BeginErrandAwait(ctx, id))
		items = []models.ErrandAwaitDeliveryItem{}
		require.NoError(t, repo.OfferErrandAwait(ctx, id, items))
	}
	if action == "errand_supersede" {
		offerSuccessorAwaitFixture(t, repo, id, items)
	}
	native := nativeAwaitReplayFrame(t, models.ErrandAwaitDelivery{Version: 1, Token: id.Token, Items: items})
	if action == "errand_status" {
		native.ErrandAwaitReceipt = nil
	}
	payload, err := json.Marshal(native)
	require.NoError(t, err)
	running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, running)
	stream := "browser-await-" + id.Token + "-" + action
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, &models.AgentDeliveryEvent{
		TaskID: id.TaskID, SessionID: id.ParentSessionID, IncarnationID: id.ParentIncarnationID,
		HarnessGeneration: id.HarnessGeneration, OwnerGeneration: id.HarnessGeneration,
		ExecutionID: running.AgentExecutionID, ExpectedStreamID: stream, StreamID: stream,
		Sequence: 1, EventType: native.Type, Payload: payload,
	}, 1)
	require.NoError(t, err)
	fmt.Println("KANDEV_E2E_ERRAND_FIXTURE:received")
}

func offerSuccessorAwaitFixture(t *testing.T, repo *tasksqlite.Repository, previous models.ErrandAwaitIdentity, oldItems []models.ErrandAwaitDeliveryItem) {
	t.Helper()
	ctx := t.Context()
	id := previous
	id.Token += "-successor"
	rows, err := repo.ListErrandsByParent(ctx, id.ParentSessionID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, models.ErrandResultPending, rows[0].ResultState)
	require.NotEqual(t, oldItems[0].ResultRevision, rows[0].ResultRevision)
	require.NoError(t, repo.BeginErrandAwait(ctx, id))
	inventory := models.ErrandInventory(rows)
	_, err = repo.ClaimResults(ctx, id.TaskID, id.ParentSessionID, models.ErrandDeliveryAwait, id.Token, inventory, time.Now().UTC(), inventory)
	require.NoError(t, err)
	require.NoError(t, repo.OfferErrandAwait(ctx, id, []models.ErrandAwaitDeliveryItem{{ErrandID: rows[0].ID, ResultRevision: rows[0].ResultRevision}}))
	changed, err := repo.RecoverErrandAwait(ctx, id)
	require.NoError(t, err)
	require.True(t, changed)
}
