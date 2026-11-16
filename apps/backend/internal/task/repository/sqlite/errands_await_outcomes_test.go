package sqlite

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

type awaitOutcomeStore interface {
	BeginErrandAwait(context.Context, models.ErrandAwaitIdentity) error
	OfferErrandAwait(context.Context, models.ErrandAwaitIdentity, []models.ErrandAwaitDeliveryItem) error
	AbortErrandAwait(context.Context, models.ErrandAwaitIdentity) ([]*models.Errand, error)
}

func TestErrandAwaitAttemptOwnerAndOffer(t *testing.T) {
	exerciseAwaitOwner(t, newRepoForSessionTests(t))
}
func TestPostgresErrandAwaitAttemptOwnerAndOffer(t *testing.T) {
	exerciseAwaitOwner(t, newPostgresErrandRepo(t))
}
func exerciseAwaitOwner(t *testing.T, r *Repository) {
	t.Helper()
	ctx := context.Background()
	store, ok := any(r).(awaitOutcomeStore)
	require.True(t, ok, "repository must bind and fence trusted await attempts")
	row := newAdmissionResult(t, r, "await-offer")
	id := models.ErrandAwaitIdentity{TaskID: row.TaskID, ParentSessionID: row.ParentSessionID, Token: "attempt", ParentIncarnationID: "launched", HarnessGeneration: 2, TurnID: "original", SubmissionID: "submission"}
	require.NoError(t, store.BeginErrandAwait(ctx, id))
	for _, field := range []string{"task", "parent", "incarnation", "generation", "turn", "submission"} {
		bad := id
		switch field {
		case "task":
			bad.TaskID = "foreign"
		case "parent":
			bad.ParentSessionID = "foreign"
		case "incarnation":
			bad.ParentIncarnationID = "foreign"
		case "generation":
			bad.HarnessGeneration++
		case "turn":
			bad.TurnID = "foreign"
		case "submission":
			bad.SubmissionID = "foreign"
		}
		require.Error(t, store.BeginErrandAwait(ctx, bad), field)
		require.Error(t, store.OfferErrandAwait(ctx, bad, nil), field)
		_, err := store.AbortErrandAwait(ctx, bad)
		require.Error(t, err, field)
	}
	inventory := claimInventory(t, r, row.ParentSessionID)
	claimed, err := r.ClaimResults(ctx, row.TaskID, row.ParentSessionID, models.ErrandDeliveryAwait, id.Token, inventory, errandScenarioBase, inventory)
	require.NoError(t, err)
	items := []models.ErrandAwaitDeliveryItem{{ErrandID: row.ID, ResultRevision: claimed[0].ResultRevision}}
	require.Error(t, store.OfferErrandAwait(ctx, id, nil), "partial identity must fail")
	require.Error(t, store.OfferErrandAwait(ctx, id, append(items, items[0])), "duplicate must fail")
	require.NoError(t, store.OfferErrandAwait(ctx, id, items))
	var state string
	require.NoError(t, r.db.Get(&state, "SELECT state FROM errand_await_attempt WHERE token = 'attempt'"))
	require.Equal(t, "offered", state)
	released, err := store.AbortErrandAwait(ctx, id)
	require.NoError(t, err)
	require.Len(t, released, 1)
	require.Error(t, store.OfferErrandAwait(ctx, id, items), "late success is fenced")
}

func TestErrandAwaitOfferCommitAmbiguitySelectsError(t *testing.T) {
	exerciseAwaitOfferCommit(t, newRepoForSessionTests)
}
func TestPostgresErrandAwaitOfferCommitAmbiguitySelectsError(t *testing.T) {
	exerciseAwaitOfferCommit(t, newPostgresErrandRepo)
}
func exerciseAwaitOfferCommit(t *testing.T, create func(*testing.T) *Repository) {
	t.Helper()
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprint(committed), func(t *testing.T) {
			r := create(t)
			ctx := t.Context()
			row := newAdmissionResult(t, r, "offer-commit")
			id := models.ErrandAwaitIdentity{TaskID: row.TaskID, ParentSessionID: row.ParentSessionID, Token: "ambiguous", ParentIncarnationID: "owner", HarnessGeneration: 1}
			require.NoError(t, r.BeginErrandAwait(ctx, id))
			inventory := claimInventory(t, r, row.ParentSessionID)
			claims, err := r.ClaimResults(ctx, row.TaskID, row.ParentSessionID, models.ErrandDeliveryAwait, id.Token, inventory, errandScenarioBase, inventory)
			require.NoError(t, err)
			items := []models.ErrandAwaitDeliveryItem{{ErrandID: row.ID, ResultRevision: claims[0].ResultRevision}}
			gate := newErrandSQLGate("unused", false)
			gate.commitFault = func(tx driver.Tx) error {
				if committed {
					require.NoError(t, tx.Commit())
				} else {
					require.NoError(t, tx.Rollback())
				}
				return errors.New("lost offer commit acknowledgement")
			}
			writer := independentErrandRepository(t, r, gate)
			require.Error(t, writer.OfferErrandAwait(ctx, id, items), "uncertain offer must select error even after a committed write")
			_, err = r.AbortErrandAwait(ctx, id)
			require.NoError(t, err)
			current, err := r.GetErrand(ctx, row.ID)
			require.NoError(t, err)
			require.Equal(t, models.ErrandResultPending, current.ResultState)
		})
	}
}
func TestAwaitReceivedCannotBeReleasedByLateOwnedAbort(t *testing.T) {
	exerciseReceivedOwnedAbort(t, newRepoForSessionTests(t))
}
func TestPostgresAwaitReceivedCannotBeReleasedByLateOwnedAbort(t *testing.T) {
	exerciseReceivedOwnedAbort(t, newPostgresErrandRepo(t))
}
func exerciseReceivedOwnedAbort(t *testing.T, r *Repository) {
	t.Helper()
	ctx := t.Context()
	row := newAdmissionResult(t, r, "terminal-receipt")
	id := models.ErrandAwaitIdentity{TaskID: row.TaskID, ParentSessionID: row.ParentSessionID, Token: "terminal", ParentIncarnationID: "owner", HarnessGeneration: 1}
	require.NoError(t, r.BeginErrandAwait(ctx, id))
	inventory := claimInventory(t, r, row.ParentSessionID)
	_, err := r.ClaimResults(ctx, row.TaskID, row.ParentSessionID, models.ErrandDeliveryAwait, id.Token, inventory, errandScenarioBase, inventory)
	require.NoError(t, err)
	// 04B owns positive ingestion; this fixture exercises the existing terminal guard.
	_, err = r.db.Exec("UPDATE errand_await_attempt SET state='received' WHERE token='terminal'")
	require.NoError(t, err)
	changed, err := r.AbortErrandAwait(ctx, id)
	require.Error(t, err)
	require.Empty(t, changed)
	current, err := r.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultDelivered, current.ResultState)
}
func TestErrandAwaitAbortBeforeOwnedClaim(t *testing.T) {
	exerciseOwnedAbortBeforeClaim(t, newRepoForSessionTests(t))
}
func TestPostgresErrandAwaitAbortBeforeOwnedClaim(t *testing.T) {
	exerciseOwnedAbortBeforeClaim(t, newPostgresErrandRepo(t))
}
func exerciseOwnedAbortBeforeClaim(t *testing.T, r *Repository) {
	t.Helper()
	ctx := t.Context()
	row := newAdmissionResult(t, r, "abort-before-owned")
	id := models.ErrandAwaitIdentity{TaskID: row.TaskID, ParentSessionID: row.ParentSessionID, Token: "early", ParentIncarnationID: "owner", HarnessGeneration: 1}
	changed, err := r.AbortErrandAwait(ctx, id)
	require.NoError(t, err)
	require.Empty(t, changed)
	require.Error(t, r.BeginErrandAwait(ctx, id))
	inventory := claimInventory(t, r, row.ParentSessionID)
	_, err = r.ClaimResults(ctx, row.TaskID, row.ParentSessionID, models.ErrandDeliveryAwait, id.Token, inventory, errandScenarioBase, inventory)
	require.Error(t, err)
	require.NoError(t, r.db.Get(new(string), "SELECT state FROM errand_await_attempt WHERE token='early'"))
}

func TestErrandAwaitOfferAndAbortShareGuard(t *testing.T) {
	exerciseAwaitOfferAbortGuard(t, newRepoForSessionTests(t))
}
func TestPostgresErrandAwaitOfferAndAbortShareGuard(t *testing.T) {
	exerciseAwaitOfferAbortGuard(t, newPostgresErrandRepo(t))
}
func exerciseAwaitOfferAbortGuard(t *testing.T, repo *Repository) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	row := newAdmissionResult(t, repo, "offer-abort-guard")
	id := models.ErrandAwaitIdentity{TaskID: row.TaskID, ParentSessionID: row.ParentSessionID, Token: "guarded", ParentIncarnationID: "owner", HarnessGeneration: 1}
	require.NoError(t, repo.BeginErrandAwait(ctx, id))
	inventory := claimInventory(t, repo, row.ParentSessionID)
	snapshots, err := repo.ClaimResults(ctx, row.TaskID, row.ParentSessionID, models.ErrandDeliveryAwait, id.Token, inventory, errandScenarioBase, inventory)
	require.NoError(t, err)
	items := []models.ErrandAwaitDeliveryItem{{ErrandID: row.ID, ResultRevision: snapshots[0].ResultRevision}}
	gate := newErrandSQLGate("UPDATE errand_await_attempt SET state='offered'", true)
	defer gate.unblock()
	offerer := independentErrandRepository(t, repo, gate)
	offered := make(chan error, 1)
	go func() { offered <- offerer.OfferErrandAwait(ctx, id, items) }()
	awaitSignal(t, gate.entered, "offered update after locked item validation")
	query := "UPDATE tasks SET id = id WHERE 0"
	if dialect.IsPostgres(repo.db.DriverName()) {
		query = "SELECT pg_advisory_xact_lock"
	}
	abortGate := newErrandSQLGate(query, false)
	aborter := independentErrandRepository(t, repo, abortGate)
	aborted := make(chan error, 1)
	go func() { _, err := aborter.AbortErrandAwait(ctx, id); aborted <- err }()
	awaitSignal(t, abortGate.entered, "abort attempting same task guard")
	select {
	case err := <-aborted:
		t.Fatalf("abort crossed offered transaction: %v", err)
	default:
	}
	gate.unblock()
	require.NoError(t, awaitError(t, offered, "offered commit"))
	require.NoError(t, awaitError(t, aborted, "abort after offer"))
	current, err := repo.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultPending, current.ResultState)
	require.Error(t, repo.OfferErrandAwait(ctx, id, items))
}
