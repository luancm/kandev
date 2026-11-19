package sqlite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

func receiptFixture(t *testing.T, r *Repository) (*models.Errand, models.ErrandAwaitIdentity, *models.AgentDeliveryEvent) {
	t.Helper()
	return receiptFixtureItems(t, r, 1)
}

func receiptFixtureItems(t *testing.T, r *Repository, count int) (*models.Errand, models.ErrandAwaitIdentity, *models.AgentDeliveryEvent) {
	t.Helper()
	row := newAdmissionResult(t, r, "receipt")
	if count == 2 {
		second := sampleErrand("second", row.TaskID, row.ParentSessionID, errandScenarioBase)
		second.ResultText, second.ResultState = "second result", models.ErrandResultPending
		mustCreateErrand(t, r, second)
	}
	id := models.ErrandAwaitIdentity{TaskID: row.TaskID, ParentSessionID: row.ParentSessionID, ParentIncarnationID: "owner", HarnessGeneration: 1, Token: "attempt"}
	require.NoError(t, r.CreateTaskSession(t.Context(), &models.TaskSession{ID: id.ParentSessionID, TaskID: id.TaskID, QueueIncarnationID: id.ParentIncarnationID, State: models.TaskSessionStateRunning}))
	require.NoError(t, r.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "running", SessionID: id.ParentSessionID, TaskID: id.TaskID, AgentExecutionID: "execution", Status: models.ExecutorRunningStatusRunning}))
	require.NoError(t, r.CreateHarnessSessionGeneration(t.Context(), &models.HarnessSessionGeneration{SessionID: id.ParentSessionID, IncarnationID: id.ParentIncarnationID, Generation: 1, NativeSessionID: "native-parent", CreationReason: "initial"}))
	require.NoError(t, r.BeginErrandAwait(t.Context(), id))
	inventory := claimInventory(t, r, id.ParentSessionID)
	var rows []*models.Errand
	if count > 0 {
		var err error
		rows, err = r.ClaimResults(t.Context(), id.TaskID, id.ParentSessionID, models.ErrandDeliveryAwait, id.Token, inventory, errandScenarioBase, inventory)
		require.NoError(t, err)
		row = rows[0]
	}
	items := make([]models.ErrandAwaitDeliveryItem, 0, len(rows))
	for _, claimed := range rows {
		items = append(items, models.ErrandAwaitDeliveryItem{ErrandID: claimed.ID, ResultRevision: claimed.ResultRevision})
	}
	require.NoError(t, r.OfferErrandAwait(t.Context(), id, items))
	payload, err := json.Marshal(map[string]any{"type": "tool_update", "tool_name": "kandev/await_errands_kandev", "tool_call_id": "native-call", "tool_status": "complete", "errand_await_receipt": map[string]any{"version": 1, "token": id.Token, "items": items}})
	require.NoError(t, err)
	return row, id, &models.AgentDeliveryEvent{TaskID: id.TaskID, SessionID: id.ParentSessionID, IncarnationID: id.ParentIncarnationID, HarnessGeneration: 1, ExecutionID: "execution", OwnerGeneration: 1, ExpectedStreamID: "stream", StreamID: "stream", Sequence: 1, EventType: streams.EventTypeToolUpdate, Payload: payload, ReceivedAt: time.Now().UTC()}
}

func receiptState(t *testing.T, r *Repository, token string) string {
	t.Helper()
	var state string
	require.NoError(t, r.db.Get(&state, r.db.Rebind("SELECT state FROM errand_await_attempt WHERE token=?"), token))
	return state
}

// @covers AC-TASKS-ERRANDS-004.8
// @covers AC-TASKS-ERRANDS-004.9
func TestErrandAwaitReceiptAndInboxCommitAtomically(t *testing.T) {
	exerciseReceiptCommit(t, newRepoForSessionTests(t))
}
func TestPostgresErrandAwaitReceiptAndInboxCommitAtomically(t *testing.T) {
	exerciseReceiptCommit(t, newPostgresErrandRepo(t))
}
func exerciseReceiptCommit(t *testing.T, r *Repository) {
	t.Helper()
	_, id, event := receiptFixture(t, r)
	inserted, err := r.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
	require.NoError(t, err)
	require.True(t, inserted)
	require.Equal(t, "received", receiptState(t, r, id.Token), "inbox acceptance must also commit the native receipt")
	var firstHash, firstCall string
	var firstSequence int64
	require.NoError(t, r.db.QueryRow("SELECT receipt_payload_hash,receipt_tool_call_id,receipt_sequence FROM errand_await_attempt WHERE token='attempt'").Scan(&firstHash, &firstCall, &firstSequence))
	require.Equal(t, event.ContentHash, firstHash)
	require.Equal(t, "native-call", firstCall)
	require.EqualValues(t, 1, firstSequence)
	inserted, err = r.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
	require.NoError(t, err)
	require.False(t, inserted)
	conflict := *event
	conflict.Payload = bytes.Replace(event.Payload, []byte("native-call"), []byte("foreign-call"), 1)
	_, err = r.ReceiveAgentDeliveryEvent(t.Context(), &conflict, 1)
	require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryEventConflict)
	event.Sequence = 2
	inserted, err = r.ReceiveAgentDeliveryEvent(t.Context(), event, 2)
	require.NoError(t, err)
	require.True(t, inserted)
	var retained int64
	require.NoError(t, r.db.Get(&retained, "SELECT receipt_sequence FROM errand_await_attempt WHERE token='attempt'"))
	require.EqualValues(t, 1, retained, "repeated native reports must retain first proof")
	_, err = r.AbortErrandAwait(t.Context(), id)
	require.Error(t, err)
	require.Equal(t, "received", receiptState(t, r, id.Token))
}

func TestErrandAwaitReceiptCannotCrossParentOrRevision(t *testing.T) {
	exerciseReceiptRejections(t, newRepoForSessionTests)
}
func TestPostgresErrandAwaitReceiptCannotCrossParentOrRevision(t *testing.T) {
	exerciseReceiptRejections(t, newPostgresErrandRepo)
}
func exerciseReceiptRejections(t *testing.T, create func(*testing.T) *Repository) {
	t.Helper()
	for _, scenario := range []string{"task", "session", "incarnation", "generation", "execution", "stream", "legacy", "token", "revision", "subset", "duplicate", "no-native-id", "lineage", "wrong-tool", "wrong-status", "tool-card", "assistant-text", "submission", "stored-task", "stored-session", "stored-incarnation", "stored-generation", "legacy-attempt"} {
		t.Run(scenario, func(t *testing.T) {
			r := create(t)
			_, id, event := receiptFixture(t, r)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(event.Payload, &payload))
			receipt := payload["errand_await_receipt"].(map[string]any)
			items := receipt["items"].([]any)
			admissionError := false
			switch scenario {
			case "stored-task":
				foreign := newErrandTask(t, r, "foreign")
				_, err := r.db.Exec(r.db.Rebind("UPDATE errand_await_attempt SET task_id=? WHERE token='attempt'"), foreign)
				require.NoError(t, err)
			case "stored-session", "stored-incarnation", "stored-generation", "legacy-attempt":
				statements := map[string]string{"stored-session": "parent_session_id='foreign'", "stored-incarnation": "parent_incarnation_id='foreign'", "stored-generation": "harness_generation=2", "legacy-attempt": "parent_incarnation_id='',harness_generation=0"}
				_, err := r.db.Exec("UPDATE errand_await_attempt SET " + statements[scenario] + " WHERE token='attempt'")
				require.NoError(t, err)
			case "task":
				event.TaskID = "foreign"
				admissionError = true
			case "session":
				event.SessionID = "foreign"
				admissionError = true
			case "incarnation":
				event.IncarnationID = "foreign"
				admissionError = true
			case "generation":
				event.HarnessGeneration = 2
				event.OwnerGeneration = 2
				admissionError = true
			case "execution":
				event.ExecutionID = "replaced"
				admissionError = true
			case "stream":
				event.ExpectedStreamID = "foreign"
				admissionError = true
			case "legacy":
				event.TaskID, event.ExecutionID, event.ExpectedStreamID, event.OwnerGeneration = "", "", "", 0
			case "token":
				receipt["token"] = "unknown"
			case "revision":
				items[0].(map[string]any)["result_revision"] = "foreign"
			case "subset":
				receipt["items"] = []any{}
			case "duplicate":
				receipt["items"] = append(items, items[0])
			case "no-native-id":
				delete(payload, "tool_call_id")
			case "lineage":
				payload["parent_tool_call_id"] = "child-call"
			case "wrong-tool":
				payload["tool_name"] = "external/await_errands_kandev"
			case "wrong-status":
				payload["tool_status"] = "error"
			case "tool-card":
				delete(payload, "errand_await_receipt")
			case "assistant-text":
				payload["type"] = "message_chunk"
				event.EventType = streams.EventTypeMessageChunk
			case "submission":
				event.SubmissionID = "foreign"
				admissionError = true
			}
			var err error
			event.Payload, err = json.Marshal(payload)
			require.NoError(t, err)
			_, err = r.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
			if admissionError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, "offered", receiptState(t, r, id.Token))
		})
	}
}

// @covers AC-TASKS-ERRANDS-004.4
// @covers AC-TASKS-ERRANDS-004.9
func TestErrandAwaitReceiptRetainsAbortedContradictionAndHistoricalProof(t *testing.T) {
	exerciseReceiptHistory(t, newRepoForSessionTests)
}
func TestPostgresErrandAwaitReceiptRetainsAbortedContradictionAndHistoricalProof(t *testing.T) {
	exerciseReceiptHistory(t, newPostgresErrandRepo)
}
func exerciseReceiptHistory(t *testing.T, create func(*testing.T) *Repository) {
	t.Helper()
	for _, aborted := range []bool{false, true} {
		t.Run(map[bool]string{false: "successor", true: "aborted"}[aborted], func(t *testing.T) {
			r := create(t)
			row, id, event := receiptFixture(t, r)
			if aborted {
				_, err := r.AbortErrandAwait(t.Context(), id)
				require.NoError(t, err)
			}
			current, err := r.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			current.ResultText = "successor result"
			current.ResultState = models.ErrandResultPending
			current.ResultRevision = ""
			current.DeliveryToken = ""
			require.NoError(t, r.UpdateErrand(t.Context(), current))
			successor, err := r.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			require.NotEqual(t, row.ResultRevision, successor.ResultRevision)
			admission := parentAdmission(successor, "new")
			_, err = r.ReserveErrandParentAdmission(t.Context(), admission)
			require.NoError(t, err)
			require.NoError(t, r.DecideErrandParentAdmission(t.Context(), admission, models.ErrandAdmissionAccepted))
			latest, err := r.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			latest.AwaitingResult = false
			latest.ResultText, latest.ResultTurnID, latest.LatestSettledTurnID = "new settled result", "turn-new", "turn-new"
			latest.ResultRevision = ""
			latest.ResultState = models.ErrandResultPending
			require.NoError(t, r.UpdateErrand(t.Context(), latest))
			inventory := claimInventory(t, r, row.ParentSessionID)
			_, err = r.ClaimResults(t.Context(), row.TaskID, row.ParentSessionID, models.ErrandDeliveryTurnContext, "successor-token", inventory, errandScenarioBase, inventory)
			require.NoError(t, err)
			before, err := r.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			require.Equal(t, "successor-token", before.DeliveryToken)
			require.EqualValues(t, 1, before.ParentAdmissionGeneration)
			_, err = r.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
			require.NoError(t, err)
			after, err := r.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			require.Equal(t, before, after, "old proof cannot alter successor result/token/generation")
			want := "received"
			if aborted {
				want = "aborted"
			}
			require.Equal(t, want, receiptState(t, r, id.Token))
			var retained int64
			require.NoError(t, r.db.Get(&retained, "SELECT receipt_sequence FROM errand_await_attempt WHERE token='attempt'"))
			require.EqualValues(t, 1, retained, "compact proof survives even a terminal contradiction")
		})
	}
}

func TestErrandAwaitReceiptRequiresCompleteSetAndAcceptsZeroItems(t *testing.T) {
	exerciseReceiptSets(t, newRepoForSessionTests)
}
func TestPostgresErrandAwaitReceiptRequiresCompleteSetAndAcceptsZeroItems(t *testing.T) {
	exerciseReceiptSets(t, newPostgresErrandRepo)
}
func exerciseReceiptSets(t *testing.T, create func(*testing.T) *Repository) {
	t.Helper()
	for _, count := range []int{0, 2} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			r := create(t)
			row, id, event := receiptFixtureItems(t, r, count)
			before, err := r.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			if count == 2 {
				var decoded map[string]any
				require.NoError(t, json.Unmarshal(event.Payload, &decoded))
				metadata := decoded["errand_await_receipt"].(map[string]any)
				items := metadata["items"].([]any)
				metadata["items"] = items[:1]
				partial := *event
				partial.Payload, err = json.Marshal(decoded)
				require.NoError(t, err)
				_, err = r.ReceiveAgentDeliveryEvent(t.Context(), &partial, 1)
				require.NoError(t, err)
				require.Equal(t, "offered", receiptState(t, r, id.Token))
				metadata["items"] = []any{items[1], items[0]}
				event.Sequence = 2
				event.Payload, err = json.Marshal(decoded)
				require.NoError(t, err)
			}
			_, err = r.ReceiveAgentDeliveryEvent(t.Context(), event, event.Sequence)
			require.NoError(t, err)
			require.Equal(t, "received", receiptState(t, r, id.Token))
			after, err := r.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			if count > 0 {
				before.DeliveryState = models.ErrandDeliveryReceived
			}
			require.Equal(t, before, after, "receipt changes no result rows, including zero-item waits")
		})
	}
}

func TestErrandAwaitMalformedReceiptIsRetainedAndDiagnosed(t *testing.T) {
	r := newRepoForSessionTests(t)
	_, id, event := receiptFixture(t, r)
	core, logs := observer.New(zap.WarnLevel)
	var err error
	r.log, err = logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	event.Payload = bytes.Replace(event.Payload, []byte(`"version":1`), []byte(`"version":"invalid"`), 1)
	inserted, err := r.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
	require.NoError(t, err)
	require.True(t, inserted)
	require.Equal(t, "offered", receiptState(t, r, id.Token))
	require.Equal(t, 1, logs.FilterMessage("errand await receipt ignored").Len(), "malformed positive observation must remain diagnosable")
}

func TestErrandAwaitNativeReceiptUsesAttemptOwnerAcrossLaterTurns(t *testing.T) {
	r := newRepoForSessionTests(t)
	row, id, event := receiptFixture(t, r)
	_, err := r.db.Exec("UPDATE errand_await_attempt SET submission_id='original',turn_id='original-turn',state='uncertain' WHERE token='attempt'")
	require.NoError(t, err)
	id.SubmissionID, id.TurnID = "original", "original-turn"
	_, err = r.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
	require.NoError(t, err)
	require.Equal(t, "received", receiptState(t, r, id.Token), "the native call/token and captured owner establish receipt independently of current turn association")
	_, err = r.AbortErrandAwait(t.Context(), id)
	require.Error(t, err)
	after, err := r.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	row.DeliveryState = models.ErrandDeliveryReceived
	require.Equal(t, row, after)
}
