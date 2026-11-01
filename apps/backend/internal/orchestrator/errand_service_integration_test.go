package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type errandProfileStub struct{}

func (errandProfileStub) CompatibleProfiles(context.Context, string) ([]errands.ProfileInfo, error) {
	return nil, nil
}

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

type lifecycleDeliveryRecorder struct {
	errands.ParentDeliverer
	parents []string
}

func (r *lifecycleDeliveryRecorder) DeliverPending(ctx context.Context, parent string) {
	r.parents = append(r.parents, parent)
	r.ParentDeliverer.DeliverPending(ctx, parent)
}
func (r *lifecycleDeliveryRecorder) AdmitPrepared(ctx context.Context, parent, content string, metadata map[string]interface{}) error {
	r.parents = append(r.parents, parent)
	return r.ParentDeliverer.AdmitPrepared(ctx, parent, content, metadata)
}

// Recovery is callable without starting either the orchestrator or its worker.
func newLifecycleService(t *testing.T, f *errandLaunchFixture, store errands.Store) (*errands.Service, *lifecycleDeliveryRecorder) {
	t.Helper()
	adapter := NewErrandAdapter(f.svc)
	if f.svc.errands == nil {
		// This wrapper borrows the fixture's existing pool; the fixture owns Close.
		pool := sqlx.NewDb(f.repo.DB(), "sqlite3")
		queueRepo, err := messagequeue.NewSQLiteRepository(pool, pool)
		require.NoError(t, err)
		f.svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
		require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), f.parent, false))
	}
	delivery := &lifecycleDeliveryRecorder{ParentDeliverer: NewErrandParentDeliverer(f.svc)}
	service, err := errands.New(errands.Dependencies{Store: store, Launcher: adapter, Stopper: adapter,
		MCP: adapter, Profiles: errandProfileStub{}, Events: &errandEventRecorder{}, Deliverer: delivery, Logger: testLogger()})
	require.NoError(t, err)
	f.svc.SetErrandController(service)
	return service, delivery
}

func fillLifecycleCeiling(t *testing.T, f *errandLaunchFixture) func() {
	t.Helper()
	f.svc.sessionCeiling = newSessionCeilingController(2, nil, nil)
	holders := []string{"ceiling-holder-1", "ceiling-holder-2"}
	for _, id := range holders {
		require.True(t, f.svc.sessionCeiling.admit(t.Context(), admissionRequest{
			taskID: id, sessionID: id, origin: launchOriginAutomatic, seam: "test-setup",
		}).admitted)
	}
	return func() {
		for _, id := range holders {
			f.svc.sessionCeiling.release(id)
		}
	}
}

func requireNoLifecycleNotification(t *testing.T, f *errandLaunchFixture, parentID string) {
	t.Helper()
	var count int
	err := f.repo.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM queued_messages WHERE session_id = ?`, parentID).Scan(&count)
	require.NoError(t, err)
	require.Zero(t, count, "dead owners receive no persisted notification")
}

func seedLifecycleQueued(t *testing.T, repo *sqliterepo.Repository, taskID, parentID, id string) *models.Errand {
	t.Helper()
	row := &models.Errand{ID: id, TaskID: taskID, ParentSessionID: parentID, Title: id, Objective: "do " + id,
		AgentProfileID: "errand-profile", FirstTurnID: "turn-" + id, SubmissionID: "submission-" + id,
		Lifecycle: models.ErrandLifecycleQueued, AwaitingResult: true, ResultState: models.ErrandResultNone}
	require.NoError(t, repo.CreateErrand(t.Context(), row))
	return row
}

func seedLifecycleHistory(t *testing.T, f *errandLaunchFixture, id string, state models.ErrandResultState) *models.Errand {
	t.Helper()
	seedOrdinarySession(t, f.repo, f.taskID, id, models.TaskSessionStateWaitingForInput, -time.Minute)
	markSessionErrand(t, f.repo, id, f.parent)
	row := seedLifecycleQueued(t, f.repo, f.taskID, f.parent, id)
	row.Lifecycle, row.AwaitingResult, row.LastOutcome = models.ErrandLifecycleLaunched, false, models.ErrandOutcomeDone
	row.ResultState, row.ResultKind, row.ResultText, row.ResultTurnID = state, models.ErrandResultOutput, "saved result "+id, row.FirstTurnID
	if state == models.ErrandResultDelivered {
		now := time.Now().UTC()
		row.DeliveredRoute, row.DeliveryToken, row.DeliveredAt = models.ErrandDeliveryNotification, "delivered-token", &now
	}
	require.NoError(t, f.repo.UpdateErrand(t.Context(), row))
	return row
}

func persistLifecycleLoss(t *testing.T, f *errandLaunchFixture, loss string) {
	t.Helper()
	var err error
	switch loss {
	case "archived task":
		_, err = f.repo.DB().ExecContext(t.Context(), `UPDATE tasks SET archived_at = ? WHERE id = ?`, time.Now().UTC(), f.taskID)
	case "missing parent":
		_, err = f.repo.DB().ExecContext(t.Context(), `DELETE FROM task_sessions WHERE id = ?`, f.parent)
	case "wrong task":
		_, err = f.repo.DB().ExecContext(t.Context(), `UPDATE task_sessions SET task_id = 'task-2' WHERE id = ?`, f.parent)
	case "errand parent":
		markSessionErrand(t, f.repo, f.parent, "other-parent")
	default:
		_, err = f.repo.DB().ExecContext(t.Context(), `UPDATE task_sessions SET state = ? WHERE id = ?`, loss, f.parent)
	}
	require.NoError(t, err)
}

// @covers AC-TASKS-ERRANDS-007.2 AC-TASKS-ERRANDS-007.4 AC-TASKS-ERRANDS-008.2 AC-TASKS-ERRANDS-012.3
func TestErrandLifecycleRecoveryCancelsLostCascadeBeforeDrain(t *testing.T) {
	for _, loss := range []string{"FAILED", "CANCELLED", "COMPLETED", "missing parent", "archived task", "wrong task", "errand parent"} {
		for _, enabled := range []bool{true, false} {
			label := "enabled"
			if !enabled {
				label = "disabled"
			}
			t.Run(loss+"/"+label, func(t *testing.T) {
				f := newErrandLaunchFixture(t)
				original, _ := newLifecycleService(t, f, f.repo)
				launched := startErrand(t, original, f, "launched before crash").Errand
				releaseCapacity := fillLifecycleCeiling(t, f)
				queued := seedLifecycleQueued(t, f.repo, f.taskID, f.parent, "lost-queued")
				pending := seedLifecycleHistory(t, f, "pending-history", models.ErrandResultPending)
				delivered := seedLifecycleHistory(t, f, "delivered-history", models.ErrandResultDelivered)
				delivered, err := f.repo.GetErrand(t.Context(), delivered.ID)
				require.NoError(t, err)
				seedLifecycleOtherTask(t, f)
				other := seedLifecycleQueued(t, f.repo, "task-2", "live-other-task", "live-other-child")
				var sibling *models.Errand
				if loss != "archived task" {
					seedOrdinarySession(t, f.repo, f.taskID, "live-sibling-parent", models.TaskSessionStateWaitingForInput, -time.Minute)
					sibling = seedLifecycleQueued(t, f.repo, f.taskID, "live-sibling-parent", "live-sibling-child")
				}
				persistLifecycleLoss(t, f, loss) // No event or in-memory cascade job survives.
				fresh, notifications := newLifecycleService(t, f, f.repo)
				fresh.SetEnabled(func() bool { return enabled })
				require.NoError(t, fresh.Reconcile(t.Context()))
				recovered, err := f.repo.GetErrand(t.Context(), queued.ID)
				require.NoError(t, err)
				require.Equal(t, models.ErrandLifecycleCancelled, recovered.Lifecycle, "startup recovery wins before capacity release or a queue scan")
				releaseCapacity()
				f.svc.ceilingSweepTick(t.Context())

				for _, row := range []*models.Errand{queued, launched} {
					got, err := f.repo.GetErrand(t.Context(), row.ID)
					require.NoError(t, err)
					require.Equal(t, models.ErrandLifecycleCancelled, got.Lifecycle, row.ID)
					require.Equal(t, models.ErrandCancelSourceCascade, got.CancelSource)
					require.NotNil(t, got.TeardownIntent)
					require.Equal(t, models.ErrandTeardownIntentCompleted, got.TeardownIntent.State)
				}
				_, err = f.repo.GetTaskSession(t.Context(), queued.ID)
				require.ErrorIs(t, err, models.ErrTaskSessionNotFound)
				_, running := f.manager.GetExecution("execution-" + launched.ID)
				require.False(t, running, "the exact captured runtime was stopped")
				got, err := f.repo.GetErrand(t.Context(), pending.ID)
				require.NoError(t, err)
				require.Equal(t, models.ErrandResultDiscarded, got.ResultState)
				got, err = f.repo.GetErrand(t.Context(), delivered.ID)
				require.NoError(t, err)
				require.Equal(t, delivered, got, "completed delivered history is immutable")
				require.NotContains(t, notifications.parents, f.parent)
				requireNoLifecycleNotification(t, f, f.parent)
				for _, live := range []*models.Errand{other, sibling} {
					if live == nil {
						continue
					}
					got, err := f.repo.GetErrand(t.Context(), live.ID)
					require.NoError(t, err)
					require.Equal(t, models.ErrandLifecycleLaunched, got.Lifecycle, "verified live parents still drain with either flag setting")
				}
			})
		}
	}
}

func seedLifecycleOtherTask(t *testing.T, f *errandLaunchFixture) {
	t.Helper()
	task, err := f.repo.GetTask(t.Context(), f.taskID)
	require.NoError(t, err)
	task.ID = "task-2"
	require.NoError(t, f.repo.CreateTask(t.Context(), task))
	seedOrdinarySession(t, f.repo, task.ID, "live-other-task", models.TaskSessionStateWaitingForInput, -time.Minute)
	otherTask := *f.taskRepo.tasks[f.taskID]
	otherTask.ID = task.ID
	f.taskRepo.tasks[task.ID] = &otherTask
}

type lifecycleReadStore struct {
	*sqliterepo.Repository
	taskFault, parentFault string
	fault                  error
	afterList              func()
}

func (r *lifecycleReadStore) GetTask(ctx context.Context, id string) (*models.Task, error) {
	if id == r.taskFault {
		return nil, r.fault
	}
	return r.Repository.GetTask(ctx, id)
}

func (r *lifecycleReadStore) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	if id == r.parentFault {
		return nil, r.fault
	}
	return r.Repository.GetTaskSession(ctx, id)
}

func (r *lifecycleReadStore) ListQueuedErrands(ctx context.Context) ([]*models.Errand, error) {
	rows, err := r.Repository.ListQueuedErrands(ctx)
	if r.afterList != nil {
		r.afterList()
	}
	return rows, err
}

// @covers AC-TASKS-ERRANDS-008.2 AC-TASKS-ERRANDS-008.3
func TestErrandLifecycleReadErrorBlocksOnlyAffectedTask(t *testing.T) {
	for _, boundary := range []string{"task", "parent"} {
		t.Run(boundary, func(t *testing.T) {
			f := newErrandLaunchFixture(t)
			pending := seedLifecycleHistory(t, f, "saved-pending", models.ErrandResultPending)
			first := seedLifecycleQueued(t, f.repo, f.taskID, f.parent, "first-unreadable")
			seedOrdinarySession(t, f.repo, f.taskID, "readable-sibling", models.TaskSessionStateWaitingForInput, -time.Minute)
			second := seedLifecycleQueued(t, f.repo, f.taskID, "readable-sibling", "second-readable")
			seedLifecycleOtherTask(t, f)
			other := seedLifecycleQueued(t, f.repo, "task-2", "live-other-task", "other-task")
			fault := errors.New("lifecycle read unavailable")
			store := &lifecycleReadStore{Repository: f.repo, fault: fault}
			if boundary == "task" {
				store.taskFault = f.taskID
			} else {
				store.parentFault = f.parent
			}
			service, delivery := newLifecycleService(t, f, store)
			before, err := f.repo.ListErrandsByTask(t.Context(), f.taskID)
			require.NoError(t, err)
			require.ErrorIs(t, service.Reconcile(t.Context()), fault)
			f.svc.ceilingSweepTick(t.Context())
			after, err := f.repo.ListErrandsByTask(t.Context(), f.taskID)
			require.NoError(t, err)
			require.Equal(t, before, after, "read errors change neither results nor queued rows, and cannot overtake the first reservation")
			require.Empty(t, delivery.parents)
			requireNoLifecycleNotification(t, f, f.parent)
			got, err := f.repo.GetErrand(t.Context(), other.ID)
			require.NoError(t, err)
			require.Equal(t, models.ErrandLifecycleLaunched, got.Lifecycle, "independent verified tasks continue")
			store.taskFault, store.parentFault = "", ""
			require.NoError(t, service.Reconcile(t.Context()))
			f.svc.ceilingSweepTick(t.Context())
			for _, row := range []*models.Errand{first, second} {
				got, err := f.repo.GetErrand(t.Context(), row.ID)
				require.NoError(t, err)
				require.Equal(t, models.ErrandLifecycleLaunched, got.Lifecycle)
			}
			require.Contains(t, delivery.parents, pending.ParentSessionID, "delivery is reachable after authoritative reads recover")
			status := f.svc.messageQueue.GetStatus(t.Context(), pending.ParentSessionID)
			require.NotNil(t, status)
			require.Equal(t, 1, status.Count, "the production deliverer actually persists the notification")
		})
	}
}

func TestErrandAdmissionRechecksParentAfterQueueScan(t *testing.T) {
	f := newErrandLaunchFixture(t)
	queued := seedLifecycleQueued(t, f.repo, f.taskID, f.parent, "scanned-child")
	seedOrdinarySession(t, f.repo, f.taskID, "live-sibling", models.TaskSessionStateWaitingForInput, -time.Minute)
	live := seedLifecycleQueued(t, f.repo, f.taskID, "live-sibling", "next-child")
	reached, allow, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	store := &lifecycleReadStore{Repository: f.repo, afterList: func() { close(reached); <-allow }}
	service, delivery := newLifecycleService(t, f, store)
	var once sync.Once
	release := func() { once.Do(func() { close(allow) }) }
	go func() { defer close(done); service.DrainQueued(t.Context()) }()
	t.Cleanup(func() { release(); coordinatorStopAwaitSignal(t, done, "queue scan cleanup") })
	coordinatorStopAwaitSignal(t, reached, "queue snapshot")
	persistLifecycleLoss(t, f, "FAILED")
	release()
	coordinatorStopAwaitSignal(t, done, "ownership recheck")
	got, err := f.repo.GetErrand(t.Context(), queued.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleCancelled, got.Lifecycle)
	_, err = f.repo.GetTaskSession(t.Context(), queued.ID)
	require.ErrorIs(t, err, models.ErrTaskSessionNotFound)
	got, err = f.repo.GetErrand(t.Context(), live.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleLaunched, got.Lifecycle, "a removed reservation permits the next eligible parent")
	require.Empty(t, delivery.parents)
}

func TestErrandLifecycleSweepRecoversWithoutAWorker(t *testing.T) {
	f := newErrandLaunchFixture(t)
	service, delivery := newLifecycleService(t, f, f.repo)
	launched := startErrand(t, service, f, "running").Errand
	queued := seedLifecycleQueued(t, f.repo, f.taskID, f.parent, "lost-sweep-child")
	pending := seedLifecycleHistory(t, f, "sweep-pending", models.ErrandResultPending)
	persistLifecycleLoss(t, f, "FAILED")
	f.svc.ceilingSweepTick(t.Context())
	for _, row := range []*models.Errand{queued, launched} {
		got, err := f.repo.GetErrand(t.Context(), row.ID)
		require.NoError(t, err)
		require.Equal(t, models.ErrandLifecycleCancelled, got.Lifecycle)
	}
	got, err := f.repo.GetErrand(t.Context(), pending.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultDiscarded, got.ResultState)
	require.Empty(t, delivery.parents)
	requireNoLifecycleNotification(t, f, f.parent)
}

// @covers AC-TASKS-ERRANDS-007.5
func TestErrandLifecycleParentTurnCancelIsNotTerminal(t *testing.T) {
	f := newErrandLaunchFixture(t)
	service, _ := newLifecycleService(t, f, f.repo)
	launched := startErrand(t, service, f, "unaffected child").Errand
	queued := seedLifecycleQueued(t, f.repo, f.taskID, f.parent, "unaffected-queue")
	// Ordinary turn cancellation settles the parent to WAITING_FOR_INPUT,
	// without requiring an execution or waiting_on_errands flag to retain children.
	persistLifecycleLoss(t, f, string(models.TaskSessionStateWaitingForInput))
	before, err := f.repo.GetErrand(t.Context(), launched.ID)
	require.NoError(t, err)
	require.NoError(t, service.Reconcile(t.Context()))
	f.svc.ceilingSweepTick(t.Context())
	after, err := f.repo.GetErrand(t.Context(), launched.ID)
	require.NoError(t, err)
	require.Equal(t, before, after)
	_, running := f.manager.GetExecution("execution-" + launched.ID)
	require.True(t, running)
	after, err = f.repo.GetErrand(t.Context(), queued.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleLaunched, after.Lifecycle)
}

func TestErrandAdmissionRechecksParentBeforeCreation(t *testing.T) {
	f := newErrandLaunchFixture(t)
	row := seedLifecycleQueued(t, f.repo, f.taskID, f.parent, "creation-child")
	service, delivery := newLifecycleService(t, f, f.repo)
	gate := &errandCreationGateRepo{Repository: f.repo, reached: make(chan struct{}), allow: make(chan struct{})}
	f.svc.repo = gate
	done := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate.allow) }) }
	go func() { defer close(done); service.DrainQueued(t.Context()) }()
	t.Cleanup(func() { release(); coordinatorStopAwaitSignal(t, done, "creation guard cleanup") })
	coordinatorStopAwaitSignal(t, gate.reached, "before session creation")
	persistLifecycleLoss(t, f, "FAILED")
	release()
	coordinatorStopAwaitSignal(t, done, "creation owner guard")
	_, err := f.repo.GetTaskSession(t.Context(), row.ID)
	require.ErrorIs(t, err, models.ErrTaskSessionNotFound)
	got, err := f.repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleCancelled, got.Lifecycle)
	require.Empty(t, f.manager.launchedWith)
	require.Empty(t, delivery.parents)
}

func TestErrandAdmissionRechecksParentBeforeRegistration(t *testing.T) {
	f := newErrandLaunchFixture(t)
	row := seedLifecycleQueued(t, f.repo, f.taskID, f.parent, "registration-child")
	service, delivery := newLifecycleService(t, f, f.repo)
	gate := &errandRegistrationGateRepo{Repository: f.repo, reached: make(chan struct{}), allow: make(chan struct{})}
	f.svc.repo = gate
	done := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate.allow) }) }
	go func() { defer close(done); service.DrainQueued(t.Context()) }()
	t.Cleanup(func() { release(); coordinatorStopAwaitSignal(t, done, "registration guard cleanup") })
	coordinatorStopAwaitSignal(t, gate.reached, "after creation before runtime allocation")
	persistLifecycleLoss(t, f, "CANCELLED")
	release()
	coordinatorStopAwaitSignal(t, done, "runtime registration owner guard")
	got, err := f.repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleCancelled, got.Lifecycle)
	require.Empty(t, f.manager.launchedWith)
	require.Empty(t, delivery.parents)
}

func TestErrandAdmissionRechecksParentBeforeDispatch(t *testing.T) {
	f := newErrandLaunchFixture(t)
	service, delivery := newLifecycleService(t, f, f.repo)
	row := startErrand(t, service, f, "dispatch-child").Errand
	f.manager.mu.Lock()
	before, accepted, failed := f.manager.initialPromptAdmissionCallback, f.manager.initialPromptDispatchCallback, f.manager.initialPromptFailureCallback
	f.manager.mu.Unlock()
	require.NotNil(t, before)
	t.Cleanup(failed)
	persistLifecycleLoss(t, f, "COMPLETED")
	require.Error(t, before(t.Context(), "initial payload", nil), "terminal owner refuses provider dispatch")
	accepted()
	require.NoError(t, service.Reconcile(t.Context()))
	got, err := f.repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleCancelled, got.Lifecycle)
	_, running := f.manager.GetExecution("execution-" + row.ID)
	require.False(t, running, "an allocated runtime is compensated by exact teardown")
	require.Empty(t, delivery.parents)
}

func TestErrandLifecycleRecoveryRetriesPartialStopFailure(t *testing.T) {
	f := newErrandLaunchFixture(t)
	service, delivery := newLifecycleService(t, f, f.repo)
	launched := startErrand(t, service, f, "stop-fails-once").Errand
	queued := seedLifecycleQueued(t, f.repo, f.taskID, f.parent, "cancel-sibling")
	pending := seedLifecycleHistory(t, f, "partial-pending", models.ErrandResultPending)
	persistLifecycleLoss(t, f, "FAILED")
	fault := errors.New("provider stop unavailable")
	stop := f.manager.stopAgentWithReasonFunc
	f.manager.stopAgentWithReasonFunc = func(context.Context, string, string, bool) error { return fault }
	require.ErrorIs(t, service.Reconcile(t.Context()), fault)
	got, err := f.repo.GetErrand(t.Context(), launched.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleCancelled, got.Lifecycle)
	require.Equal(t, models.ErrandTeardownIntentPending, got.TeardownIntent.State)
	intentID := got.TeardownIntent.ID
	got, err = f.repo.GetErrand(t.Context(), queued.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleCancelled, got.Lifecycle, "one failed physical stop cannot hide queued siblings")
	got, err = f.repo.GetErrand(t.Context(), pending.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultDiscarded, got.ResultState)
	f.manager.stopAgentWithReasonFunc = stop
	service.SetEnabled(func() bool { return false })
	f.svc.ceilingSweepTick(t.Context())
	got, err = f.repo.GetErrand(t.Context(), launched.ID)
	require.NoError(t, err)
	require.Equal(t, intentID, got.TeardownIntent.ID)
	require.Equal(t, models.ErrandTeardownIntentCompleted, got.TeardownIntent.State)
	_, running := f.manager.GetExecution("execution-" + launched.ID)
	require.False(t, running)
	require.Empty(t, delivery.parents)
}

// The lifecycle-only test controller implements the production capability.
func (*recordingErrandController) ReconcileLifecycle(context.Context) error { return nil }

type lifecycleOpaqueController struct {
	ErrandController
	deliveries int
}

func (c *lifecycleOpaqueController) SweepDelivery(context.Context) error {
	c.deliveries++
	return nil
}

func TestErrandLifecycleMissingCapabilityBlocksAdmissionAndDelivery(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	underlying := &recordingErrandController{}
	opaque := &lifecycleOpaqueController{ErrandController: underlying}
	svc.SetErrandController(opaque)
	svc.ceilingSweepTick(t.Context())
	require.Zero(t, underlying.drainCall)
	require.Zero(t, opaque.deliveries)
}
