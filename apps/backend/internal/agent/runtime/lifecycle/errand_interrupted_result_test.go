//go:build linux

package lifecycle_test

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/db"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// These external capabilities are unused during durable result reconciliation.
// A call through an embedded nil capability fails the test instead of launching.
type interruptionExternalBoundaries struct {
	errands.Launcher
	errands.ErrandTeardown
	errands.ProfileResolver
	errands.MCPSelections
	notifications int
	publications  int
}

func (b *interruptionExternalBoundaries) PublishErrandUpdated(context.Context, *models.Errand, models.ErrandStatus) {
	b.publications++
}
func (b *interruptionExternalBoundaries) NotificationBlocked(context.Context, string) bool {
	return b.notifications > 0
}
func (b *interruptionExternalBoundaries) DeliverPending(context.Context, string) { b.notifications++ }
func (b *interruptionExternalBoundaries) AdmitPrepared(context.Context, string, string, map[string]interface{}) error {
	panic("unexpected prepared notification")
}

type interruptionTranscript struct {
	errands.TurnReader
	repo *tasksqlite.Repository
}

func (r interruptionTranscript) ReadTurn(ctx context.Context, sessionID, turnID string) (errands.TurnView, error) {
	turn, err := r.repo.GetTurn(ctx, turnID)
	if err != nil {
		return errands.TurnView{}, err
	}
	if turn.TaskSessionID != sessionID {
		return errands.TurnView{}, models.ErrInvalidErrand
	}
	return errands.TurnView{Completed: turn.CompletedAt != nil}, nil
}

func verifyInterruptedParentResult(t *testing.T, repo *tasksqlite.Repository) {
	t.Helper()
	service, boundary := newInterruptionResultService(t, repo)
	require.NoError(t, service.ReconcileTerminalDecisions(t.Context(), "session"))
	before, err := repo.GetErrand(t.Context(), "session")
	require.NoError(t, err)
	require.Equal(t, models.ErrandOutcomeFailed, before.LastOutcome)
	require.Equal(t, models.ErrandResultPending, before.ResultState)
	require.Equal(t, "The errand was interrupted before it finished.", before.ResultText)
	require.False(t, before.AwaitingResult)
	require.Equal(t, 1, boundary.notifications)
	require.NoError(t, service.ReconcileTerminalDecisions(t.Context(), "session"))
	after, err := repo.GetErrand(t.Context(), "session")
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, 1, boundary.notifications)
	require.Equal(t, 1, boundary.publications)
}

func TestErrandInterruptedDelegationNativeRecovery(t *testing.T) {
	manager, repo, journal := lifecycle.NewInterruptedDelegationRecoveryFixture(t, true)
	beforeTask, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	_, err = manager.ReconcileLoadedNativeDelivery(t.Context(), "session")
	require.NoError(t, err)
	verifyInterruptedParentResult(t, repo)
	_, err = manager.ReconcileLoadedNativeDelivery(t.Context(), "session")
	require.NoError(t, err)
	saved, err := repo.GetAgentDeliverySubmission(t.Context(), "interrupted")
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, saved.State)
	require.Equal(t, []byte("original instruction"), saved.Payload)
	require.Empty(t, saved.TerminalStreamID)
	require.Zero(t, saved.TerminalSequence)
	retained, err := journal.GetSubmission(t.Context(), "interrupted")
	require.NoError(t, err)
	require.True(t, retained.Retired)
	afterTask, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, beforeTask, afterTask)
}

func TestErrandInterruptedDelegationRetriesAfterRetirement(t *testing.T) {
	manager, repo, journal := lifecycle.NewInterruptedDelegationRecoveryFixture(t)
	_, err := repo.DB().Exec(`CREATE TRIGGER reject_interruption BEFORE UPDATE ON agent_delivery_effects WHEN NEW.effect_key = 'original-intent' BEGIN SELECT RAISE(ABORT, 'interruption failpoint'); END`)
	require.NoError(t, err)
	_, err = manager.ReconcileLoadedNativeDelivery(t.Context(), "session")
	require.ErrorContains(t, err, "interruption failpoint")
	retained, err := journal.GetSubmission(t.Context(), "interrupted")
	require.NoError(t, err)
	require.True(t, retained.Retired)
	turn, err := repo.GetTurn(t.Context(), "original-turn")
	require.NoError(t, err)
	require.Nil(t, turn.CompletedAt, "failed settlement must roll back turn closure")
	_, err = repo.DB().Exec(`DROP TRIGGER reject_interruption`)
	require.NoError(t, err)
	// Simulate evidence replacement after retirement. Only the assessment survives.
	_, err = repo.DB().Exec(`UPDATE executors_running SET metadata = '{}' WHERE session_id = 'session'`)
	require.NoError(t, err)
	_, err = manager.ReconcileLoadedNativeDelivery(t.Context(), "session")
	require.NoError(t, err)
	verifyInterruptedParentResult(t, repo)
}

func TestErrandInterruptedDelegationPersistenceFailurePreservesJournal(t *testing.T) {
	manager, repo, journal := lifecycle.NewInterruptedDelegationRecoveryFixture(t)
	_, err := repo.DB().Exec(`CREATE TRIGGER reject_assessment BEFORE INSERT ON agent_delivery_effects WHEN NEW.effect_type = 'errand.interrupted_delegation.v1' BEGIN SELECT RAISE(ABORT, 'assessment failpoint'); END`)
	require.NoError(t, err)
	_, err = manager.ReconcileLoadedNativeDelivery(t.Context(), "session")
	require.ErrorContains(t, err, "assessment failpoint")
	retained, err := journal.GetSubmission(t.Context(), "interrupted")
	require.NoError(t, err)
	require.False(t, retained.Retired)
	turn, err := repo.GetTurn(t.Context(), "original-turn")
	require.NoError(t, err)
	require.Nil(t, turn.CompletedAt)
}

func TestErrandInterruptedDelegationAdmissionOnlyDoesNotSettle(t *testing.T) {
	manager, repo, journal := lifecycle.NewInterruptedDelegationRecoveryFixture(t)
	_, err := journal.RetireSubmission(t.Context(), "interrupted", 2)
	require.NoError(t, err)
	require.NoError(t, repo.AbandonTurn(t.Context(), "original-turn"))
	_, err = manager.ReconcileLoadedNativeDelivery(t.Context(), "session")
	require.NoError(t, err)
	pending, err := repo.PendingErrandTurnSettlements(t.Context(), "session")
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "parent", pending[0].Outcome)
	row, err := repo.GetErrand(t.Context(), "session")
	require.NoError(t, err)
	require.True(t, row.AwaitingResult)
}

func TestErrandInterruptedDelegationRejectsChangedRecoverySnapshot(t *testing.T) {
	for _, field := range []string{"execution", "native", "incarnation", "generation", "submission", "hash", "stream"} {
		t.Run(field, func(t *testing.T) {
			manager, repo, journal := lifecycle.NewInterruptedDelegationRecoveryFixture(t)
			running, err := repo.GetExecutorRunningBySessionID(t.Context(), "session")
			require.NoError(t, err)
			require.NoError(t, repo.PreserveErrandInterruptedDelegation(t.Context(), running))
			_, err = journal.RetireSubmission(t.Context(), "interrupted", 2)
			require.NoError(t, err)
			proof, err := manager.GetNativeResumeDeliveryEvidence(t.Context(), "session")
			require.NoError(t, err)
			switch field {
			case "execution":
				proof.ExecutionID = "foreign"
			case "native":
				proof.NativeSessionID = "foreign"
			case "incarnation":
				proof.IncarnationID = "foreign"
			case "generation":
				proof.HarnessGeneration++
			case "submission":
				proof.Submissions[0].ID = "foreign"
			case "hash":
				proof.Submissions[0].PayloadHash = "foreign"
			case "stream":
				proof.Submissions[0].StreamID = "foreign"
			}
			require.Error(t, repo.ConsumeErrandInterruptedDelegations(t.Context(), proof))
			turn, err := repo.GetTurn(t.Context(), "original-turn")
			require.NoError(t, err)
			require.Nil(t, turn.CompletedAt)
			row, err := repo.GetErrand(t.Context(), "session")
			require.NoError(t, err)
			require.True(t, row.AwaitingResult)
		})
	}
}

func TestErrandInterruptedDelegationTerminalEvidenceWins(t *testing.T) {
	for _, mode := range []string{"unknown", "adopted", "cancelled", "receipt_replay", "adopted_turn", "rollback", "stream", "generation", "hash", "ambiguous"} {
		t.Run(mode, func(t *testing.T) { exerciseTerminalSupersession(t, mode) })
	}
}

func exerciseTerminalSupersession(t *testing.T, mode string) {
	t.Helper()
	manager, repo, _ := lifecycle.NewInterruptedDelegationRecoveryFixture(t)
	running, err := repo.GetExecutorRunningBySessionID(t.Context(), "session")
	require.NoError(t, err)
	require.NoError(t, repo.PreserveErrandInterruptedDelegation(t.Context(), running))
	_, err = repo.DB().Exec(`INSERT INTO agent_delivery_effects (effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome, created_at) VALUES ('actual-terminal', 'stream', 2, ?, 'completed', 'session', 'incarnation', 2, 'interrupted', 'original-turn', 'completed', CURRENT_TIMESTAMP)`, models.DeliveryTerminalEffectType)
	require.NoError(t, err)

	_, err = repo.DB().Exec(`UPDATE task_session_turns SET completed_at = CURRENT_TIMESTAMP WHERE id = 'original-turn'`)
	require.NoError(t, err)
	switch mode {
	case "receipt_replay":
		_, err = repo.DB().Exec(`UPDATE agent_delivery_effects SET state = 'completed', outcome = 'parent:done', completed_at = CURRENT_TIMESTAMP WHERE effect_key = 'original-intent'`)
		require.NoError(t, err)
		row, readErr := repo.GetErrand(t.Context(), "session")
		require.NoError(t, readErr)
		row.LastOutcome, row.ResultState, row.ResultTurnID, row.AwaitingResult = models.ErrandOutcomeDone, models.ErrandResultPending, "original-turn", false
		row.ResultText = "actual terminal result"
		err = repo.UpdateErrand(t.Context(), row)
	case "cancelled":
		_, err = repo.DB().Exec(`UPDATE task_session_errands SET lifecycle = 'cancelled', cancel_source = 'parent', result_state = 'discarded' WHERE id = 'session'`)
	case "adopted":
		_, err = repo.DB().Exec(`UPDATE agent_delivery_submissions SET state = 'completed', terminal_stream_id = 'stream', terminal_sequence = 2, terminal_turn_id = 'original-turn' WHERE id = 'interrupted'`)
	case "adopted_turn":
		_, err = repo.DB().Exec(`UPDATE agent_delivery_submissions SET state = 'completed', terminal_stream_id = 'stream', terminal_sequence = 2, terminal_turn_id = 'foreign' WHERE id = 'interrupted'`)
	case "rollback":
		_, err = repo.DB().Exec(`CREATE TRIGGER reject_supersession BEFORE UPDATE ON agent_delivery_effects WHEN NEW.state = 'block_resolved' BEGIN SELECT RAISE(ABORT, 'supersession failpoint'); END`)
	case "stream":
		_, err = repo.DB().Exec(`UPDATE agent_delivery_effects SET stream_id = 'foreign' WHERE effect_key = 'actual-terminal'`)
	case "generation":
		_, err = repo.DB().Exec(`UPDATE agent_delivery_effects SET harness_generation = 7 WHERE effect_key = 'actual-terminal'`)
	case "hash":
		_, err = repo.DB().Exec(`UPDATE agent_delivery_submissions SET payload_hash = 'changed' WHERE id = 'interrupted'`)
	case "ambiguous":
		_, err = repo.DB().Exec(`INSERT INTO agent_delivery_effects (effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome, created_at, completed_at) SELECT 'other-terminal', stream_id, sequence + 1, effect_type, state, session_id, incarnation_id, harness_generation, submission_id, turn_id, 'failed', created_at, completed_at FROM agent_delivery_effects WHERE effect_key = 'actual-terminal'`)
	}
	require.NoError(t, err)
	service, boundary := newInterruptionResultService(t, repo)
	service.SetInterruptedDelegationReconciler(manager.ReconcileInterruptedErrandDelivery)
	if mode != "unknown" && mode != "adopted" && mode != "cancelled" && mode != "receipt_replay" {
		require.Error(t, service.ReconcileTerminalDecisions(t.Context(), "session"))
		pending, err := repo.HasPendingInterruptedDelegation(t.Context(), "session")
		require.NoError(t, err)
		require.True(t, pending)
		row, err := repo.GetErrand(t.Context(), "session")
		require.NoError(t, err)
		require.True(t, row.AwaitingResult)
		require.Zero(t, boundary.notifications)
		if mode != "rollback" {
			return
		}
		_, err = repo.DB().Exec("DROP TRIGGER reject_supersession")
		require.NoError(t, err)
	}
	require.NoError(t, service.ReconcileTerminalDecisions(t.Context(), "session"))
	row, err := repo.GetErrand(t.Context(), "session")
	require.NoError(t, err)
	if mode == "cancelled" {
		require.Equal(t, models.ErrandLifecycleCancelled, row.Lifecycle)
		require.Equal(t, models.ErrandResultDiscarded, row.ResultState)
		return
	}
	require.Equal(t, models.ErrandOutcomeDone, row.LastOutcome)
	require.False(t, row.AwaitingResult)
	if mode == "receipt_replay" {
		require.Zero(t, boundary.notifications)
		require.Zero(t, boundary.publications)
		require.Equal(t, "actual terminal result", row.ResultText)
	} else {
		require.Equal(t, 1, boundary.notifications)
	}

	for range 2 {
		require.NoError(t, service.ReconcileTerminalDecisions(t.Context(), "session"))
		pending, err := repo.HasPendingInterruptedDelegation(t.Context(), "session")
		require.NoError(t, err)
		require.False(t, pending)
		unsettled, err := repo.HasUnsettledInterruptedDelegation(t.Context(), "session")
		require.NoError(t, err)
		require.False(t, unsettled)
	}
	effect, err := repo.GetAgentDeliveryEffect(t.Context(), "errand.interruption:original-intent")
	require.NoError(t, err)
	require.Equal(t, models.DeliveryEffectBlockResolved, effect.State)
	require.Contains(t, effect.Outcome, "original_owner_terminated")
	require.NoError(t, repo.CreateTurn(t.Context(), &models.Turn{ID: "next-turn", TaskID: "task", TaskSessionID: "session", StartedAt: time.Now().UTC()}))
	require.NoError(t, repo.ReserveErrandTurnSettlement(t.Context(), "task", &models.AgentDeliveryEffect{EffectKey: "successor", EffectType: db.ErrandTurnSettlementEffect, SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream", SubmissionID: "next", TurnID: "next-turn", Outcome: "parent"}))
}

func TestErrandInterruptedDelegationNonErrandIsolation(t *testing.T) {
	manager, repo, _ := lifecycle.NewInterruptedDelegationRecoveryFixture(t)
	_, err := repo.DB().Exec(`UPDATE task_sessions SET role = 'primary' WHERE id = 'session'`)
	require.NoError(t, err)
	_, err = manager.ReconcileLoadedNativeDelivery(t.Context(), "session")
	require.NoError(t, err)
	var count int
	require.NoError(t, repo.DB().QueryRow(`SELECT count(*) FROM agent_delivery_effects WHERE effect_type = ?`, models.InterruptedDelegationEffectType).Scan(&count))
	require.Zero(t, count)
	turn, err := repo.GetTurn(t.Context(), "original-turn")
	require.NoError(t, err)
	require.Nil(t, turn.CompletedAt)
}

func TestErrandInterruptedDelegationRejectsMalformedAssessment(t *testing.T) {
	for _, mutation := range []string{"sequence", "turn", "version", "json"} {
		t.Run(mutation, func(t *testing.T) {
			manager, repo, journal := lifecycle.NewInterruptedDelegationRecoveryFixture(t)
			running, err := repo.GetExecutorRunningBySessionID(t.Context(), "session")
			require.NoError(t, err)
			require.NoError(t, repo.PreserveErrandInterruptedDelegation(t.Context(), running))
			switch mutation {
			case "sequence":
				_, err = repo.DB().Exec(`UPDATE agent_delivery_effects SET sequence = 7 WHERE effect_type = ?`, models.InterruptedDelegationEffectType)
			case "turn":
				_, err = repo.DB().Exec(`UPDATE agent_delivery_effects SET turn_id = 'foreign' WHERE effect_type = ?`, models.InterruptedDelegationEffectType)
			case "version":
				_, err = repo.DB().Exec(`UPDATE agent_delivery_effects SET outcome = replace(outcome, '"version":1', '"version":9') WHERE effect_type = ?`, models.InterruptedDelegationEffectType)
			case "json":
				_, err = repo.DB().Exec(`UPDATE agent_delivery_effects SET outcome = 'broken' WHERE effect_type = ?`, models.InterruptedDelegationEffectType)
			}
			require.NoError(t, err)
			_, err = manager.ReconcileLoadedNativeDelivery(t.Context(), "session")
			require.Error(t, err)
			retained, readErr := journal.GetSubmission(t.Context(), "interrupted")
			require.NoError(t, readErr)
			require.False(t, retained.Retired, "malformed saved proof cannot authorize destructive retirement")
			turn, err := repo.GetTurn(t.Context(), "original-turn")
			require.NoError(t, err)
			require.Nil(t, turn.CompletedAt)
		})
	}
}

func newInterruptionResultService(t *testing.T, repo *tasksqlite.Repository) (*errands.Service, *interruptionExternalBoundaries) {
	t.Helper()
	boundary := &interruptionExternalBoundaries{}
	service, err := errands.New(errands.Dependencies{Store: repo, Launcher: boundary, Stopper: boundary, Profiles: boundary, MCP: boundary, Events: boundary, Deliverer: boundary})
	require.NoError(t, err)
	service.SetTurnReader(interruptionTranscript{repo: repo})

	return service, boundary
}

func TestErrandInterruptedDelegationPreservesLaterWorkAndCancellation(t *testing.T) {
	for _, mode := range []string{"later_work", "parent_cancel", "user_cancel"} {
		t.Run(mode, func(t *testing.T) {
			manager, repo, _ := lifecycle.NewInterruptedDelegationRecoveryFixture(t)
			row, err := repo.GetErrand(t.Context(), "session")
			require.NoError(t, err)
			if mode == "later_work" {
				at := time.Now().UTC().Add(time.Second)
				_, err = repo.DB().Exec(`INSERT INTO task_session_turns (id, task_id, task_session_id, started_at, completed_at, created_at, updated_at, metadata) VALUES ('later-work', 'task', 'session', ?, ?, ?, ?, '{}')`, at, at, at, at)
				require.NoError(t, err)
				row.ResultTurnID, row.LatestSettledTurnID = "later-work", "later-work"
				row.ResultText, row.ResultKind, row.ResultState = "later independent result", models.ErrandResultOutput, models.ErrandResultPending
				row.LastOutcome, row.AwaitingResult = models.ErrandOutcomeDone, false
			} else {
				row.Lifecycle = models.ErrandLifecycleCancelled
				row.CancelSource = models.ErrandCancelSourceParent
				if mode == "user_cancel" {
					row.CancelSource = models.ErrandCancelSourceUser
				}
				row.ResultState = models.ErrandResultDiscarded
			}
			require.NoError(t, repo.UpdateErrand(t.Context(), row))
			before, err := repo.GetErrand(t.Context(), "session")
			require.NoError(t, err)
			_, err = manager.ReconcileLoadedNativeDelivery(t.Context(), "session")
			require.NoError(t, err)
			service, boundary := newInterruptionResultService(t, repo)
			err = service.ReconcileTerminalDecisions(t.Context(), "session")
			if mode == "later_work" {
				require.ErrorIs(t, err, db.ErrErrandSettlementPending)
			} else {
				require.NoError(t, err)
			}
			after, err := repo.GetErrand(t.Context(), "session")
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Zero(t, boundary.notifications)
			effect, err := repo.GetAgentDeliveryEffect(t.Context(), "original-intent")
			require.NoError(t, err)
			require.Equal(t, models.DeliveryEffectCompleted, effect.State)
		})
	}
}

func TestErrandInterruptedDelegationRetriesFromResultReconciliation(t *testing.T) {
	manager, repo, journal := lifecycle.NewInterruptedDelegationRecoveryFixture(t)
	running, err := repo.GetExecutorRunningBySessionID(t.Context(), "session")
	require.NoError(t, err)
	require.NoError(t, repo.PreserveErrandInterruptedDelegation(t.Context(), running))
	_, err = journal.RetireSubmission(t.Context(), "interrupted", 2)
	require.NoError(t, err)
	service, boundary := newInterruptionResultService(t, repo)
	setter, canSet := any(service).(interface {
		SetInterruptedDelegationReconciler(func(context.Context, string) error)
	})
	reconciler, canReconcile := any(manager).(interface {
		ReconcileInterruptedErrandDelivery(context.Context, string) error
	})
	if canSet && canReconcile {
		setter.SetInterruptedDelegationReconciler(reconciler.ReconcileInterruptedErrandDelivery)
	}
	require.NoError(t, service.ReconcileTerminalDecisions(t.Context(), "session"))
	after, err := repo.GetErrand(t.Context(), "session")
	require.NoError(t, err)
	require.False(t, after.AwaitingResult, "restart result reconciliation must consume retained proof after native retirement")
	require.Equal(t, models.ErrandOutcomeFailed, after.LastOutcome)
	require.Equal(t, 1, boundary.notifications)
}

func TestErrandInterruptedDelegationRetriesResultCommitOnSweep(t *testing.T) {
	manager, repo, _ := lifecycle.NewInterruptedDelegationRecoveryFixture(t)
	_, err := manager.ReconcileLoadedNativeDelivery(t.Context(), "session")
	require.NoError(t, err)
	service, boundary := newInterruptionResultService(t, repo)
	service.SetInterruptedDelegationReconciler(manager.ReconcileInterruptedErrandDelivery)
	_, err = repo.DB().Exec(`CREATE TRIGGER reject_result BEFORE UPDATE ON task_session_errands BEGIN SELECT RAISE(ABORT, 'result failpoint'); END`)
	require.NoError(t, err)
	require.ErrorContains(t, service.ReconcileTerminalDecisions(t.Context(), "session"), "result failpoint")
	require.Zero(t, boundary.notifications)
	_, err = repo.DB().Exec(`DROP TRIGGER reject_result`)
	require.NoError(t, err)
	require.NoError(t, service.SweepDelivery(t.Context()))
	after, err := repo.GetErrand(t.Context(), "session")
	require.NoError(t, err)
	require.False(t, after.AwaitingResult)
	require.Equal(t, models.ErrandOutcomeFailed, after.LastOutcome)
	require.Equal(t, 1, boundary.notifications)
}

func TestErrandInterruptedDelegationRetriesBetweenCleanupAndRetirement(t *testing.T) {
	manager, repo, _ := lifecycle.NewInterruptedDelegationRecoveryFixture(t)
	running, err := repo.GetExecutorRunningBySessionID(t.Context(), "session")
	require.NoError(t, err)
	require.NoError(t, repo.PreserveErrandInterruptedDelegation(t.Context(), running))
	_, err = repo.DB().Exec(`UPDATE executors_running SET metadata = '{}' WHERE session_id = 'session'`)
	require.NoError(t, err)
	_, err = manager.ReconcileLoadedNativeDelivery(t.Context(), "session")
	require.NoError(t, err, "durable original-owner proof must survive source binding cleanup before retirement")
	verifyInterruptedParentResult(t, repo)
}

func TestErrandInterruptedDelegationEarlierGenerationCleanupRetry(t *testing.T) {
	manager, repo, _ := lifecycle.NewEarlierInterruptedDelegationRecoveryFixture(t)
	running, err := repo.GetExecutorRunningBySessionID(t.Context(), "session")
	require.NoError(t, err)
	require.NoError(t, repo.PreserveErrandInterruptedDelegation(t.Context(), running))
	_, err = repo.DB().Exec(`UPDATE executors_running SET metadata = '{}' WHERE session_id = 'session'`)
	require.NoError(t, err)
	_, err = manager.ReconcileLoadedNativeDelivery(t.Context(), "session")
	require.NoError(t, err)
	verifyInterruptedParentResult(t, repo)
}

func TestErrandInterruptedDelegationExistingTerminalNeedsNoAssessment(t *testing.T) {
	manager, repo, _ := lifecycle.NewInterruptedDelegationRecoveryFixture(t)
	_, err := repo.DB().Exec(`INSERT INTO agent_delivery_effects (effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome, created_at) VALUES ('actual-terminal', 'stream', 2, ?, 'completed', 'session', 'incarnation', 2, 'interrupted', 'original-turn', 'completed', CURRENT_TIMESTAMP)`, models.DeliveryTerminalEffectType)
	require.NoError(t, err)
	require.NoError(t, repo.CompleteTurn(t.Context(), "original-turn"))
	_, err = manager.ReconcileLoadedNativeDelivery(t.Context(), "session")
	require.NoError(t, err)
	service, _ := newInterruptionResultService(t, repo)
	require.NoError(t, service.ReconcileTerminalDecisions(t.Context(), "session"))
	row, err := repo.GetErrand(t.Context(), "session")
	require.NoError(t, err)
	require.Equal(t, models.ErrandOutcomeDone, row.LastOutcome)
	var count int
	require.NoError(t, repo.DB().QueryRow(`SELECT count(*) FROM agent_delivery_effects WHERE effect_type = ?`, models.InterruptedDelegationEffectType).Scan(&count))
	require.Zero(t, count)
}

type interruptionCursorFailureRepository struct {
	*tasksqlite.Repository
	journal *journal.Journal
}

func (r *interruptionCursorFailureRepository) GetAgentDeliveryCursor(ctx context.Context, stream string) (*models.AgentDeliveryCursor, error) {
	submission, err := r.journal.GetSubmission(ctx, "interrupted")
	if err != nil {
		return nil, err
	}
	if submission.Retired {
		return nil, errors.New("post-retirement cursor unavailable")
	}
	return r.Repository.GetAgentDeliveryCursor(ctx, stream)
}

func TestErrandInterruptedDelegationLeavesOrdinaryNativeAcknowledgementUnchanged(t *testing.T) {
	manager, repo, journal := lifecycle.NewInterruptedDelegationRecoveryFixture(t)
	_, err := repo.DB().Exec(`UPDATE task_sessions SET role = 'primary' WHERE id = 'session'`)
	require.NoError(t, err)
	manager.SetAgentDeliveryRepository(&interruptionCursorFailureRepository{Repository: repo, journal: journal})
	require.NoError(t, manager.AcknowledgeNativeResumeDelivery(t.Context(), "session"), "ordinary native acknowledgement must not acquire an Errand settlement snapshot")
	retained, err := journal.GetSubmission(t.Context(), "interrupted")
	require.NoError(t, err)
	require.True(t, retained.Retired)
}
