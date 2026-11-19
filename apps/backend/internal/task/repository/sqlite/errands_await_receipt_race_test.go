package sqlite

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

func TestErrandAwaitReceiptRollbackAndUnknownCommit(t *testing.T) {
	exerciseReceiptFaults(t, newRepoForSessionTests)
}
func TestPostgresErrandAwaitReceiptRollbackAndUnknownCommit(t *testing.T) {
	exerciseReceiptFaults(t, newPostgresErrandRepo)
}
func exerciseReceiptFaults(t *testing.T, create func(*testing.T) *Repository) {
	t.Helper()
	for _, fault := range []string{"receipt", "cursor", "commit-before", "commit-after"} {
		t.Run(fault, func(t *testing.T) {
			r := create(t)
			_, id, event := receiptFixture(t, r)
			writer := r
			cleanup := func() {}
			if fault == "receipt" || fault == "cursor" {
				cleanup = rejectAwaitWrite(t, r, fault)
			} else {
				gate := newErrandSQLGate("unused", false)
				gate.commitFault = func(tx driver.Tx) error {
					if fault == "commit-after" {
						require.NoError(t, tx.Commit())
					} else {
						require.NoError(t, tx.Rollback())
					}
					return errors.New("lost receipt commit acknowledgement")
				}
				writer = independentErrandRepository(t, r, gate)
			}
			_, err := writer.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
			require.Error(t, err, "injected failure must reach real receive transaction")
			cleanup()
			committed := fault == "commit-after"
			want := "offered"
			if committed {
				want = "received"
			}
			require.Equal(t, want, receiptState(t, r, id.Token))
			var count int
			require.NoError(t, r.db.Get(&count, "SELECT COUNT(*) FROM agent_delivery_inbox WHERE stream_id='stream'"))
			require.Equal(t, map[bool]int{false: 0, true: 1}[committed], count)
			require.NoError(t, r.db.Get(&count, "SELECT COUNT(*) FROM agent_delivery_cursors WHERE stream_id='stream'"))
			require.Equal(t, map[bool]int{false: 0, true: 1}[committed], count)
			reopened := independentErrandRepository(t, r, newErrandSQLGate("unused", false))
			inserted, err := reopened.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
			require.NoError(t, err)
			require.Equal(t, !committed, inserted, "same stream/sequence/hash retry reads back unknown commit")
			require.Equal(t, "received", receiptState(t, r, id.Token))
			cursor, err := r.GetAgentDeliveryCursor(t.Context(), event.StreamID)
			require.NoError(t, err)
			require.EqualValues(t, 1, cursor.ReceivedSequence)
		})
	}
}

func rejectAwaitWrite(t *testing.T, r *Repository, location string) func() {
	t.Helper()
	table, operation := "errand_await_attempt", "UPDATE"
	if location == "cursor" {
		table, operation = "agent_delivery_cursors", "INSERT"
	}
	var create, remove string
	if dialect.IsPostgres(r.db.DriverName()) {
		create = fmt.Sprintf(`CREATE FUNCTION fail_await_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'await fault'; END $$; CREATE TRIGGER fail_await BEFORE %s ON %s FOR EACH ROW EXECUTE FUNCTION fail_await_write()`, operation, table)
		remove = fmt.Sprintf(`DROP TRIGGER fail_await ON %s; DROP FUNCTION fail_await_write()`, table)
	} else {
		create = fmt.Sprintf(`CREATE TRIGGER fail_await BEFORE %s ON %s BEGIN SELECT RAISE(ABORT,'await fault'); END`, operation, table)
		remove = `DROP TRIGGER fail_await`
	}
	_, err := r.db.Exec(create)
	require.NoError(t, err)
	return func() { _, err := r.db.Exec(remove); require.NoError(t, err) }
}

func TestErrandAwaitReceiptGuardSerializesOfferAbortAdmissionAndReplacement(t *testing.T) {
	exerciseReceiptRaces(t, newRepoForSessionTests)
}
func TestPostgresErrandAwaitReceiptGuardSerializesOfferAbortAdmissionAndReplacement(t *testing.T) {
	exerciseReceiptRaces(t, newPostgresErrandRepo)
}
func exerciseReceiptRaces(t *testing.T, create func(*testing.T) *Repository) {
	t.Helper()
	for _, first := range []string{"receipt", "abort", "offer", "admission", "replacement"} {
		t.Run(first, func(t *testing.T) {
			r := create(t)
			row, id, event := receiptFixture(t, r)
			prefix := "UPDATE errand_await_attempt SET state"
			if first == "admission" {
				prefix = "UPDATE task_session_errands SET parent_admission_generation"
			}
			if first == "replacement" {
				prefix = "UPDATE task_session_errands SET"
			}
			if first == "offer" {
				_, err := r.db.Exec("UPDATE errand_await_attempt SET state='claimed' WHERE token='attempt'")
				require.NoError(t, err)
			}
			admission := parentAdmission(row, "race")
			if first == "admission" {
				_, err := r.ReserveErrandParentAdmission(t.Context(), admission)
				require.NoError(t, err)
			}
			gate := newErrandSQLGate(prefix, true)
			defer gate.unblock()
			leader := independentErrandRepository(t, r, gate)
			result := make(chan error, 1)
			go func() {
				var err error
				switch first {
				case "receipt":
					_, err = leader.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
				case "abort":
					_, err = leader.AbortErrandAwait(t.Context(), id)
				case "offer":
					err = leader.OfferErrandAwait(t.Context(), id, []models.ErrandAwaitDeliveryItem{{ErrandID: row.ID, ResultRevision: row.ResultRevision}})
				case "admission":
					err = leader.DecideErrandParentAdmission(t.Context(), admission, models.ErrandAdmissionAccepted)
				case "replacement":
					changed := *row
					changed.ResultText = "new result"
					changed.ResultRevision = ""
					changed.ResultState = models.ErrandResultPending
					changed.DeliveryToken = ""
					err = leader.UpdateErrand(t.Context(), &changed)
				}
				result <- err
			}()
			awaitSignal(t, gate.entered, "first operation after task guard")
			guard := "UPDATE tasks SET id = id WHERE 0"
			if dialect.IsPostgres(r.db.DriverName()) {
				guard = "SELECT pg_advisory_xact_lock"
			}
			competingGate := newErrandSQLGate(guard, false)
			follower := independentErrandRepository(t, r, competingGate)
			competing := make(chan error, 1)
			go func() {
				var err error
				if first == "receipt" {
					_, err = follower.AbortErrandAwait(t.Context(), id)
				} else {
					_, err = follower.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
				}
				competing <- err
			}()
			awaitSignal(t, competingGate.entered, "competing operation attempts same task guard")
			select {
			case err := <-competing:
				t.Fatalf("crossed task guard: %v", err)
			default:
			}
			gate.unblock()
			require.NoError(t, awaitError(t, result, "first commit"))
			err := awaitError(t, competing, "competing decision")
			if first == "receipt" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			want := "received"
			if first == "abort" {
				want = "aborted"
			}
			require.Equal(t, want, receiptState(t, r, id.Token))
			current, err := r.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			switch first {
			case "abort":
				require.Equal(t, models.ErrandResultPending, current.ResultState)
			case "admission":
				require.EqualValues(t, 1, current.ParentAdmissionGeneration)
				require.Equal(t, models.ErrandResultDelivered, current.ResultState)
			case "replacement":
				require.Equal(t, "new result", current.ResultText)
				require.NotEqual(t, row.ResultRevision, current.ResultRevision)
				require.Empty(t, current.DeliveryToken)
			default:
				require.Equal(t, models.ErrandResultDelivered, current.ResultState)
			}
		})
	}
}

// A receipt blocked on the task guard must hold no session barrier. A new
// generation can commit first, and admission must then reject the old owner.
func TestPostgresErrandAwaitReceiptTakesTaskGuardBeforeSessionReplacement(t *testing.T) {
	r := newPostgresErrandRepo(t)
	row, id, event := receiptFixture(t, r)
	offerGate := newErrandSQLGate("UPDATE errand_await_attempt SET state='offered'", true)
	defer offerGate.unblock()
	offerer := independentErrandRepository(t, r, offerGate)
	offered := make(chan error, 1)
	go func() {
		offered <- offerer.OfferErrandAwait(t.Context(), id, []models.ErrandAwaitDeliveryItem{{ErrandID: row.ID, ResultRevision: row.ResultRevision}})
	}()
	awaitSignal(t, offerGate.entered, "offer holds task guard")
	receiptGate := newErrandSQLGate("SELECT pg_advisory_xact_lock", false)
	receiver := independentErrandRepository(t, r, receiptGate)
	received := make(chan error, 1)
	go func() { _, err := receiver.ReceiveAgentDeliveryEvent(t.Context(), event, 1); received <- err }()
	awaitSignal(t, receiptGate.entered, "receipt attempts task guard before taking session locks")
	generationGate := newErrandSQLGate("INSERT INTO harness_session_generations", true)
	defer generationGate.unblock()
	replacement := independentErrandRepository(t, r, generationGate)
	replaced := make(chan error, 1)
	go func() {
		ok, err := replacement.CommitHarnessSessionGeneration(t.Context(), &models.HarnessSessionGeneration{SessionID: id.ParentSessionID, IncarnationID: id.ParentIncarnationID, Generation: 2, PredecessorGeneration: 1, NativeSessionID: "replacement-native", CreationReason: "reset"}, 1)
		if err == nil && !ok {
			err = errors.New("generation replacement not committed")
		}
		replaced <- err
	}()
	awaitSignal(t, generationGate.entered, "replacement acquired session locks while receipt awaits task guard")
	generationGate.unblock()
	require.NoError(t, awaitError(t, replaced, "replacement generation commit"))
	offerGate.unblock()
	require.NoError(t, awaitError(t, offered, "offer commit"))
	require.ErrorIs(t, awaitError(t, received, "stale receipt rejected after session lock"), repoerrors.ErrAgentDeliveryIdentityMismatch)
	require.Equal(t, "offered", receiptState(t, r, id.Token))
	var inbox int
	require.NoError(t, r.db.Get(&inbox, "SELECT COUNT(*) FROM agent_delivery_inbox"))
	require.Zero(t, inbox)
}
