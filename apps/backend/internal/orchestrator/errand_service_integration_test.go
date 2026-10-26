package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

type errandProfileStub struct{}

func (errandProfileStub) ResolveProfile(_ context.Context, _, selector, defaultProfileID string) (errands.ResolvedProfile, error) {
	id := selector
	if id == "" {
		id = defaultProfileID
	}
	return errands.ResolvedProfile{ID: "errand-profile", Name: "Errand profile (" + id + ")"}, nil
}

type errandEventRecorder struct {
	mu       sync.Mutex
	statuses []models.ErrandStatus
}

func (r *errandEventRecorder) PublishErrandUpdated(_ context.Context, _ *models.Errand, status models.ErrandStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses = append(r.statuses, status)
}

func (r *errandEventRecorder) seen() []models.ErrandStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]models.ErrandStatus(nil), r.statuses...)
}

func newErrandServiceOnRealStack(t *testing.T, f *errandLaunchFixture) (*errands.Service, *errandEventRecorder) {
	t.Helper()
	adapter := NewErrandAdapter(f.svc)
	recorder := &errandEventRecorder{}
	service, err := errands.New(errands.Dependencies{
		Store: f.repo, Launcher: adapter, Stopper: adapter, MCP: adapter,
		Profiles: errandProfileStub{}, Events: recorder, Logger: testLogger(),
	})
	require.NoError(t, err)
	f.svc.SetErrandController(service)
	service.StartWorkers(context.Background())
	t.Cleanup(service.StopWorkers)
	return service, recorder
}

func startErrand(t *testing.T, service *errands.Service, f *errandLaunchFixture, title string) *errands.StartResult {
	t.Helper()
	result, err := service.Start(context.Background(), errands.StartRequest{
		TaskID: f.taskID, ParentSessionID: f.parent, Title: title, Objective: "do " + title,
	})
	require.NoError(t, err)
	return result
}

// @covers AC-TASKS-ERRANDS-001.1 AC-TASKS-ERRANDS-007.1
func TestErrandServiceStartsAndCancelsAnErrandOnTheRealStack(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	service, recorder := newErrandServiceOnRealStack(t, f)

	result := startErrand(t, service, f, "audit")
	require.Equal(t, models.ErrandStatusRunning, result.Status)
	session, err := f.repo.GetTaskSession(ctx, result.Errand.ID)
	require.NoError(t, err)
	require.Equal(t, models.SessionRoleErrand, session.Role)
	require.False(t, session.IsPrimary)
	require.Equal(t, []models.ErrandStatus{models.ErrandStatusQueued, models.ErrandStatusRunning}, recorder.seen())

	snapshot, err := service.Cancel(ctx, errands.CancelRequest{
		ErrandID: result.Errand.ID, TaskID: f.taskID, Source: models.ErrandCancelSourceUser,
	})
	require.NoError(t, err)
	require.Equal(t, models.ErrandStatusCancelled, snapshot.Status)
	require.Eventually(t, func() bool {
		f.manager.mu.Lock()
		defer f.manager.mu.Unlock()
		return len(f.manager.stopAgentWithReasonArgs) > 0 &&
			f.manager.stopAgentWithReasonArgs[0].Reason == errands.StopReasonErrandCancelled
	}, 5*time.Second, 10*time.Millisecond, "the errand's runtime was stopped with the errand_cancelled reason")

	repeat, err := service.Cancel(ctx, errands.CancelRequest{
		ErrandID: result.Errand.ID, TaskID: f.taskID, Source: models.ErrandCancelSourceUser,
	})
	require.NoError(t, err)
	require.Equal(t, models.ErrandStatusCancelled, repeat.Status, "a repeated cancel succeeds without change")
	snapshots, err := service.List(ctx, f.taskID)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	require.Equal(t, models.ErrandStatusCancelled, snapshots[0].Status)
	transcriptKept, err := f.repo.GetTaskSession(ctx, result.Errand.ID)
	require.NoError(t, err)
	require.Equal(t, result.Errand.ID, transcriptKept.ID, "cancelling keeps the errand session and its transcript")
}

// @covers AC-TASKS-ERRANDS-008.1
func TestErrandServiceEnforcesTheLiveLimitAgainstTheRealRepository(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	service, _ := newErrandServiceOnRealStack(t, f)
	for i := 0; i < errands.MaxLiveErrands; i++ {
		startErrand(t, service, f, string(rune('a'+i)))
	}
	_, err := service.Start(ctx, errands.StartRequest{TaskID: f.taskID, ParentSessionID: f.parent, Title: "six", Objective: "six"})
	reason, typed := errands.ReasonOf(err)
	require.True(t, typed)
	require.Equal(t, errands.ReasonLimitReached, reason)
	sessions, err := f.repo.ListTaskSessions(ctx, f.taskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1+errands.MaxLiveErrands, "the refused start created no session")
}

// @covers AC-TASKS-ERRANDS-008.2
func TestErrandServiceQueuesAtTheCeilingAndTheSweepStartsItWhenCapacityFrees(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	service, _ := newErrandServiceOnRealStack(t, f)
	f.svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	require.True(t, f.svc.sessionCeiling.admit(ctx, admissionRequest{
		taskID: "occupier", sessionID: "occupier-session", origin: launchOriginAutomatic, seam: "test-setup",
	}).admitted)

	queued := startErrand(t, service, f, "waits")
	require.Equal(t, models.ErrandStatusQueued, queued.Status)
	task, err := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)
	require.NotContains(t, task.Metadata, models.MetaKeyDeferredLaunch)

	f.svc.ceilingSweepTick(ctx)
	row, err := f.repo.GetErrand(ctx, queued.Errand.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleQueued, row.Lifecycle, "no capacity yet")

	f.svc.sessionCeiling.release("occupier-session")
	f.svc.ceilingSweepTick(ctx)
	row, err = f.repo.GetErrand(ctx, queued.Errand.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleLaunched, row.Lifecycle)
	session, err := f.repo.GetTaskSession(ctx, queued.Errand.ID)
	require.NoError(t, err)
	require.Equal(t, models.SessionRoleErrand, session.Role)
}

// @covers AC-TASKS-ERRANDS-007.2
func TestStoppingTheParentCancelsItsErrandsThroughTheWorker(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	service, _ := newErrandServiceOnRealStack(t, f)
	result := startErrand(t, service, f, "audit")

	require.NoError(t, f.svc.StopSession(ctx, f.parent, "stopped by user", false))
	require.Eventually(t, func() bool {
		row, err := f.repo.GetErrand(ctx, result.Errand.ID)
		return err == nil && row.Lifecycle == models.ErrandLifecycleCancelled &&
			row.CancelSource == models.ErrandCancelSourceCascade
	}, 5*time.Second, 10*time.Millisecond)
}

// A user stop of the errand session through the generic stop path records a
// user cancellation first, so the errand reads as cancelled, not failed.
func TestGenericStopOfAnErrandSessionIsRecordedAsAUserCancellation(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	service, _ := newErrandServiceOnRealStack(t, f)
	result := startErrand(t, service, f, "audit")

	require.NoError(t, f.svc.StopSession(ctx, result.Errand.ID, "stopped by user", false))
	row, err := f.repo.GetErrand(ctx, result.Errand.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleCancelled, row.Lifecycle)
	require.Equal(t, models.ErrandCancelSourceUser, row.CancelSource)
	parent, err := f.repo.GetTaskSession(ctx, f.parent)
	require.NoError(t, err)
	require.NotEqual(t, models.TaskSessionStateCancelled, parent.State, "stopping an errand never stops its parent")
}
