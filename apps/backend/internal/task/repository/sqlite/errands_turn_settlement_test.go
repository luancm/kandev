package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

type exactErrandSettlementStore interface {
	ReserveErrandTurnSettlement(context.Context, string, *models.AgentDeliveryEffect) error
	CommitErrandTurnSettlement(context.Context, *models.Errand, *models.AgentDeliveryEffect) (bool, error)
	PendingErrandTurnSettlements(context.Context, string) ([]*models.AgentDeliveryEffect, error)
}

func exactSettlementStore(t *testing.T, repo *Repository) exactErrandSettlementStore {
	t.Helper()
	store, ok := any(repo).(exactErrandSettlementStore)
	require.True(t, ok, "repository must atomically commit an exact decision and fence successor reservations")
	return store
}

func seedExactSettlement(t *testing.T, repo *Repository) (*models.Errand, *models.AgentDeliveryEffect) {
	t.Helper()
	taskID, parentID, childID := newErrandIsolationTask(t, repo, "exact-settlement")
	row := sampleErrand(childID, taskID, parentID, errandScenarioBase)
	row.FirstTurnID, row.SubmissionID = "exact-A", "prompt:exact-A"
	row.Lifecycle, row.AwaitingResult = models.ErrandLifecycleLaunched, true
	mustCreateErrand(t, repo, row)
	require.NoError(t, repo.CreateTurn(t.Context(), &models.Turn{ID: row.FirstTurnID, TaskID: taskID,
		TaskSessionID: childID, StartedAt: errandScenarioBase}))
	session, err := repo.GetTaskSession(t.Context(), childID)
	require.NoError(t, err)
	return row, &models.AgentDeliveryEffect{EffectKey: "exact-settlement-A", EffectType: "errand.turn_settlement",
		SessionID: childID, IncarnationID: session.QueueIncarnationID, StreamID: "exact-execution",
		HarnessGeneration: 1, SubmissionID: row.SubmissionID, TurnID: row.FirstTurnID, Outcome: "parent", State: "pending"}
}

func TestErrandExactTerminalDecisionAtomic(t *testing.T) {
	exerciseExactTerminalDecision(t, newRepoForSessionTests(t))
}

func TestPostgresErrandExactTerminalDecisionAtomic(t *testing.T) {
	exerciseExactTerminalDecision(t, newPostgresErrandRepo(t))
}

func exerciseExactTerminalDecision(t *testing.T, repo *Repository) {
	t.Helper()
	store := exactSettlementStore(t, repo)
	row, effect := seedExactSettlement(t, repo)
	require.NoError(t, store.ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
	require.NoError(t, repo.CompleteTurn(t.Context(), effect.TurnID))
	row.LatestSettledTurnID, row.ResultTurnID, row.ResultText = effect.TurnID, effect.TurnID, "committed A"
	row.ResultState, row.LastOutcome, row.AwaitingResult = models.ErrandResultPending, models.ErrandOutcomeDone, false
	row.ResultKind = models.ErrandResultOutput
	at := time.Now().UTC()
	row.SettledAt = &at
	effect.State, effect.Outcome, effect.CompletedAt = "completed", "parent:done", &at
	removeFault := failErrandUpdate(t, repo, "NEW.latest_settled_turn_id = 'exact-A'")
	require.Error(t, commitExactForTest(store, t.Context(), row, effect))
	pending, err := store.PendingErrandTurnSettlements(t.Context(), row.ID)
	require.NoError(t, err)
	require.Len(t, pending, 1, "decision must roll back with failed row CAS")
	stored, err := repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Empty(t, stored.LatestSettledTurnID)
	removeFault()
	require.NoError(t, commitExactForTest(store, t.Context(), row, effect))
	require.NoError(t, commitExactForTest(store, t.Context(), row, effect), "exact decision replay is idempotent")
	pending, err = store.PendingErrandTurnSettlements(t.Context(), row.ID)
	require.NoError(t, err)
	require.Empty(t, pending)
}

func TestErrandExactTerminalDecisionGatesReservations(t *testing.T) {
	exerciseExactReservationGate(t, newRepoForSessionTests(t))
}

func TestPostgresErrandExactTerminalDecisionGatesReservations(t *testing.T) {
	exerciseExactReservationGate(t, newPostgresErrandRepo(t))
}

func exerciseExactReservationGate(t *testing.T, repo *Repository) {
	t.Helper()
	store := exactSettlementStore(t, repo)
	row, effect := seedExactSettlement(t, repo)
	require.NoError(t, store.ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
	require.NoError(t, repo.CompleteTurn(t.Context(), effect.TurnID))
	require.NoError(t, repo.UpdateTaskSessionState(t.Context(), row.ID, models.TaskSessionStateWaitingForInput, ""))
	queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.ro)
	require.NoError(t, err)
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	queue := messagequeue.NewService(queueRepo, 10, log)
	entry, err := queue.QueueMessage(t.Context(), row.ID, row.TaskID, "B", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	identity, err := queue.ResolveSessionIdentity(t.Context(), row.TaskID, row.ID)
	require.NoError(t, err)
	_, _, _, err = queue.ReserveQueuedWithAutoRunForSession(t.Context(), identity)
	require.Error(t, err, "FIFO cannot claim an undecided predecessor")
	_, err = queue.ClaimSendNowForSession(t.Context(), identity, []messagequeue.QueuedMessage{*entry})
	require.Error(t, err, "Send Now cannot claim an undecided predecessor")
	_, err = repo.ClaimPromptableTaskSessionIfActive(t.Context(), row.ID)
	require.Error(t, err, "direct promptability claim cannot bypass the decision")
	err = repo.CreateTurn(t.Context(), &models.Turn{ID: "exact-B", TaskID: row.TaskID, TaskSessionID: row.ID, StartedAt: time.Now()})
	require.Error(t, err, "turn creation cannot hide an undecided predecessor")
	status := queue.GetStatus(t.Context(), row.ID)
	require.Len(t, status.Entries, 1)
	require.Equal(t, entry.ID, status.Entries[0].ID)
}

func commitExactForTest(store exactErrandSettlementStore, ctx context.Context, row *models.Errand, effect *models.AgentDeliveryEffect) error {
	_, err := store.CommitErrandTurnSettlement(ctx, row, effect)
	return err
}
