package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-ERRANDS-003.4 AC-TASKS-ERRANDS-012.3
func TestErrandReadyStaleTerminalPreservesSuccessorIdentity(t *testing.T) {
	f := newQueueSettlementFixture(t, false)
	f.ready(t)
	f.awaitDispatch(t)
	before, err := f.repo.GetActiveTurnBySessionID(t.Context(), f.child.ID)
	require.NoError(t, err)
	require.NotNil(t, before)
	require.NotEqual(t, "settlement-A", before.ID)
	pending, err := f.svc.messageQueue.QueueMessage(t.Context(), f.child.ID, f.taskID, "pending C", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	readSubmission := func() string {
		var id string
		require.NoError(t, f.repo.DB().QueryRow(`SELECT submission_id FROM agent_delivery_effects WHERE effect_type = 'errand.turn_settlement' AND turn_id = ?`, before.ID).Scan(&id))
		return id
	}
	submissionID := readSubmission()
	reservation := f.svc.acceptedQueuedDispatchForSession(f.child.ID)
	f.ready(t)
	assertSuccessor := func() {
		after, err := f.repo.GetActiveTurnBySessionID(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, before.ID, after.ID)
		require.Nil(t, after.CompletedAt)
		session, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
		require.NoError(t, err)
		require.Equal(t, models.TaskSessionStateRunning, session.State)
		require.Equal(t, submissionID, readSubmission())
		require.Same(t, reservation, f.svc.acceptedQueuedDispatchForSession(f.child.ID))
		status := f.svc.messageQueue.GetStatus(t.Context(), f.child.ID)
		require.Len(t, status.Entries, 1)
		require.Equal(t, pending.ID, status.Entries[0].ID)
	}
	assertSuccessor()
	f.svc.handleAgentCompleted(t.Context(), watcher.AgentEventData{TaskID: f.taskID, SessionID: f.child.ID,
		AgentExecutionID: "settlement-execution", TurnID: "settlement-A"})
	require.False(t, f.svc.isExecutionCompleted(f.child.ID, "settlement-execution"), "old A must not retire B's execution")
	assertSuccessor()
}

func TestErrandAgentCompletedCapturesResultBeforeQueuedSuccessor(t *testing.T) {
	for _, turnID := range []string{"settlement-A", ""} {
		t.Run("observed_turn="+turnID, func(t *testing.T) {
			f := newQueueSettlementFixture(t, false)
			f.svc.handleAgentCompleted(t.Context(), watcher.AgentEventData{TaskID: f.taskID, SessionID: f.child.ID,
				AgentExecutionID: "settlement-execution", TurnID: turnID})
			row, err := f.repo.GetErrand(t.Context(), f.child.ID)
			require.NoError(t, err)
			require.Equal(t, "canonical result A", row.ResultText)
			f.awaitDispatch(t)
			require.Zero(t, f.taskRepo.stateWrites[f.taskID])
		})
	}
}

func TestErrandReadyCancelledChildKeepsNoticeAndPausedQueue(t *testing.T) {
	for _, source := range []models.ErrandCancelSource{models.ErrandCancelSourceUser, models.ErrandCancelSourceParent, models.ErrandCancelSourceCascade} {
		t.Run(string(source), func(t *testing.T) {
			f := newQueueSettlementFixture(t, false)
			row, err := f.repo.GetErrand(t.Context(), f.child.ID)
			require.NoError(t, err)
			row.Lifecycle, row.CancelSource, row.AwaitingResult = models.ErrandLifecycleCancelled, source, false
			row.ResultState = models.ErrandResultDiscarded
			if source == models.ErrandCancelSourceUser {
				row.ResultText, row.ResultKind, row.ResultState = "The cancellation notice.", models.ErrandResultCancelled, models.ErrandResultPending
			}
			require.NoError(t, f.repo.UpdateErrand(t.Context(), row))
			require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), f.child.ID, false))
			f.ready(t)
			after, err := f.repo.GetErrand(t.Context(), f.child.ID)
			require.NoError(t, err)
			require.Equal(t, row, after, "late exact completion must preserve the committed cancellation")
			status := f.svc.messageQueue.GetStatus(t.Context(), f.child.ID)
			require.False(t, status.AutoRun)
			require.Len(t, status.Entries, 1)
			require.Equal(t, f.entry.ID, status.Entries[0].ID)
			var outcome string
			require.NoError(t, f.repo.DB().QueryRow(`SELECT outcome FROM agent_delivery_effects WHERE effect_type = 'errand.turn_settlement' AND turn_id = 'settlement-A'`).Scan(&outcome))
			require.Equal(t, "parent:cancelled", outcome)
			require.Zero(t, f.taskRepo.stateWrites[f.taskID])
		})
	}
}

func TestErrandSettlementFailureRecoversProjectedTerminalBeforeQueue(t *testing.T) {
	f := newQueueSettlementFixture(t, false)
	row, err := f.repo.GetErrand(t.Context(), f.child.ID)
	require.NoError(t, err)
	_, err = f.repo.DB().Exec(`INSERT INTO agent_delivery_effects
		(effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome, created_at)
		VALUES ('projected-before-capture', 'retained-stream', 7, ?, ?, ?, ?, 1, ?, ?, ?, ?)`, models.DeliveryTerminalEffectType,
		models.DeliveryEffectBlockResolved, row.ID, f.child.QueueIncarnationID, row.SubmissionID, row.FirstTurnID,
		models.DeliverySubmissionCompleted, row.CreatedAt)
	require.NoError(t, err)
	_, err = f.repo.DB().Exec(`CREATE TRIGGER fail_terminal_intent BEFORE INSERT ON agent_delivery_effects
		WHEN NEW.effect_type = 'errand.turn_settlement' BEGIN SELECT RAISE(ABORT, 'intent write fault'); END`)
	require.NoError(t, err)
	f.ready(t)
	status := f.svc.messageQueue.GetStatus(t.Context(), row.ID)
	require.Len(t, status.Entries, 1)
	require.Equal(t, f.entry.ID, status.Entries[0].ID)
	_, err = f.repo.DB().Exec(`DROP TRIGGER fail_terminal_intent`)
	require.NoError(t, err)
	f.errands.StopWorkers()
	f.errands, _ = newErrandServiceOnRealStack(t, f.errandLaunchFixture)
	f.errands.SetTurnReader(settlementTranscript{f.repo})
	dispatched, err := f.svc.DrainQueuedMessage(t.Context(), row.ID)
	require.NoError(t, err)
	require.True(t, dispatched)
	f.awaitDispatch(t)
	after, err := f.repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, "canonical result A", after.ResultText)
	require.Equal(t, row.FirstTurnID, after.LatestSettledTurnID)
	require.Zero(t, f.taskRepo.stateWrites[f.taskID])
}
