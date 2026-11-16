package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func completedExactResult(row *models.Errand, effect *models.AgentDeliveryEffect) {
	row.LatestSettledTurnID, row.ResultTurnID, row.ResultText = effect.TurnID, effect.TurnID, "exact output"
	row.ResultState, row.ResultKind, row.LastOutcome, row.AwaitingResult = models.ErrandResultPending, models.ErrandResultOutput, models.ErrandOutcomeDone, false
	at := time.Now().UTC()
	row.SettledAt = &at
	effect.Outcome, effect.State, effect.CompletedAt = "parent:done", models.DeliveryEffectCompleted, &at
}

func TestErrandExactTerminalDecisionSerializesReservation(t *testing.T) {
	exerciseExactSerialization(t, newRepoForSessionTests(t))
}

func TestPostgresErrandExactTerminalDecisionSerializesReservation(t *testing.T) {
	exerciseExactSerialization(t, newPostgresErrandRepo(t))
}

func exerciseExactSerialization(t *testing.T, repo *Repository) {
	t.Helper()
	row, effect := seedExactSettlement(t, repo)
	store := exactSettlementStore(t, repo)
	require.NoError(t, store.ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
	require.NoError(t, repo.CompleteTurn(t.Context(), effect.TurnID))
	completedExactResult(row, effect)
	decisionGate := newErrandSQLGate("UPDATE agent_delivery_effects SET state", true)
	defer decisionGate.unblock()
	writer := independentErrandRepository(t, repo, decisionGate)
	guardQuery := "UPDATE tasks SET id = id WHERE 0"
	if dialect.IsPostgres(repo.db.DriverName()) {
		guardQuery = "SELECT id FROM tasks"
	}
	reservationGate := newErrandSQLGate(guardQuery, false)
	reserver := independentErrandRepository(t, repo, reservationGate)
	taskID, childID := row.TaskID, row.ID
	decisionDone := make(chan error, 1)
	go func() {
		decisionDone <- commitExactForTest(writer, t.Context(), row, effect)
	}()
	awaitSignal(t, decisionGate.entered, "decision transaction paused before receipt write")
	reservationDone := make(chan error, 1)
	go func() {
		_, err := reserver.CreateTurnWithStepStamp(t.Context(), &models.Turn{ID: "serialized-B", TaskID: taskID, TaskSessionID: childID, StartedAt: time.Now()})
		reservationDone <- err
	}()
	awaitSignal(t, reservationGate.entered, "successor attempting the shared task guard")
	decisionGate.unblock()
	require.NoError(t, awaitError(t, decisionDone, "exact decision commit"))
	require.NoError(t, awaitError(t, reservationDone, "reservation following decision"))
	stored, err := repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, "exact output", stored.ResultText)
}

func TestErrandExactTerminalDecisionPreexistingSuccessor(t *testing.T) {
	exercisePreexistingSuccessor(t, newRepoForSessionTests(t))
}

func TestPostgresErrandExactTerminalDecisionPreexistingSuccessor(t *testing.T) {
	exercisePreexistingSuccessor(t, newPostgresErrandRepo(t))
}

func exercisePreexistingSuccessor(t *testing.T, repo *Repository) {
	t.Helper()
	row, effect := seedExactSettlement(t, repo)
	store := exactSettlementStore(t, repo)
	require.NoError(t, store.ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
	require.NoError(t, repo.CompleteTurn(t.Context(), effect.TurnID))
	// An older backend admitted B without the settlement gate. Recovery must
	// still preserve B rather than interpreting A's pending receipt as current.
	at := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_session_turns
		(id, task_id, task_session_id, started_at, created_at, updated_at, metadata)
		VALUES ('preexisting-B', ?, ?, ?, ?, ?, '{}')`), row.TaskID, row.ID, at, at, at)
	require.NoError(t, err)
	completedExactResult(row, effect)
	require.Error(t, commitExactForTest(store, t.Context(), row, effect))
	stored, err := repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.True(t, stored.AwaitingResult)
	require.Empty(t, stored.LatestSettledTurnID)
}

func TestErrandExactTerminalDecisionReplayReturnsCommittedRow(t *testing.T) {
	exerciseExactReplayRow(t, newRepoForSessionTests(t))
}

func TestPostgresErrandExactTerminalDecisionReplayReturnsCommittedRow(t *testing.T) {
	exerciseExactReplayRow(t, newPostgresErrandRepo(t))
}

func exerciseExactReplayRow(t *testing.T, repo *Repository) {
	t.Helper()
	row, effect := seedExactSettlement(t, repo)
	store := exactSettlementStore(t, repo)
	require.NoError(t, store.ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
	require.NoError(t, repo.CompleteTurn(t.Context(), effect.TurnID))
	completedExactResult(row, effect)
	require.NoError(t, commitExactForTest(store, t.Context(), row, effect))
	old := *row
	a := parentAdmission(row, "replay-B")
	_, err := repo.ReserveErrandParentAdmission(t.Context(), a)
	require.NoError(t, err)
	require.NoError(t, repo.DecideErrandParentAdmission(t.Context(), a, models.ErrandAdmissionAccepted))
	require.NoError(t, commitExactForTest(store, t.Context(), &old, effect))
	require.Equal(t, models.ErrandResultSuperseded, old.ResultState, "replay must return a committed row for publication")
	require.True(t, old.AwaitingResult)
}

func TestErrandExactTerminalDecisionReceiptFailureRollsBackResult(t *testing.T) {
	exerciseExactReceiptFailure(t, newRepoForSessionTests(t))
}

func TestPostgresErrandExactTerminalDecisionReceiptFailureRollsBackResult(t *testing.T) {
	exerciseExactReceiptFailure(t, newPostgresErrandRepo(t))
}

func exerciseExactReceiptFailure(t *testing.T, repo *Repository) {
	t.Helper()
	row, effect := seedExactSettlement(t, repo)
	require.NoError(t, exactSettlementStore(t, repo).ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
	require.NoError(t, repo.CompleteTurn(t.Context(), effect.TurnID))
	completedExactResult(row, effect)
	gate := newErrandSQLGate("UPDATE agent_delivery_effects SET state", true)
	defer gate.unblock()
	faulted := independentErrandRepository(t, repo, gate)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- commitExactForTest(faulted, ctx, row, effect) }()
	awaitSignal(t, gate.entered, "receipt write after row mutation")
	cancel()
	require.Error(t, awaitError(t, done, "cancelled receipt transaction"))
	stored, err := repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Empty(t, stored.ResultText)
	pending, err := exactSettlementStore(t, repo).PendingErrandTurnSettlements(t.Context(), row.ID)
	require.NoError(t, err)
	require.Len(t, pending, 1)
}

func TestErrandExactTerminalDecisionRejectsMissingAndAmbiguousEvidence(t *testing.T) {
	exerciseExactAmbiguity(t, newRepoForSessionTests(t))
}

func TestPostgresErrandExactTerminalDecisionRejectsMissingAndAmbiguousEvidence(t *testing.T) {
	exerciseExactAmbiguity(t, newPostgresErrandRepo(t))
}

func exerciseExactAmbiguity(t *testing.T, repo *Repository) {
	t.Helper()
	row, effect := seedExactSettlement(t, repo)
	require.NoError(t, repo.CompleteTurn(t.Context(), effect.TurnID))
	_, err := repo.ClaimPromptableTaskSessionIfActive(t.Context(), row.ID)
	require.Error(t, err, "missing receipt cannot authorize a successor")
	require.NoError(t, exactSettlementStore(t, repo).ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
	completedExactResult(row, effect)
	require.NoError(t, commitExactForTest(repo, t.Context(), row, effect))
	_, err = repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_effects
		(effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, submission_id, turn_id, outcome, created_at)
		VALUES ('ambiguous-exact', 'other-execution', 0, 'errand.turn_settlement', 'pending', ?, ?, 'foreign-submission', ?, 'user', ?)`), row.ID, effect.IncarnationID, effect.TurnID, time.Now().UTC())
	require.NoError(t, err)
	_, err = repo.ClaimPromptableTaskSessionIfActive(t.Context(), row.ID)
	require.Error(t, err, "one completed plus one conflicting intent is ambiguous")
}

func TestErrandExactTerminalDecisionRecoversProjectedEvidence(t *testing.T) {
	exerciseProjectedErrandEvidence(t, newRepoForSessionTests)
}

func TestPostgresErrandExactTerminalDecisionRecoversProjectedEvidence(t *testing.T) {
	exerciseProjectedErrandEvidence(t, newPostgresErrandRepo)
}

func exerciseProjectedErrandEvidence(t *testing.T, create func(*testing.T) *Repository) {
	t.Helper()
	cases := []struct {
		name, terminalOutcome, expected                                string
		generation                                                     int64
		foreignSubmission, pendingProjection, ambiguous, missingIntent bool
		projectedSequence, cursorGeneration                            int64
	}{
		{name: "done", terminalOutcome: "completed", expected: "parent:done", generation: 1},
		{name: "failed", terminalOutcome: "failed", expected: "parent:failed", generation: 1},
		{name: "cancelled", terminalOutcome: "cancelled", expected: "parent:interrupted", generation: 1},
		{name: "foreign_generation", terminalOutcome: "completed", expected: errandTerminalParentOrigin, generation: 2},
		{name: "foreign_submission", terminalOutcome: "completed", expected: errandTerminalParentOrigin, generation: 1, foreignSubmission: true},
		{name: "unprojected", terminalOutcome: "completed", expected: errandTerminalParentOrigin, generation: 1, pendingProjection: true},
		{name: "ambiguous", terminalOutcome: "completed", expected: errandTerminalParentOrigin, generation: 1, ambiguous: true},
		{name: "legacy_first_identity", terminalOutcome: "completed", expected: "parent:done", generation: 1, missingIntent: true},
		{name: "projected_pending", terminalOutcome: "completed", expected: "parent:done", generation: 1, pendingProjection: true, projectedSequence: 7, cursorGeneration: 1},
		{name: "foreign_cursor_generation", terminalOutcome: "completed", expected: errandTerminalParentOrigin, generation: 1, pendingProjection: true, projectedSequence: 7, cursorGeneration: 2},
		{name: "incomplete_projection", terminalOutcome: "completed", expected: errandTerminalParentOrigin, generation: 1, pendingProjection: true, projectedSequence: 6, cursorGeneration: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := create(t)
			row, effect := seedExactSettlement(t, repo)
			store := exactSettlementStore(t, repo)
			if !tc.missingIntent {
				require.NoError(t, store.ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
			}
			require.NoError(t, repo.CompleteTurn(t.Context(), effect.TurnID))
			submissionID, state := effect.SubmissionID, models.DeliveryEffectBlockResolved
			if tc.foreignSubmission {
				submissionID = "foreign-submission"
			}
			if tc.pendingProjection {
				state = models.DeliveryEffectPending
			}
			_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_effects
				(effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome, created_at)
				VALUES ('projected-terminal', 'real-stream', 7, ?, ?, ?, ?, ?, ?, ?, ?, ?)`), models.DeliveryTerminalEffectType,
				state, row.ID, effect.IncarnationID, tc.generation, submissionID, effect.TurnID, tc.terminalOutcome, time.Now().UTC())
			require.NoError(t, err)
			if tc.projectedSequence > 0 {
				_, err = repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_cursors
					(stream_id, session_id, incarnation_id, harness_generation, received_sequence, projected_sequence, updated_at)
					VALUES ('real-stream', ?, ?, ?, 7, ?, ?)`), row.ID, effect.IncarnationID,
					tc.cursorGeneration, tc.projectedSequence, time.Now().UTC())
				require.NoError(t, err)
			}
			if tc.ambiguous {
				_, err = repo.db.Exec(`INSERT INTO agent_delivery_effects SELECT 'conflicting-terminal', stream_id, 8,
					effect_type, state, created_at, completed_at, session_id, incarnation_id, harness_generation, submission_id, turn_id, 'failed'
					FROM agent_delivery_effects WHERE effect_key = 'projected-terminal'`)
				require.NoError(t, err)
			}
			pending, err := store.PendingErrandTurnSettlements(t.Context(), row.ID)
			require.NoError(t, err)
			require.Len(t, pending, 1)
			require.Equal(t, tc.expected, pending[0].Outcome)
		})
	}
}
