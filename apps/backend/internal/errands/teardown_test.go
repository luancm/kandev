package errands

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// The service fake implements the complete capability; runtime identity and
// admission arbitration are exercised by the production-adapter integration.
func (s *fakeStopper) CancelErrandWithTarget(ctx context.Context, row *models.Errand, commit func(ErrandTeardownTarget) error) error {
	target := ErrandTeardownTarget{TaskID: row.TaskID, SessionID: row.ID}
	_, err := s.store.GetTaskSession(ctx, row.ID)
	switch {
	case errors.Is(err, models.ErrTaskSessionNotFound):
		target.FirstTurnID, target.ReservedSubmissionID = row.FirstTurnID, row.SubmissionID
	case err != nil:
		return err
	default:
		target.SessionIncarnation, target.ExecutionID, target.NoTurn = "test-incarnation", row.ID, true
	}
	if err := commit(target); err != nil {
		return err
	}
	current, err := s.store.GetErrand(ctx, row.ID)
	if err != nil {
		return err
	}
	_, err = s.RetryErrandTeardown(ctx, *current.TeardownIntent)
	return err
}

func (s *fakeStopper) RetryErrandTeardown(ctx context.Context, intent ErrandTeardownIntent) (TeardownOutcome, error) {
	row, err := s.store.GetErrand(ctx, intent.Target.SessionID)
	if err != nil {
		return "", err
	}
	if row.TeardownIntent.ID != intent.ID {
		return "", models.ErrErrandVersionConflict
	}
	if row.TeardownIntent.State == models.ErrandTeardownIntentCompleted {
		return TeardownCompleted, nil
	}
	if intent.Target.ExecutionID != "" {
		if err := s.StopErrandSession(ctx, intent.Target.SessionID, StopReasonErrandCancelled); err != nil {
			return "", err
		}
	}
	_, err = s.store.TransitionErrandTeardownIntent(ctx, row.ID, intent.ID, row.Version, models.ErrandTeardownIntentCompleted)
	return TeardownCompleted, err
}

func (s *memoryStore) TransitionErrandTeardownIntent(ctx context.Context, id, intentID string, version int64, state models.ErrandTeardownIntentState) (*models.Errand, error) {
	row, err := s.GetErrand(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.Version != version || row.TeardownIntent == nil || row.TeardownIntent.ID != intentID || row.TeardownIntent.State != models.ErrandTeardownIntentPending {
		return nil, models.ErrErrandVersionConflict
	}
	intent := *row.TeardownIntent
	intent.State = state
	row.TeardownIntent = &intent
	return row, s.UpdateErrand(ctx, row)
}

type teardownProjectionFailureStore struct {
	Store
	broken string
	fault  error
}

type legacyOnlyStopper struct{ SessionStopper }

func TestTeardownRequiresIdentityAwareProductionWiring(t *testing.T) {
	h := newHarness(t)
	_, err := New(Dependencies{Store: h.store, Launcher: h.launcher,
		Stopper: legacyOnlyStopper{h.stopper}, Profiles: h.profiles, MCP: h.mcp, Events: h.publisher})
	require.ErrorContains(t, err, "identity-aware")
}

func (s teardownProjectionFailureStore) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	if id == s.broken {
		return nil, s.fault
	}
	return s.Store.GetTaskSession(ctx, id)
}

func TestStopAllDoesNotCountTeardownRetriesAsNewCancellations(t *testing.T) {
	h := newHarness(t)
	first, second := h.mustStart("first").Errand, h.mustStart("second").Errand
	h.stopper.err = errors.New("physical stop unavailable")
	count, err := h.service.CancelAll(t.Context(), testTask)
	require.Error(t, err)
	require.Equal(t, 2, count)
	require.Equal(t, []string{testParent}, h.deliverer.delivered())
	one, err := h.store.GetErrand(t.Context(), first.ID)
	require.NoError(t, err)
	two, err := h.store.GetErrand(t.Context(), second.ID)
	require.NoError(t, err)
	h.stopper.err = nil
	count, err = h.service.CancelAll(t.Context(), testTask)
	require.NoError(t, err)
	require.Zero(t, count)
	require.Equal(t, []string{testParent}, h.deliverer.delivered())
	got, err := h.store.GetErrand(t.Context(), first.ID)
	require.NoError(t, err)
	require.Equal(t, one.TeardownIntent.ID, got.TeardownIntent.ID)
	require.Equal(t, one.TeardownIntent.Target, got.TeardownIntent.Target)
	require.Equal(t, one.ResultText, got.ResultText)
	got, err = h.store.GetErrand(t.Context(), second.ID)
	require.NoError(t, err)
	require.Equal(t, two.TeardownIntent.ID, got.TeardownIntent.ID)
}

func TestCancellationProjectionFailureDoesNotHidePendingTeardownOrSiblings(t *testing.T) {
	for _, entry := range []string{"all", "parent", "task"} {
		t.Run(entry, func(t *testing.T) {
			h := newHarness(t)
			pending := h.mustStart("pending").Errand
			broken := h.mustStart("broken").Errand
			sibling := h.mustStart("sibling").Errand
			h.stopper.err = errors.New("stop fails")
			_, err := h.service.Cancel(t.Context(), cancelRequest(pending, models.ErrandCancelSourceUser))
			require.Error(t, err)
			h.stopper.err = nil
			fault := errors.New("projection unavailable")
			h.service.store = teardownProjectionFailureStore{Store: h.store, broken: broken.ID, fault: fault}
			var count int
			switch entry {
			case "all":
				count, err = h.service.CancelAll(t.Context(), testTask)
			case "parent":
				count, err = h.service.CancelForParent(t.Context(), testParent)
			case "task":
				count, err = h.service.CancelForTask(t.Context(), testTask)
			}
			require.ErrorIs(t, err, fault)
			require.Equal(t, 1, count)
			row, err := h.store.GetErrand(t.Context(), pending.ID)
			require.NoError(t, err)
			require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
			row, err = h.store.GetErrand(t.Context(), sibling.ID)
			require.NoError(t, err)
			require.Equal(t, models.ErrandLifecycleCancelled, row.Lifecycle)
		})
	}
}

func TestTeardownSweepBoundsAttemptsAndRotatesPastFailures(t *testing.T) {
	h := newHarness(t)
	h.stopper.err = errors.New("provider unavailable")
	for i := 0; i < 35; i++ {
		id := fmt.Sprintf("job-%03d", i)
		row := &models.Errand{ID: id, TaskID: testTask, ParentSessionID: testParent, FirstTurnID: "first-" + id,
			SubmissionID: "submission-" + id, Lifecycle: models.ErrandLifecycleLaunched, ResultState: models.ErrandResultNone}
		require.NoError(t, h.store.CreateErrand(t.Context(), row))
		h.store.addSession(&models.TaskSession{ID: id, TaskID: testTask, Role: models.SessionRoleErrand,
			ParentSessionID: testParent, State: models.TaskSessionStateRunning})
		_, err := h.service.Cancel(t.Context(), cancelRequest(row, models.ErrandCancelSourceUser))
		require.Error(t, err)
	}
	before := len(h.stopper.stops())
	require.Error(t, h.service.SweepDelivery(t.Context()))
	require.Equal(t, 32, len(h.stopper.stops())-before, "one sweep has a bounded attempt budget")
	h.stopper.err = nil
	require.NoError(t, h.service.SweepDelivery(t.Context()))
	last, err := h.store.GetErrand(t.Context(), "job-034")
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentCompleted, last.TeardownIntent.State, "failing early rows cannot starve later rows")
	require.NoError(t, h.service.SweepDelivery(t.Context()))
	rows, err := h.store.ListErrands(t.Context())
	require.NoError(t, err)
	for _, row := range rows {
		require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
	}
}

func TestCancelOfDoneErrandWithoutPendingTeardownIsNoOp(t *testing.T) {
	h := newHarness(t)
	row := h.mustStart("done").Errand
	h.store.setSessionState(row.ID, models.TaskSessionStateWaitingForInput)
	_, _, err := h.service.update(t.Context(), row.ID, func(row *models.Errand) bool {
		row.AwaitingResult, row.LastOutcome = false, models.ErrandOutcomeDone
		return true
	})
	require.NoError(t, err)
	before, err := h.store.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	snapshot, err := h.service.Cancel(t.Context(), cancelRequest(row, models.ErrandCancelSourceUser))
	require.NoError(t, err)
	require.Equal(t, models.ErrandStatusDone, snapshot.Status)
	after, err := h.store.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Empty(t, h.stopper.stops())
}

// Receipt completion is the only allowed concurrent write in stale-settlement
// assertions. Identity, result and all other row metadata must remain exact.
func expectedAfterTeardownReceipt(before, after *models.Errand) *models.Errand {
	expected := *before
	if before.TeardownIntent != nil && after.TeardownIntent != nil &&
		before.TeardownIntent.State == models.ErrandTeardownIntentPending && after.TeardownIntent.State == models.ErrandTeardownIntentCompleted {
		intent := *before.TeardownIntent
		intent.State = models.ErrandTeardownIntentCompleted
		expected.TeardownIntent = &intent
		expected.Version++
	}
	return &expected
}
