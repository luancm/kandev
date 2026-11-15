package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type historicalReceiptStore interface {
	SealErrandTurnSettlement(context.Context, string, *models.AgentDeliveryEffect) error
	CommitHistoricalErrandTurnReceipt(context.Context, string, *models.AgentDeliveryEffect) error
}

func TestErrandHistoricalReceipt(t *testing.T) { exerciseHistoricalReceipt(t, newRepoForSessionTests) }
func TestPostgresErrandHistoricalReceipt(t *testing.T) {
	exerciseHistoricalReceipt(t, newPostgresErrandRepo)
}

func exerciseHistoricalReceipt(t *testing.T, create func(*testing.T) *Repository) {
	for _, mode := range []string{"open_turn", "completed", "newer_result", "parent_cancel", "user_cancel"} {
		t.Run(mode, func(t *testing.T) {
			repo := create(t)
			row, effect := seedExactSettlement(t, repo)
			require.NoError(t, repo.ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
			require.NoError(t, repo.CompleteTurn(t.Context(), effect.TurnID))
			at := time.Now().UTC()
			var completed *time.Time
			if mode != "open_turn" {
				completed = &at
			}
			_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_session_turns
    (id, task_id, task_session_id, started_at, completed_at, created_at, updated_at, metadata)
    VALUES ('historical-B', ?, ?, ?, ?, ?, ?, '{}')`), row.TaskID, row.ID, at, completed, at, at)
			require.NoError(t, err)
			if mode == "newer_result" {
				row.ResultTurnID, row.LatestSettledTurnID = "historical-B", "historical-B"
				row.ResultText, row.ResultKind, row.ResultState = "newer result", models.ErrandResultOutput, models.ErrandResultPending
				row.LastOutcome, row.AwaitingResult = models.ErrandOutcomeDone, false
				require.NoError(t, repo.UpdateErrand(t.Context(), row))
			}
			if mode == "parent_cancel" || mode == "user_cancel" {
				row.Lifecycle = models.ErrandLifecycleCancelled
				row.CancelSource = models.ErrandCancelSourceParent
				if mode == "user_cancel" {
					row.CancelSource = models.ErrandCancelSourceUser
				}
				row.ResultState = models.ErrandResultDiscarded
				require.NoError(t, repo.UpdateErrand(t.Context(), row))
			}
			before, err := repo.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			store, ok := any(repo).(historicalReceiptStore)
			require.True(t, ok)
			effect.Outcome = "parent:interrupted"
			require.NoError(t, store.SealErrandTurnSettlement(t.Context(), row.TaskID, effect))
			effect.State, effect.CompletedAt = models.DeliveryEffectCompleted, &at
			require.NoError(t, store.CommitHistoricalErrandTurnReceipt(t.Context(), row.TaskID, effect))
			require.NoError(t, store.CommitHistoricalErrandTurnReceipt(t.Context(), row.TaskID, effect))
			after, err := repo.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			require.Equal(t, before, after)
			receipt, err := repo.readErrandSettlementEffect(t.Context(), row.ID, effect.IncarnationID, effect.TurnID)
			require.NoError(t, err)
			require.Equal(t, models.DeliveryEffectCompleted, receipt.State)
			if completed == nil {
				require.NoError(t, repo.CompleteTurn(t.Context(), "historical-B"))
			}
			require.ErrorIs(t, repo.CreateTurn(t.Context(), &models.Turn{ID: "historical-C", TaskID: row.TaskID, TaskSessionID: row.ID, StartedAt: at.Add(time.Second)}), db.ErrErrandSettlementPending)
		})
	}
}

func TestErrandHistoricalReceiptRejectsInvalid(t *testing.T) {
	exerciseHistoricalInvalid(t, newRepoForSessionTests)
}
func TestPostgresErrandHistoricalReceiptRejectsInvalid(t *testing.T) {
	exerciseHistoricalInvalid(t, newPostgresErrandRepo)
}

func exerciseHistoricalInvalid(t *testing.T, create func(*testing.T) *Repository) {
	for _, mode := range []string{"unsealed", "open_turn", "task", "session", "incarnation", "generation", "submission", "origin", "outcome", "role"} {
		t.Run(mode, func(t *testing.T) {
			repo := create(t)
			row, effect := seedExactSettlement(t, repo)
			require.NoError(t, repo.ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
			store, ok := any(repo).(historicalReceiptStore)
			require.True(t, ok)
			if mode != "open_turn" {
				require.NoError(t, repo.CompleteTurn(t.Context(), effect.TurnID))
			}
			if mode != "unsealed" {
				effect.Outcome = "parent:interrupted"
				require.NoError(t, store.SealErrandTurnSettlement(t.Context(), row.TaskID, effect))
			}
			effect.Outcome = "parent:interrupted"
			at := time.Now().UTC()
			effect.State, effect.CompletedAt = models.DeliveryEffectCompleted, &at
			taskID := row.TaskID
			switch mode {
			case "task":
				taskID = "wrong-task"
			case "session":
				effect.SessionID = "wrong-session"
			case "incarnation":
				effect.IncarnationID = "wrong-incarnation"
			case "generation":
				effect.HarnessGeneration++
			case "submission":
				effect.SubmissionID = "wrong-submission"
			case "origin":
				effect.Outcome = "user:interrupted"
			case "outcome":
				effect.Outcome = "parent:done"
			case "role":
				_, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_sessions SET role = '' WHERE id = ?`), row.ID)
				require.NoError(t, err)
			}
			require.Error(t, store.CommitHistoricalErrandTurnReceipt(t.Context(), taskID, effect))
			var state string
			require.NoError(t, repo.db.QueryRow(`SELECT state FROM agent_delivery_effects WHERE effect_key = 'exact-settlement-A'`).Scan(&state))
			require.Equal(t, models.DeliveryEffectPending, state)
		})
	}
}

func TestErrandHistoricalReceiptRollbackAndSerialization(t *testing.T) {
	exerciseHistoricalTransaction(t, newRepoForSessionTests)
}
func TestPostgresErrandHistoricalReceiptRollbackAndSerialization(t *testing.T) {
	exerciseHistoricalTransaction(t, newPostgresErrandRepo)
}

func exerciseHistoricalTransaction(t *testing.T, create func(*testing.T) *Repository) {
	for _, rollback := range []bool{true, false} {
		name := "serialize"
		if rollback {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			repo := create(t)
			row, effect := seedExactSettlement(t, repo)
			require.NoError(t, repo.ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
			require.NoError(t, repo.CompleteTurn(t.Context(), effect.TurnID))
			effect.Outcome = "parent:interrupted"
			require.NoError(t, repo.SealErrandTurnSettlement(t.Context(), row.TaskID, effect))
			at := time.Now().UTC()
			effect.State, effect.CompletedAt = models.DeliveryEffectCompleted, &at
			gate := newErrandSQLGate("UPDATE agent_delivery_effects SET state", true)
			defer gate.unblock()
			writer := independentErrandRepository(t, repo, gate)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- writer.CommitHistoricalErrandTurnReceipt(ctx, row.TaskID, effect) }()
			awaitSignal(t, gate.entered, "historical receipt before commit")
			if rollback {
				cancel()
				require.Error(t, awaitError(t, done, "cancelled historical receipt"))
				stored, err := repo.readErrandSettlementEffect(t.Context(), row.ID, effect.IncarnationID, effect.TurnID)
				require.NoError(t, err)
				require.Equal(t, models.DeliveryEffectPending, stored.State)
				gate.unblock()
				require.NoError(t, repo.CommitHistoricalErrandTurnReceipt(t.Context(), row.TaskID, effect))
			} else {
				query := "UPDATE tasks SET id = id WHERE 0"
				if dialect.IsPostgres(repo.db.DriverName()) {
					query = "SELECT id FROM tasks"
				}
				reservationGate := newErrandSQLGate(query, false)
				reserver := independentErrandRepository(t, repo, reservationGate)
				reserved := make(chan error, 1)
				go func() {
					reserved <- reserver.CreateTurn(t.Context(), &models.Turn{ID: "serialized-history-B", TaskID: row.TaskID, TaskSessionID: row.ID, StartedAt: at})
				}()
				awaitSignal(t, reservationGate.entered, "successor waiting for historical receipt")
				gate.unblock()
				require.NoError(t, awaitError(t, done, "historical commit"))
				require.NoError(t, awaitError(t, reserved, "successor after receipt"))
			}
			after, err := repo.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			require.Equal(t, row, after)
		})
	}
}

func TestErrandHistoricalReceiptEnumeratesPastAmbiguity(t *testing.T) {
	exerciseHistoricalEnumeration(t, newRepoForSessionTests(t))
}
func TestPostgresErrandHistoricalReceiptEnumeratesPastAmbiguity(t *testing.T) {
	exerciseHistoricalEnumeration(t, newPostgresErrandRepo(t))
}
func exerciseHistoricalEnumeration(t *testing.T, repo *Repository) {
	row, effect := seedExactSettlement(t, repo)
	require.NoError(t, repo.ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
	require.NoError(t, repo.CompleteTurn(t.Context(), effect.TurnID))
	effect.Outcome = "parent:interrupted"
	require.NoError(t, repo.SealErrandTurnSettlement(t.Context(), row.TaskID, effect))
	at := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_session_turns
 (id, task_id, task_session_id, started_at, completed_at, created_at, updated_at, metadata)
 VALUES ('ambiguous-history-B', ?, ?, ?, ?, ?, ?, '{}')`), row.TaskID, row.ID, at, at, at, at)
	require.NoError(t, err)
	for _, key := range []string{"ambiguous-history-1", "ambiguous-history-2"} {
		_, err = repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_effects
  (effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, submission_id, turn_id, outcome, created_at)
  VALUES (?, '', 0, 'errand.turn_settlement', 'pending', ?, ?, 'unknown', 'ambiguous-history-B', 'user', ?)`), key, row.ID, effect.IncarnationID, at)
		require.NoError(t, err)
	}
	pending, err := repo.PendingErrandTurnSettlements(t.Context(), row.ID)
	require.ErrorIs(t, err, db.ErrErrandSettlementPending)
	require.Len(t, pending, 1)
	require.Equal(t, effect.EffectKey, pending[0].EffectKey)
}
