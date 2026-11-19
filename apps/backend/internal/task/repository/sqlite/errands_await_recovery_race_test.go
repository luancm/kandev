package sqlite

import (
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestErrandAwaitClassificationAndPositiveSerialize(t *testing.T) {
	exerciseAwaitClassificationRace(t, newRepoForSessionTests)
}

func TestPostgresErrandAwaitClassificationAndPositiveSerialize(t *testing.T) {
	exerciseAwaitClassificationRace(t, newPostgresErrandRepo)
}

func exerciseAwaitClassificationRace(t *testing.T, create func(*testing.T) *Repository) {
	t.Helper()
	for _, terminal := range []bool{false, true} {
		for _, positiveFirst := range []bool{false, true} {
			name := "recovery"
			if terminal {
				name = "terminal"
			}
			if positiveFirst {
				name += "/positive-first"
			} else {
				name += "/classification-first"
			}
			t.Run(name, func(t *testing.T) {
				r := create(t)
				row, id, receipt := receiptFixture(t, r)
				var end *models.AgentDeliveryEvent
				if terminal {
					id.TurnID = "origin-turn"
					end = awaitTerminalFixture(t, r, id, receipt, id.TurnID, "")
					_, err := r.ReceiveAgentDeliveryEvent(t.Context(), end, 1)
					require.NoError(t, err)
					_, err = r.ProjectAgentDeliveryEvent(t.Context(), end, nil)
					require.NoError(t, err)
					receipt.Sequence = 2
				}
				gate := newErrandSQLGate("UPDATE errand_await_attempt SET state", true)
				defer gate.unblock()
				leader := independentErrandRepository(t, r, gate)
				result := make(chan error, 1)
				decide := func(repo *Repository, positive bool) error {
					if positive {
						_, err := repo.ReceiveAgentDeliveryEvent(t.Context(), receipt, receipt.Sequence)
						return err
					}
					if terminal {
						return repo.ClassifyErrandAwaitTerminal(t.Context(), end)
					}
					_, err := repo.RecoverErrandAwait(t.Context(), id)
					return err
				}
				go func() { result <- decide(leader, positiveFirst) }()
				awaitSignal(t, gate.entered, "classification/receipt inside task guard")
				guard := "UPDATE tasks SET id = id WHERE 0"
				if dialect.IsPostgres(r.db.DriverName()) {
					guard = "SELECT pg_advisory_xact_lock"
				}
				followerGate := newErrandSQLGate(guard, false)
				follower := independentErrandRepository(t, r, followerGate)
				competing := make(chan error, 1)
				go func() { competing <- decide(follower, !positiveFirst) }()
				awaitSignal(t, followerGate.entered, "second connection attempts task guard")
				select {
				case err := <-competing:
					t.Fatalf("classification crossed task guard: %v", err)
				default:
				}
				gate.unblock()
				require.NoError(t, awaitError(t, result, "first commit"))
				require.NoError(t, awaitError(t, competing, "competing commit"))
				after := assertAwaitProjection(t, r, row.ID, "received")
				require.Equal(t, row.DeliveryToken, after.DeliveryToken)
				require.Equal(t, row.Version, after.Version)
			})
		}
	}
}

func TestErrandAwaitRecoveryCommitFailureRetainsClaim(t *testing.T) {
	exerciseAwaitRecoveryFault(t, newRepoForSessionTests)
}

func TestPostgresErrandAwaitRecoveryCommitFailureRetainsClaim(t *testing.T) {
	exerciseAwaitRecoveryFault(t, newPostgresErrandRepo)
}

func exerciseAwaitRecoveryFault(t *testing.T, create func(*testing.T) *Repository) {
	t.Helper()
	for _, committed := range []bool{false, true} {
		r := create(t)
		row, id, _ := receiptFixture(t, r)
		gate := newErrandSQLGate("unused", false)
		gate.commitFault = func(tx driver.Tx) error {
			if committed {
				require.NoError(t, tx.Commit())
			} else {
				require.NoError(t, tx.Rollback())
			}
			return errors.New("lost classification acknowledgement")
		}
		writer := independentErrandRepository(t, r, gate)
		changed, err := writer.RecoverErrandAwait(t.Context(), id)
		require.Error(t, err)
		require.False(t, changed)
		want := ""
		if committed {
			want = "uncertain"
		}
		after := assertAwaitProjection(t, r, row.ID, want)
		require.Equal(t, row.DeliveryToken, after.DeliveryToken)
		require.Equal(t, models.ErrandResultDelivered, after.ResultState)
		reopened := independentErrandRepository(t, r, newErrandSQLGate("unused", false))
		_, err = reopened.RecoverErrandAwait(t.Context(), id)
		require.NoError(t, err)
		assertAwaitProjection(t, r, row.ID, "uncertain")
	}
}
