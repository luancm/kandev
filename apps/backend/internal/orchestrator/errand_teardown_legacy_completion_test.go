package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type legacyCompletedIdentityRepo struct {
	*sqliterepo.Repository
	decision     func(*models.AgentDeliveryEffect) *models.AgentDeliveryEffect
	decisionErr  error
	inventoryErr error
}

func (r *legacyCompletedIdentityRepo) GetErrandTurnSettlementDecision(ctx context.Context, sessionID, incarnationID, turnID string) (*models.AgentDeliveryEffect, error) {
	if r.decisionErr != nil {
		return nil, r.decisionErr
	}
	decision, err := r.Repository.GetErrandTurnSettlementDecision(ctx, sessionID, incarnationID, turnID)
	if err != nil || r.decision == nil {
		return decision, err
	}
	return r.decision(decision), nil
}

func (r *legacyCompletedIdentityRepo) ListAgentDeliverySubmissions(ctx context.Context, sessionID string) ([]*models.AgentDeliverySubmission, error) {
	if r.inventoryErr != nil {
		return nil, r.inventoryErr
	}
	return r.Repository.ListAgentDeliverySubmissions(ctx, sessionID)
}

func newLegacyCompletedIdentityFixture(t *testing.T) (*errandTeardownFixture, *errands.Service) {
	t.Helper()
	f := newErrandTeardownFixture(t)
	service, _ := teardownIntegrationService(t, f, f.repo)
	service.SetTurnReader(settlementTranscript{f.repo})
	ctx := t.Context()
	session, err := f.repo.GetTaskSession(ctx, f.row.ID)
	require.NoError(t, err)
	require.NoError(t, f.svc.reserveErrandTerminalIntent(ctx, session,
		errands.ParentPromptOrigin{SenderSessionID: f.row.ParentSessionID}, f.row.FirstTurnID, f.row.SubmissionID, ""))
	require.NoError(t, f.svc.turnService.CompleteTurn(ctx, f.row.FirstTurnID))
	require.NoError(t, service.OnErrandTurnSettled(ctx, errands.TurnSettlement{
		SessionID: f.row.ID, TurnID: f.row.FirstTurnID, Outcome: models.ErrandOutcomeDone,
	}))
	turn := &models.Turn{ID: "legacy-user-turn", TaskID: f.row.TaskID, TaskSessionID: f.row.ID,
		StartedAt: time.Now().UTC()}
	require.NoError(t, f.repo.CreateTurn(ctx, turn))
	require.NoError(t, f.svc.reserveErrandTerminalIntent(ctx, session, errands.ParentPromptOrigin{}, turn.ID, "", "exec-1"))
	require.NoError(t, f.svc.turnService.CompleteTurn(ctx, turn.ID))
	require.NoError(t, service.OnErrandTurnSettled(ctx, errands.TurnSettlement{
		SessionID: f.row.ID, TurnID: turn.ID, Outcome: models.ErrandOutcomeDone,
	}))
	f.row, err = f.repo.GetErrand(ctx, f.row.ID)
	require.NoError(t, err)
	require.NotNil(t, f.row.TeardownIntent)
	require.Equal(t, models.ErrandTeardownIntentPending, f.row.TeardownIntent.State)
	require.Equal(t, turn.ID, f.row.TeardownIntent.Target.TurnID)
	require.NotEqual(t, f.row.SubmissionID, f.row.TeardownIntent.Target.SubmissionID)
	require.Equal(t, models.ErrandOutcomeDone, f.row.LastOutcome)
	require.Equal(t, models.ErrandResultPending, f.row.ResultState)
	submissions, err := f.repo.ListAgentDeliverySubmissions(ctx, f.row.ID)
	require.NoError(t, err)
	require.Empty(t, submissions)
	require.Empty(t, f.provider.stops)
	return f, service
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.1 AC-TASKS-ERRAND-COMPLETION-001.3
func TestErrandTeardownLegacyCompletedIdentityRejectsInvalidDecisions(t *testing.T) {
	cases := []struct {
		name   string
		change func(*models.AgentDeliveryEffect) *models.AgentDeliveryEffect
	}{
		{"missing", func(*models.AgentDeliveryEffect) *models.AgentDeliveryEffect { return nil }},
		{"uncommitted", func(e *models.AgentDeliveryEffect) *models.AgentDeliveryEffect {
			e.State = models.DeliveryEffectPending
			return e
		}},
		{"unsealed", func(e *models.AgentDeliveryEffect) *models.AgentDeliveryEffect { e.Outcome = "user"; return e }},
		{"failed", func(e *models.AgentDeliveryEffect) *models.AgentDeliveryEffect { e.Outcome = "user:failed"; return e }},
		{"cancelled", func(e *models.AgentDeliveryEffect) *models.AgentDeliveryEffect {
			e.Outcome = "user:cancelled"
			return e
		}},
		{"session", func(e *models.AgentDeliveryEffect) *models.AgentDeliveryEffect {
			e.SessionID = "other-session"
			return e
		}},
		{"incarnation", func(e *models.AgentDeliveryEffect) *models.AgentDeliveryEffect {
			e.IncarnationID = "other-incarnation"
			return e
		}},
		{"historical turn", func(e *models.AgentDeliveryEffect) *models.AgentDeliveryEffect { e.TurnID = "turn-1"; return e }},
		{"submission", func(e *models.AgentDeliveryEffect) *models.AgentDeliveryEffect {
			e.SubmissionID = "other-submission"
			return e
		}},
		{"execution", func(e *models.AgentDeliveryEffect) *models.AgentDeliveryEffect {
			e.StreamID = "other-execution"
			return e
		}},
		{"missing execution", func(e *models.AgentDeliveryEffect) *models.AgentDeliveryEffect { e.StreamID = ""; return e }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := newLegacyCompletedIdentityFixture(t)
			// Cancellation inherits the exact target but does not run completion's
			// separate success-projection validation after resolving identity.
			inheritLegacyCompletionCancellation(t, f)
			f.svc.repo = &legacyCompletedIdentityRepo{Repository: f.repo, decision: tc.change}
			_, err := NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), *f.row.TeardownIntent)
			require.Error(t, err, "aggregate success and an earlier parent result are insufficient")
			requireLegacyIdentityRefused(t, f)
		})
	}
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.1 AC-TASKS-ERRAND-COMPLETION-001.3
func TestErrandTeardownLegacyCompletedIdentityRequiresCompletedCurrentTurn(t *testing.T) {
	for _, kind := range []string{"incomplete", "different latest settlement"} {
		t.Run(kind, func(t *testing.T) {
			f, _ := newLegacyCompletedIdentityFixture(t)
			inheritLegacyCompletionCancellation(t, f)
			if kind == "incomplete" {
				turn, err := f.repo.GetTurn(t.Context(), f.row.TeardownIntent.Target.TurnID)
				require.NoError(t, err)
				turn.CompletedAt = nil
				require.NoError(t, f.repo.UpdateTurn(t.Context(), turn))
			} else {
				f.row.LatestSettledTurnID = f.row.FirstTurnID
				require.NoError(t, f.repo.UpdateErrand(t.Context(), f.row))
			}
			_, err := NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), *f.row.TeardownIntent)
			require.Error(t, err)
			requireLegacyIdentityRefused(t, f)
		})
	}
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.1 AC-TASKS-ERRAND-COMPLETION-001.4
func TestErrandTeardownLegacyCompletedIdentityPreservesReadErrors(t *testing.T) {
	for _, source := range []string{"submission inventory", "terminal decision"} {
		t.Run(source, func(t *testing.T) {
			f, _ := newLegacyCompletedIdentityFixture(t)
			inheritLegacyCompletionCancellation(t, f)
			fault := errors.New("retained evidence read failed")
			store := &legacyCompletedIdentityRepo{Repository: f.repo}
			if source == "submission inventory" {
				store.inventoryErr = fault
			} else {
				store.decisionErr = fault
			}
			f.svc.repo = store
			_, err := NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), *f.row.TeardownIntent)
			require.ErrorIs(t, err, fault)
			requireLegacyIdentityRefused(t, f)
		})
	}
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.3
func TestErrandTeardownLegacyCompletedIdentityKeepsConflictingSubmission(t *testing.T) {
	for _, state := range []models.DeliverySubmissionState{models.DeliverySubmissionAccepted, models.DeliverySubmissionPrepared} {
		t.Run(string(state), func(t *testing.T) {
			f, _ := newLegacyCompletedIdentityFixture(t)
			inheritLegacyCompletionCancellation(t, f)
			_, err := f.repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{
				ID: "prompt:successor", SessionID: f.row.ID, IncarnationID: f.row.TeardownIntent.Target.SessionIncarnation,
				HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "successor", Payload: []byte("new instruction"), State: state,
			})
			require.NoError(t, err)
			f.svc.repo = &legacyCompletedIdentityRepo{Repository: f.repo, decisionErr: errors.New("receipt must not replace a selected submission")}
			outcome, err := NewErrandAdapter(f.svc).RetryErrandTeardown(t.Context(), *f.row.TeardownIntent)
			if state == models.DeliverySubmissionPrepared {
				require.ErrorContains(t, err, "errand submission admission remains unresolved")
				requireLegacyIdentityRefused(t, f)
				return
			}
			require.NoError(t, err)
			require.Equal(t, errands.TeardownSuperseded, outcome)
			require.Empty(t, f.provider.stops)
			row, err := f.repo.GetErrand(t.Context(), f.row.ID)
			require.NoError(t, err)
			require.Equal(t, models.ErrandTeardownIntentSuperseded, row.TeardownIntent.State)
			require.Equal(t, models.ErrandLifecycleCancelled, row.Lifecycle)
			_, alive := f.provider.GetExecution("exec-1")
			require.True(t, alive)
		})
	}
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.1 AC-TASKS-ERRAND-COMPLETION-001.3
func TestErrandTeardownLegacyCompletedIdentityCancellationInheritsTarget(t *testing.T) {
	f, service := newLegacyCompletedIdentityFixture(t)
	target := f.row.TeardownIntent.Target
	f.provider.beforeStop = func(ctx context.Context, executionID string) error {
		row, err := f.repo.GetErrand(ctx, f.row.ID)
		if err != nil {
			return err
		}
		require.Equal(t, target.ExecutionID, executionID)
		require.Equal(t, models.ErrandLifecycleCancelled, row.Lifecycle)
		require.Equal(t, models.ErrandTeardownPurposeCancellation, row.TeardownIntent.Purpose)
		require.Equal(t, target, row.TeardownIntent.Target)
		return nil
	}
	request := errands.CancelRequest{ErrandID: f.row.ID, TaskID: f.row.TaskID,
		ParentSessionID: f.row.ParentSessionID, Source: models.ErrandCancelSourceUser}
	_, err := service.Cancel(t.Context(), request)
	require.NoError(t, err)
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleCancelled, row.Lifecycle)
	require.Equal(t, models.ErrandCancelSourceUser, row.CancelSource)
	require.Equal(t, models.ErrandResultCancelled, row.ResultKind)
	require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
	require.Equal(t, target, row.TeardownIntent.Target)
	session, err := f.repo.GetTaskSession(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, session.State)
	require.Equal(t, []string{"exec-1"}, f.provider.stops)
	_, alive := f.provider.GetExecution("exec-1")
	require.False(t, alive)
	_, err = service.Cancel(t.Context(), request)
	require.NoError(t, err)
	require.NoError(t, service.RetryErrandCompletion(t.Context(), f.row.ID))
	after, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, row, after, "cleanup retries cannot restore success or replace the cancellation result")
	require.Equal(t, []string{"exec-1"}, f.provider.stops, "the inherited target closes exactly once")
}

func inheritLegacyCompletionCancellation(t *testing.T, f *errandTeardownFixture) {
	t.Helper()
	f.row.Lifecycle, f.row.CancelSource = models.ErrandLifecycleCancelled, models.ErrandCancelSourceUser
	f.row.TeardownIntent = &models.ErrandTeardownIntent{Version: models.ErrandTeardownIntentVersion,
		ID: "inherited-cancellation", State: models.ErrandTeardownIntentPending, Purpose: models.ErrandTeardownPurposeCancellation,
		RequestedAt: time.Now().UTC(), Target: f.row.TeardownIntent.Target}
	require.NoError(t, f.repo.UpdateErrand(t.Context(), f.row))
}

func requireLegacyIdentityRefused(t *testing.T, f *errandTeardownFixture) {
	t.Helper()
	require.Empty(t, f.provider.stops)
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, f.row, row)
	require.Equal(t, models.ErrandTeardownIntentPending, row.TeardownIntent.State)
	session, err := f.repo.GetTaskSession(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.NotEqual(t, models.TaskSessionStateCompleted, session.State)
	_, alive := f.provider.GetExecution("exec-1")
	require.True(t, alive)
}
