package sqlite

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type awaitRecoveryRepository interface {
	ListUnresolvedErrandAwaits(context.Context, string) ([]models.ErrandAwaitIdentity, error)
	RecoverErrandAwait(context.Context, models.ErrandAwaitIdentity) (bool, error)
}

func recoveryRepository(t *testing.T, r *Repository) awaitRecoveryRepository {
	t.Helper()
	recovery, ok := any(r).(awaitRecoveryRepository)
	require.True(t, ok, "repository must classify retained awaits under the task guard")
	return recovery
}

func deliveryProjection(t *testing.T, row *models.Errand) string {
	t.Helper()
	return string(row.DeliveryState)
}

func assertAwaitProjection(t *testing.T, r *Repository, id, expected string) *models.Errand {
	t.Helper()
	row, err := r.GetErrand(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, expected, deliveryProjection(t, row), "detail projection")
	for _, read := range []func() ([]*models.Errand, error){
		func() ([]*models.Errand, error) { return r.ListErrandsByTask(t.Context(), row.TaskID) },
		func() ([]*models.Errand, error) { return r.ListErrandsByParent(t.Context(), row.ParentSessionID) },
	} {
		rows, err := read()
		require.NoError(t, err)
		found := false
		for _, item := range rows {
			if item.ID == id {
				found = true
				require.Equal(t, expected, deliveryProjection(t, item), "list/reconnect projection")
			}
		}
		require.True(t, found)
	}
	return row
}

// @covers AC-TASKS-ERRANDS-004.8
func TestErrandAwaitCurrentReceiptProjection(t *testing.T) {
	exerciseAwaitCurrentReceiptProjection(t, newRepoForSessionTests(t))
}

func TestPostgresErrandAwaitCurrentReceiptProjection(t *testing.T) {
	exerciseAwaitCurrentReceiptProjection(t, newPostgresErrandRepo(t))
}

func exerciseAwaitCurrentReceiptProjection(t *testing.T, r *Repository) {
	t.Helper()
	row, _, event := receiptFixture(t, r)
	assertAwaitProjection(t, r, row.ID, "")
	_, err := r.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
	require.NoError(t, err)
	after := assertAwaitProjection(t, r, row.ID, "received")
	require.Equal(t, row.Version, after.Version)
	require.Equal(t, row.ResultRevision, after.ResultRevision)
	require.Equal(t, row.Lifecycle, after.Lifecycle)
}

// @covers AC-TASKS-ERRANDS-004.9
func TestErrandAwaitRecoveredClassificationPreservesClaim(t *testing.T) {
	exerciseAwaitRecoveredClassification(t, newRepoForSessionTests(t))
}

func TestPostgresErrandAwaitRecoveredClassificationPreservesClaim(t *testing.T) {
	exerciseAwaitRecoveredClassification(t, newPostgresErrandRepo(t))
}

func exerciseAwaitRecoveredClassification(t *testing.T, r *Repository) {
	t.Helper()
	row, id, event := receiptFixture(t, r)
	recovery := recoveryRepository(t, r)
	for _, state := range []string{"open", "claimed", "offered"} {
		_, err := r.db.Exec(r.db.Rebind("UPDATE errand_await_attempt SET state=? WHERE token=?"), state, id.Token)
		require.NoError(t, err)
		assertAwaitProjection(t, r, row.ID, "")
		changed, err := recovery.RecoverErrandAwait(t.Context(), id)
		require.NoError(t, err)
		require.True(t, changed)
		after := assertAwaitProjection(t, r, row.ID, "uncertain")
		require.Equal(t, row.DeliveryToken, after.DeliveryToken)
		require.Equal(t, row.ResultRevision, after.ResultRevision)
		require.Equal(t, row.ResultState, after.ResultState)
		require.Equal(t, row.Version, after.Version)
	}
	_, err := r.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
	require.NoError(t, err)
	changed, err := recovery.RecoverErrandAwait(t.Context(), id)
	require.NoError(t, err)
	require.False(t, changed, "recovery must reduce committed positive evidence before classification")
	assertAwaitProjection(t, r, row.ID, "received")
}

type awaitTerminalRepository interface {
	ClassifyErrandAwaitTerminal(context.Context, *models.AgentDeliveryEvent) error
}

func classifyAwaitTerminal(t *testing.T, r *Repository, event *models.AgentDeliveryEvent) {
	t.Helper()
	reducer, ok := any(r).(awaitTerminalRepository)
	require.True(t, ok, "ordered authenticated terminal must classify only its originating await")
	require.NoError(t, reducer.ClassifyErrandAwaitTerminal(t.Context(), event))
}

func awaitTerminalFixture(t *testing.T, r *Repository, id models.ErrandAwaitIdentity, source *models.AgentDeliveryEvent, turn, submission string) *models.AgentDeliveryEvent {
	t.Helper()
	_, err := r.db.Exec(r.db.Rebind("UPDATE errand_await_attempt SET turn_id=?,submission_id=? WHERE token=?"), id.TurnID, id.SubmissionID, id.Token)
	require.NoError(t, err)
	if submission != "" {
		_, err = r.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{
			ID: submission, SessionID: id.ParentSessionID, IncarnationID: id.ParentIncarnationID,
			HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "prompt-hash", Payload: []byte("prompt"), State: models.DeliverySubmissionDispatching,
		})
		require.NoError(t, err)
	}
	event := *source
	event.Terminal, event.EventType, event.SubmissionID = true, streams.EventTypeComplete, submission
	event.Payload, err = json.Marshal(streams.AgentEvent{Type: streams.EventTypeComplete, TurnID: turn})
	require.NoError(t, err)
	return &event
}

// @covers AC-TASKS-ERRANDS-004.13
func TestErrandAwaitTerminalUsesExactOriginAfterOrderedIngestion(t *testing.T) {
	exerciseAwaitTerminalOrigin(t, newRepoForSessionTests)
}

func TestPostgresErrandAwaitTerminalUsesExactOriginAfterOrderedIngestion(t *testing.T) {
	exerciseAwaitTerminalOrigin(t, newPostgresErrandRepo)
}

func exerciseAwaitTerminalOrigin(t *testing.T, create func(*testing.T) *Repository) {
	t.Helper()
	for _, scenario := range []string{"exact", "turn-only", "submission-only", "missing-origin", "wrong-turn", "wrong-submission", "terminal-missing-turn"} {
		t.Run(scenario, func(t *testing.T) {
			r := create(t)
			row, id, receipt := receiptFixture(t, r)
			id.TurnID, id.SubmissionID = "origin-turn", "origin-submission"
			turn, submission := id.TurnID, id.SubmissionID
			switch scenario {
			case "turn-only":
				id.SubmissionID = ""
			case "submission-only":
				id.TurnID = ""
			case "missing-origin":
				id.TurnID, id.SubmissionID = "", ""
			case "wrong-turn":
				turn = "different-turn"
			case "wrong-submission":
				submission = "different-submission"
			case "terminal-missing-turn":
				turn = ""
			}
			terminal := awaitTerminalFixture(t, r, id, receipt, turn, submission)
			_, err := r.ReceiveAgentDeliveryEvent(t.Context(), terminal, 1)
			require.NoError(t, err)
			classifyAwaitTerminal(t, r, terminal)
			assertAwaitProjection(t, r, row.ID, "")
			_, err = r.ProjectAgentDeliveryEvent(t.Context(), terminal, nil)
			require.NoError(t, err)
			classifyAwaitTerminal(t, r, terminal)
			expected := ""
			if scenario == "exact" || scenario == "turn-only" || scenario == "submission-only" {
				expected = "uncertain"
			}
			assertAwaitProjection(t, r, row.ID, expected)
			if expected == "" {
				_, err = recoveryRepository(t, r).RecoverErrandAwait(t.Context(), id)
				require.NoError(t, err)
				assertAwaitProjection(t, r, row.ID, "uncertain")
			}
			receipt.Sequence = 2
			_, err = r.ReceiveAgentDeliveryEvent(t.Context(), receipt, 2)
			require.NoError(t, err)
			after := assertAwaitProjection(t, r, row.ID, "received")
			require.Equal(t, row.Version, after.Version)
			require.Equal(t, row.ResultState, after.ResultState)
		})
	}
}

func TestErrandAwaitProjectionLegacyZeroAndOmission(t *testing.T) {
	exerciseAwaitProjectionOmission(t, newRepoForSessionTests)
}

func TestPostgresErrandAwaitProjectionLegacyZeroAndOmission(t *testing.T) {
	exerciseAwaitProjectionOmission(t, newPostgresErrandRepo)
}

func exerciseAwaitProjectionOmission(t *testing.T, create func(*testing.T) *Repository) {
	t.Helper()
	for _, scenario := range []string{"zero", "legacy-no-attempt", "legacy-owner", "pending", "superseded", "discarded", "other-route", "aborted-conflict"} {
		t.Run(scenario, func(t *testing.T) {
			r := create(t)
			count := 1
			if scenario == "zero" {
				count = 0
			}
			row, id, event := receiptFixtureItems(t, r, count)
			expected := ""
			switch scenario {
			case "legacy-no-attempt":
				_, err := r.db.Exec("DELETE FROM errand_await_attempt WHERE token='attempt'")
				require.NoError(t, err)
				expected = "uncertain"
			case "legacy-owner":
				_, err := r.db.Exec("UPDATE errand_await_attempt SET parent_incarnation_id='',harness_generation=0 WHERE token='attempt'")
				require.NoError(t, err)
				expected = "uncertain"
			case "pending", "superseded", "discarded":
				_, err := r.db.Exec(r.db.Rebind("UPDATE task_session_errands SET result_state=? WHERE id=?"), scenario, row.ID)
				require.NoError(t, err)
			case "other-route":
				_, err := r.db.Exec(r.db.Rebind("UPDATE task_session_errands SET delivered_route='turn_context' WHERE id=?"), row.ID)
				require.NoError(t, err)
			case "aborted-conflict":
				_, err := r.AbortErrandAwait(t.Context(), id)
				require.NoError(t, err)
			}
			if scenario != "legacy-no-attempt" && scenario != "legacy-owner" {
				_, err := recoveryRepository(t, r).RecoverErrandAwait(t.Context(), id)
				require.NoError(t, err)
			}
			assertAwaitProjection(t, r, row.ID, expected)
			if scenario == "zero" || scenario == "aborted-conflict" {
				_, err := r.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
				require.NoError(t, err)
				assertAwaitProjection(t, r, row.ID, "")
			}
		})
	}
}

func TestErrandAwaitTerminalReducesPositiveBeforeClassification(t *testing.T) {
	exerciseAwaitTerminalPositiveFirst(t, newRepoForSessionTests(t))
}

func TestPostgresErrandAwaitTerminalReducesPositiveBeforeClassification(t *testing.T) {
	exerciseAwaitTerminalPositiveFirst(t, newPostgresErrandRepo(t))
}

func exerciseAwaitTerminalPositiveFirst(t *testing.T, r *Repository) {
	t.Helper()
	row, id, receipt := receiptFixture(t, r)
	id.TurnID = "origin-turn"
	end := awaitTerminalFixture(t, r, id, receipt, id.TurnID, "")
	end.Sequence = 2
	_, err := r.ReceiveAgentDeliveryEvent(t.Context(), end, 2)
	require.NoError(t, err)
	_, err = r.ProjectAgentDeliveryEvent(t.Context(), end, nil)
	require.NoError(t, err)
	classifyAwaitTerminal(t, r, end)
	assertAwaitProjection(t, r, row.ID, "")
	_, err = r.ReceiveAgentDeliveryEvent(t.Context(), receipt, 2)
	require.NoError(t, err)
	_, err = r.ProjectAgentDeliveryEvent(t.Context(), receipt, nil)
	require.NoError(t, err)
	classifyAwaitTerminal(t, r, end)
	assertAwaitProjection(t, r, row.ID, "received")
}

func TestErrandAwaitOldReceiptCannotClearSuccessorWarning(t *testing.T) {
	exerciseAwaitSuccessorProjection(t, newRepoForSessionTests(t))
}

func TestPostgresErrandAwaitOldReceiptCannotClearSuccessorWarning(t *testing.T) {
	exerciseAwaitSuccessorProjection(t, newPostgresErrandRepo(t))
}

func exerciseAwaitSuccessorProjection(t *testing.T, r *Repository) {
	t.Helper()
	row, id, oldReceipt := receiptFixture(t, r)
	_, err := r.RecoverErrandAwait(t.Context(), id)
	require.NoError(t, err)
	assertAwaitProjection(t, r, row.ID, "uncertain")
	row.ResultText, row.ResultState, row.DeliveryToken, row.DeliveredRoute = "successor result", models.ErrandResultPending, "", ""
	require.NoError(t, r.UpdateErrand(t.Context(), row))
	assertAwaitProjection(t, r, row.ID, "")
	id.Token = "successor"
	require.NoError(t, r.BeginErrandAwait(t.Context(), id))
	inventory := claimInventory(t, r, id.ParentSessionID)
	claimed, err := r.ClaimResults(t.Context(), id.TaskID, id.ParentSessionID, models.ErrandDeliveryAwait, id.Token, inventory, errandScenarioBase, inventory)
	require.NoError(t, err)
	items := []models.ErrandAwaitDeliveryItem{{ErrandID: row.ID, ResultRevision: claimed[0].ResultRevision}}
	require.NoError(t, r.OfferErrandAwait(t.Context(), id, items))
	_, err = r.RecoverErrandAwait(t.Context(), id)
	require.NoError(t, err)
	before := assertAwaitProjection(t, r, row.ID, "uncertain")
	_, err = r.ReceiveAgentDeliveryEvent(t.Context(), oldReceipt, 1)
	require.NoError(t, err)
	after := assertAwaitProjection(t, r, row.ID, "uncertain")
	require.Equal(t, before, after)
	require.Equal(t, "received", receiptState(t, r, "attempt"))
	var observation map[string]any
	require.NoError(t, json.Unmarshal(oldReceipt.Payload, &observation))
	observation["errand_await_receipt"] = models.ErrandAwaitDelivery{Version: 1, Token: id.Token, Items: items}
	currentReceipt := *oldReceipt
	currentReceipt.Sequence = 2
	currentReceipt.Payload, err = json.Marshal(observation)
	require.NoError(t, err)
	_, err = r.ReceiveAgentDeliveryEvent(t.Context(), &currentReceipt, 2)
	require.NoError(t, err)
	assertAwaitProjection(t, r, row.ID, "received")
}

func TestErrandAwaitMutationSnapshotDoesNotReuseOldProjection(t *testing.T) {
	exerciseAwaitMutationProjection(t, newRepoForSessionTests(t))
}

func TestPostgresErrandAwaitMutationSnapshotDoesNotReuseOldProjection(t *testing.T) {
	exerciseAwaitMutationProjection(t, newPostgresErrandRepo(t))
}

func exerciseAwaitMutationProjection(t *testing.T, r *Repository) {
	t.Helper()
	row, id, receipt := receiptFixture(t, r)
	_, err := r.RecoverErrandAwait(t.Context(), id)
	require.NoError(t, err)
	stale := assertAwaitProjection(t, r, row.ID, "uncertain")
	_, err = r.ReceiveAgentDeliveryEvent(t.Context(), receipt, 1)
	require.NoError(t, err)
	stale.LatestSettledTurnID = "later-child-turn"
	require.NoError(t, r.UpdateErrand(t.Context(), stale))
	require.Equal(t, models.ErrandDeliveryReceived, stale.DeliveryState, "mutation response must use current committed evidence")
	stale.ResultText = "replacement result"
	require.NoError(t, r.UpdateErrand(t.Context(), stale))
	require.Empty(t, stale.DeliveryState, "replacement revision cannot inherit an older receipt")
	assertAwaitProjection(t, r, row.ID, "")
}
