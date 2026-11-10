package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-002.8
func TestErrandReceiptReservation(t *testing.T) {
	exerciseErrandReceiptReservation(t, newRepoForSessionTests)
}
func TestPostgresErrandReceiptReservation(t *testing.T) {
	exerciseErrandReceiptReservation(t, newPostgresErrandRepo)
}

func exerciseErrandReceiptReservation(t *testing.T, create func(*testing.T) *Repository) {
	for _, mode := range []string{"receipt", "step_receipt", "unstamped"} {
		t.Run(mode, func(t *testing.T) {
			repo := create(t)
			row, effect := seedExactSettlement(t, repo)
			require.NoError(t, repo.ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
			require.NoError(t, repo.CompleteTurn(t.Context(), effect.TurnID))
			if mode == "unstamped" {
				_, err := repo.db.Exec(repo.db.Rebind(`UPDATE tasks SET workflow_step_id = '' WHERE id = ?`), row.TaskID)
				require.NoError(t, err)
			}
			revision, err := repo.ReadConversationRevision(t.Context(), row.ID)
			require.NoError(t, err)
			write := func() (*models.ConversationMutationReceipt, error) {
				turn := &models.Turn{ID: "receipt-successor", TaskID: row.TaskID, TaskSessionID: row.ID, StartedAt: time.Now().UTC()}
				if mode == "receipt" {
					return repo.CreateTurnWithConversationReceipt(t.Context(), turn)
				}
				_, receipt, err := repo.CreateTurnWithStepStampConversationReceipt(t.Context(), turn)
				return receipt, err
			}
			for range 2 {
				receipt, err := write()
				require.ErrorIs(t, err, db.ErrErrandSettlementPending)
				require.Nil(t, receipt)
				after, err := repo.ReadConversationRevision(t.Context(), row.ID)
				require.NoError(t, err)
				require.Equal(t, revision, after)
				var count int
				require.NoError(t, repo.db.QueryRow(`SELECT COUNT(*) FROM task_session_turns WHERE id = 'receipt-successor'`).Scan(&count))
				require.Zero(t, count)
			}
			effect.Outcome = "parent:interrupted"
			require.NoError(t, repo.SealErrandTurnSettlement(t.Context(), row.TaskID, effect))
			at := time.Now().UTC()
			effect.State, effect.CompletedAt = models.DeliveryEffectCompleted, &at
			require.NoError(t, repo.CommitHistoricalErrandTurnReceipt(t.Context(), row.TaskID, effect))
			receipt, err := write()
			require.NoError(t, err)
			require.NotNil(t, receipt)
		})
	}
}
