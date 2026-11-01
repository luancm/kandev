package errands

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type lifecycleReadCounter struct {
	Store
	tasks, parents int
	taskMissing    bool
}

func (r *lifecycleReadCounter) GetTask(ctx context.Context, id string) (*models.Task, error) {
	r.tasks++
	if r.taskMissing {
		return nil, fmt.Errorf("removed: %w", repoerrors.ErrTaskNotFound)
	}
	return r.Store.GetTask(ctx, id)
}

func (r *lifecycleReadCounter) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	r.parents++
	return r.Store.GetTaskSession(ctx, id)
}

func TestLifecycleRecoveryReadsOwnersByGroup(t *testing.T) {
	h := newHarness(t)
	h.launcher.behave = func(int, LaunchRequest) error { return ErrCeilingRefused }
	first := h.mustStart("retained").Errand
	for i := range 100 {
		row := *first
		row.ID, row.FirstTurnID, row.SubmissionID = fmt.Sprint("history-", i), fmt.Sprint("turn-", i), fmt.Sprint("submission-", i)
		row.Lifecycle, row.AwaitingResult, row.LastOutcome = models.ErrandLifecycleLaunched, false, models.ErrandOutcomeDone
		row.ResultState, row.ResultKind, row.ResultText = models.ErrandResultDelivered, models.ErrandResultOutput, "saved history"
		h.store.addSession(&models.TaskSession{ID: row.ID, TaskID: testTask, ParentSessionID: testParent,
			Role: models.SessionRoleErrand, State: models.TaskSessionStateWaitingForInput})
		require.NoError(t, h.store.CreateErrand(t.Context(), &row))
	}
	reads := &lifecycleReadCounter{Store: h.store}
	h.service.store = reads
	require.NoError(t, h.service.ReconcileLifecycle(t.Context()))
	require.Equal(t, 1, reads.tasks)
	require.Equal(t, 1, reads.parents)
	require.Empty(t, h.stopper.stops())
}

func TestLifecycleRecoveryMissingTaskCancelsQueuedReservation(t *testing.T) {
	h := newHarness(t)
	h.launcher.behave = func(int, LaunchRequest) error { return ErrCeilingRefused }
	row := h.mustStart("lost task").Errand
	h.service.store = &lifecycleReadCounter{Store: h.store, taskMissing: true}
	require.NoError(t, h.service.ReconcileLifecycle(t.Context()))
	got, err := h.store.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleCancelled, got.Lifecycle)
	require.Equal(t, models.ErrandCancelSourceCascade, got.CancelSource)
	require.Empty(t, h.deliverer.delivered())
}

func TestLifecycleRecoveryLeavesPendingTeardownToBoundedOwner(t *testing.T) {
	h := newHarness(t)
	row := h.mustStart("pending physical stop").Errand
	h.stopper.err = errors.New("physical stop failed")
	_, err := h.service.Cancel(t.Context(), cancelRequest(row, models.ErrandCancelSourceUser))
	require.Error(t, err)
	before := len(h.stopper.stops())
	h.store.setSessionState(testParent, models.TaskSessionStateCompleted)
	require.NoError(t, h.service.ReconcileLifecycle(t.Context()))
	require.Len(t, h.stopper.stops(), before, "the lifecycle pass must not bypass Task10's bounded retry sweep")
	got, err := h.store.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultDiscarded, got.ResultState)
	h.stopper.err = nil
	require.NoError(t, h.service.SweepDelivery(t.Context()))
	got, err = h.store.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentCompleted, got.TeardownIntent.State)
}
