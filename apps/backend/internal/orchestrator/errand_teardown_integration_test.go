package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func (m *teardownProvider) RowLiveness(row *models.ExecutorRunning) models.ProcessLiveness {
	if m.rowLivenessFn != nil {
		return m.rowLivenessFn(row)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lookupErr != nil {
		return models.ProcessLivenessUnknown
	}
	if m.runtimes[row.AgentExecutionID] != nil {
		return models.ProcessLivenessAlive
	}
	return models.ProcessLivenessDead
}

type teardownIntegrationRepo struct {
	*sqliterepo.Repository
	afterCancel func()
	sessionErr  error
}

func (r *teardownIntegrationRepo) UpdateErrand(ctx context.Context, row *models.Errand) error {
	if err := r.Repository.UpdateErrand(ctx, row); err != nil {
		return err
	}
	if row.Lifecycle == models.ErrandLifecycleCancelled && r.afterCancel != nil {
		r.afterCancel()
	}
	return nil
}

func (r *teardownIntegrationRepo) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	if r.sessionErr != nil && id == "s1" {
		return nil, r.sessionErr
	}
	return r.Repository.GetTaskSession(ctx, id)
}

func teardownIntegrationService(t *testing.T, f *errandTeardownFixture, store errands.Store) (*errands.Service, *errandEventRecorder) {
	t.Helper()
	return teardownIntegrationServiceWithLogger(t, f, store, testLogger())
}

func teardownIntegrationServiceWithLogger(t *testing.T, f *errandTeardownFixture, store errands.Store, log *logger.Logger) (*errands.Service, *errandEventRecorder) {
	t.Helper()
	adapter := NewErrandAdapter(f.svc)
	recorder := &errandEventRecorder{}
	service, err := errands.New(errands.Dependencies{
		Store: store, Launcher: adapter, Stopper: adapter, MCP: adapter,
		Profiles: errandProfileStub{}, Events: recorder, Logger: log,
	})
	require.NoError(t, err)
	f.svc.SetErrandController(service)
	return service, recorder
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.4
func TestErrandLegacyAdmissionRefusalLogsOnceAndRetriesChangedEvidence(t *testing.T) {
	f := newErrandTeardownFixture(t)
	now := time.Now().UTC()
	f.row.Lifecycle, f.row.CancelSource, f.row.SettledAt = models.ErrandLifecycleCancelled, models.ErrandCancelSourceUser, &now
	require.NoError(t, f.repo.UpdateErrand(t.Context(), f.row))

	core, logs := observer.New(zapcore.DebugLevel)
	zapLogger := zap.New(core)
	wrapped, err := logger.NewFromZap(zapLogger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = zapLogger.Sync() })
	service, _ := teardownIntegrationServiceWithLogger(t, f, f.repo, wrapped)

	require.ErrorContains(t, service.Reconcile(t.Context()), "legacy errand lacks positive initial admission authority")
	require.NoError(t, service.Reconcile(t.Context()), "unchanged evidence keeps retrying without repeating the diagnostic")
	require.Len(t, logs.FilterMessage("errand teardown remains pending").All(), 1)
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Nil(t, row.TeardownIntent, "unknown admission remains protected")
	require.Empty(t, f.provider.stops)
	assertChangedAdmissionRefusals(t, f, service, logs, "submission_id", "sender_session_id", "state")

	require.NoError(t, f.svc.turnService.PatchTurnMetadata(t.Context(), f.row.ID, f.row.FirstTurnID, map[string]interface{}{
		errandAdmissionTurnMetadataKey: map[string]interface{}{"submission_id": f.row.SubmissionID,
			"sender_session_id": f.row.ParentSessionID, "state": string(models.ErrandAdmissionAccepted)},
	}))
	require.NoError(t, service.Reconcile(t.Context()), "new exact admission evidence makes the retry eligible")
	row, err = f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
	require.Equal(t, []string{"exec-1"}, f.provider.stops)
}

func TestErrandPendingReservedAdmissionDiagnosticLogsOnceAndRetriesChangedEvidence(t *testing.T) {
	f := newErrandTeardownFixture(t)
	f.row = &models.Errand{ID: "reserved", TaskID: f.row.TaskID, ParentSessionID: f.row.ParentSessionID,
		Title: "Reserved cleanup", Objective: "Recover the reserved launch", AgentProfileID: "profile",
		FirstTurnID: "reserved-turn", SubmissionID: "prompt:reserved", Lifecycle: models.ErrandLifecycleQueued,
		ResultState: models.ErrandResultNone}
	require.NoError(t, f.repo.CreateErrand(t.Context(), f.row))
	intent := f.pending(t)
	require.Equal(t, f.row.FirstTurnID, intent.Target.FirstTurnID)

	seedErrandSession(t, f.repo, f.row.TaskID, f.row.ParentSessionID, f.row.ID, models.TaskSessionStateRunning)
	session, err := f.repo.GetTaskSession(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.NoError(t, f.repo.CreateTurn(t.Context(), &models.Turn{ID: f.row.FirstTurnID, TaskID: f.row.TaskID,
		TaskSessionID: f.row.ID, StartedAt: time.Now().UTC()}))
	seedExecutorRunning(t, f.repo, f.row.ID, f.row.TaskID, "exec-reserved")
	f.provider.mu.Lock()
	f.provider.current = "exec-reserved"
	f.provider.runtimes["exec-reserved"] = &agentruntime.AgentExecution{ID: "exec-reserved", TaskID: f.row.TaskID,
		SessionID: f.row.ID, DeliveryIncarnationID: session.QueueIncarnationID}
	f.provider.mu.Unlock()

	core, logs := observer.New(zapcore.DebugLevel)
	zapLogger := zap.New(core)
	wrapped, err := logger.NewFromZap(zapLogger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = zapLogger.Sync() })
	service, _ := teardownIntegrationServiceWithLogger(t, f, f.repo, wrapped)

	require.ErrorContains(t, service.Reconcile(t.Context()), "legacy errand lacks positive initial admission authority")
	require.NoError(t, service.Reconcile(t.Context()), "unchanged persisted evidence remains retryable without repeating its warning")
	require.Len(t, logs.FilterMessage("errand teardown remains pending").All(), 1,
		"the per-row warning and returned sweep error share one stable refusal report")
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentPending, row.TeardownIntent.State)
	require.Empty(t, f.provider.stops)
	// A different observed submission supersedes a reserved target before admission diagnostics.
	assertChangedAdmissionRefusals(t, f, service, logs, "sender_session_id", "state")

	require.NoError(t, f.svc.turnService.PatchTurnMetadata(t.Context(), f.row.ID, f.row.FirstTurnID, map[string]interface{}{
		errandAdmissionTurnMetadataKey: map[string]interface{}{"submission_id": f.row.SubmissionID,
			"sender_session_id": f.row.ParentSessionID, "state": string(models.ErrandAdmissionAccepted)},
	}))
	require.NoError(t, service.Reconcile(t.Context()), "changed exact authority is retried")
	row, err = f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
	require.Equal(t, []string{"exec-reserved"}, f.provider.stops)
}

func assertChangedAdmissionRefusals(t *testing.T, f *errandTeardownFixture, service *errands.Service, logs *observer.ObservedLogs, fields ...string) {
	t.Helper()
	count := logs.FilterMessage("errand teardown remains pending").Len()
	for _, field := range fields {
		for _, value := range []interface{}{nil, "conflicting-a", "conflicting-b"} {
			marker := map[string]interface{}{"submission_id": f.row.SubmissionID,
				"sender_session_id": f.row.ParentSessionID, "state": string(models.ErrandAdmissionAccepted)}
			marker[field] = value
			require.NoError(t, f.svc.turnService.PatchTurnMetadata(t.Context(), f.row.ID, f.row.FirstTurnID,
				map[string]interface{}{errandAdmissionTurnMetadataKey: marker}))
			require.ErrorContains(t, service.Reconcile(t.Context()), "legacy errand lacks positive initial admission authority",
				"changed %s=%v evidence must be reported, including within the same refusal category", field, value)
			count++
			require.Len(t, logs.FilterMessage("errand teardown remains pending").All(), count)
			require.NoError(t, service.Reconcile(t.Context()), "identical evidence remains deduplicated")
			require.Len(t, logs.FilterMessage("errand teardown remains pending").All(), count)
			require.Empty(t, f.provider.stops, "invalid admission must not authorize stopping")
		}
	}
}

// @covers AC-TASKS-ERRANDS-007.1 AC-TASKS-ERRANDS-007.6 AC-TASKS-ERRANDS-012.3
func TestBulkCancellationRetriesPersistedTeardown(t *testing.T) {
	for _, entry := range []string{"CancelAll", "CancelForParent", "CancelForTask"} {
		for _, faultAt := range []string{"session", "execution"} {
			t.Run(entry+"/"+faultAt, func(t *testing.T) {
				f := newErrandTeardownFixture(t)
				fault := errors.New("postcommit inventory unavailable")
				repo := &teardownIntegrationRepo{Repository: f.repo}
				repo.afterCancel = func() {
					if faultAt == "session" {
						repo.sessionErr = fault
					} else {
						f.provider.lookupErr = fault
					}
				}
				newCancellations := 1
				if faultAt == "session" {
					sibling := &models.Errand{ID: "queued-sibling", TaskID: f.row.TaskID, ParentSessionID: f.row.ParentSessionID,
						Title: "Sibling", Objective: "Independent reserved launch", AgentProfileID: "profile", FirstTurnID: "sibling-first",
						SubmissionID: "sibling-submission", Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone}
					require.NoError(t, f.repo.CreateErrand(t.Context(), sibling))
					newCancellations++
				}
				f.svc.repo = repo
				service, recorder := teardownIntegrationService(t, f, repo)
				var count int
				var err error
				switch entry {
				case "CancelAll":
					count, err = service.CancelAll(t.Context(), f.row.TaskID)
				case "CancelForParent":
					count, err = service.CancelForParent(t.Context(), f.row.ParentSessionID)
				case "CancelForTask":
					count, err = service.CancelForTask(t.Context(), f.row.TaskID)
				}
				require.ErrorIs(t, err, fault)
				require.Equal(t, newCancellations, count, "committed cancellation counts even when physical teardown fails; siblings still commit")
				row, err := f.repo.GetErrand(t.Context(), f.row.ID)
				require.NoError(t, err)
				require.Equal(t, models.ErrandLifecycleCancelled, row.Lifecycle)
				require.NotNil(t, row.TeardownIntent, "logical cancellation must leave durable exact stop authority")
				require.Equal(t, models.ErrandTeardownIntentPending, row.TeardownIntent.State)
				require.Len(t, recorder.seen(), newCancellations)
				for _, status := range recorder.seen() {
					require.Equal(t, models.ErrandStatusCancelled, status)
				}
				require.Empty(t, f.provider.stops)
				if faultAt == "session" {
					sibling, err := f.repo.GetErrand(t.Context(), "queued-sibling")
					require.NoError(t, err)
					require.Equal(t, models.ErrandTeardownIntentCompleted, sibling.TeardownIntent.State)
				}
				repo.afterCancel, repo.sessionErr, f.provider.lookupErr = nil, nil, nil
				fresh, _ := teardownIntegrationService(t, f, repo)
				fresh.SetEnabled(func() bool { return false })
				require.NoError(t, fresh.Reconcile(t.Context()))
				got, err := f.repo.GetErrand(t.Context(), f.row.ID)
				require.NoError(t, err)
				require.Equal(t, models.ErrandTeardownIntentCompleted, got.TeardownIntent.State)
				require.Equal(t, row.TeardownIntent.ID, got.TeardownIntent.ID)
				require.Equal(t, []string{"exec-1"}, f.provider.stops)
			})
		}
	}
}

func TestErrandTeardownServiceRetriesPhysicalStopFailureOnDisabledSweep(t *testing.T) {
	f := newErrandTeardownFixture(t)
	service, recorder := teardownIntegrationService(t, f, f.repo)
	fault := errors.New("physical provider failure")
	f.provider.stopErr = fault
	_, err := service.Cancel(t.Context(), errands.CancelRequest{ErrandID: f.row.ID, TaskID: f.row.TaskID, Source: models.ErrandCancelSourceUser})
	require.ErrorIs(t, err, fault)
	before, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentPending, before.TeardownIntent.State)
	require.Equal(t, []models.ErrandStatus{models.ErrandStatusCancelled}, recorder.seen())
	f.provider.stopErr = nil
	service.SetEnabled(func() bool { return false })
	f.svc.ceilingSweepTick(t.Context())
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
	require.Equal(t, before.TeardownIntent.ID, row.TeardownIntent.ID)
	require.Equal(t, before.ResultRevision, row.ResultRevision)
	require.Equal(t, []string{"exec-1", "exec-1"}, f.provider.stops)
	require.Len(t, recorder.seen(), 1, "teardown retry does not publish another cancellation")
}

func TestStaleErrandTeardownServiceRecoveryPreservesSuccessor(t *testing.T) {
	for _, variant := range []string{"unchanged", "same execution", "replacement execution", "replacement incarnation"} {
		t.Run(variant, func(t *testing.T) {
			f := newErrandTeardownFixture(t)
			repo := &teardownIntegrationRepo{Repository: f.repo}
			fault := errors.New("crash after cancellation commit")
			repo.afterCancel = func() { f.provider.lookupErr = fault }
			f.svc.repo = repo
			service, _ := teardownIntegrationService(t, f, repo)
			_, err := service.Cancel(t.Context(), errands.CancelRequest{ErrandID: f.row.ID, TaskID: f.row.TaskID, Source: models.ErrandCancelSourceUser})
			require.ErrorIs(t, err, fault)
			repo.afterCancel, f.provider.lookupErr = nil, nil
			ready, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			allow := func() { once.Do(func() { close(release) }) }
			t.Cleanup(func() { allow(); coordinatorStopAwaitSignal(t, done, "recovery worker cleanup") })
			fresh, recorder := teardownIntegrationService(t, f, repo)
			var retryErr error
			go func() {
				defer close(done)
				close(ready)
				<-release
				retryErr = fresh.Reconcile(t.Context())
			}()
			coordinatorStopAwaitSignal(t, ready, "fresh service retry held")
			if variant != "unchanged" {
				f.successor(t, true, variant == "replacement execution")
			}
			if variant == "replacement incarnation" {
				_, err = f.repo.DB().ExecContext(t.Context(), "UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?", "new-incarnation", f.row.ID)
				require.NoError(t, err)
			}
			before, err := f.repo.GetTaskSession(t.Context(), f.row.ID)
			require.NoError(t, err)
			queue := f.svc.messageQueue.GetStatus(t.Context(), f.row.ID)
			baseline := recorder.seen()
			allow()
			coordinatorStopAwaitSignal(t, done, "fresh service retry completed")
			require.NoError(t, retryErr)
			row, err := f.repo.GetErrand(t.Context(), f.row.ID)
			require.NoError(t, err)
			if variant == "unchanged" || variant == "replacement execution" {
				require.Equal(t, []string{"exec-1"}, f.provider.stops)
				require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
			} else {
				require.Empty(t, f.provider.stops)
				require.Equal(t, models.ErrandTeardownIntentSuperseded, row.TeardownIntent.State)
			}
			if variant != "unchanged" {
				after, err := f.repo.GetTaskSession(t.Context(), f.row.ID)
				require.NoError(t, err)
				require.Equal(t, before, after)
				require.Equal(t, queue, f.svc.messageQueue.GetStatus(t.Context(), f.row.ID))
			}
			require.Equal(t, baseline, recorder.seen())
		})
	}
}

func TestGenericStopOfAnErrandRoutesOnceAndFailsClosed(t *testing.T) {
	for _, variant := range []string{"errand", "role read failure", "ordinary", "unwired"} {
		t.Run(variant, func(t *testing.T) {
			f := newErrandTeardownFixture(t)
			fault := errors.New("errand stop failed")
			controller := &recordingErrandController{stopErr: fault}
			f.svc.SetErrandController(controller)
			if variant == "unwired" {
				f.svc.SetErrandController(nil)
			}
			if variant == "ordinary" {
				_, err := f.repo.DB().ExecContext(t.Context(), "UPDATE task_sessions SET role = '', parent_session_id = '' WHERE id = ?", f.row.ID)
				require.NoError(t, err)
			}
			if variant == "role read failure" {
				f.svc.repo = &teardownIntegrationRepo{Repository: f.repo, sessionErr: fault}
			}
			stopped := make(chan struct{})
			if variant == "ordinary" {
				f.provider.beforeStop = func(context.Context, string) error { close(stopped); return nil }
			}
			err := f.svc.StopSession(t.Context(), f.row.ID, "user stop", false)
			switch variant {
			case "errand":
				require.ErrorIs(t, err, fault)
				require.Equal(t, []string{"cancel-on-stop:s1"}, controller.seen())
				require.Empty(t, f.provider.stops, "a failed identity-aware stop cannot fall through to broad stop")
			case "role read failure":
				require.ErrorIs(t, err, fault)
				require.Empty(t, controller.seen())
				require.Empty(t, f.provider.stops)
			case "unwired":
				require.ErrorContains(t, err, "errand")
				require.Empty(t, f.provider.stops)
			case "ordinary":
				require.NoError(t, err)
				coordinatorStopAwaitSignal(t, stopped, "ordinary physical stop positive control")
				require.NotEmpty(t, controller.seen())
				for _, event := range controller.seen() {
					require.Equal(t, "cascade-parent:s1", event)
				}
			}
		})
	}
}

func TestErrandTeardownRecoveryWaitsForAuthenticatedInventory(t *testing.T) {
	f := newErrandTeardownFixture(t)
	intent := f.pending(t)
	f.provider.current = ""
	delete(f.provider.runtimes, "exec-1")
	f.svc.SetPendingRemoteRecoveryChecker(func(session, execution string) bool { return session == "s1" && execution == "exec-1" })
	service, _ := teardownIntegrationService(t, f, f.repo)
	service.SetEnabled(func() bool { return false })
	require.Error(t, service.Reconcile(t.Context()), "a runtime not yet restored is not confirmed absent")
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentPending, row.TeardownIntent.State)
	require.Equal(t, intent.ID, row.TeardownIntent.ID)
	f.svc.SetPendingRemoteRecoveryChecker(nil)
	running, err := f.repo.GetExecutorRunningBySessionID(t.Context(), f.row.ID)
	require.NoError(t, err)
	running.Status, running.LocalPID = models.ExecutorRunningStatusStopped, 0
	_, err = f.repo.DB().ExecContext(t.Context(), "UPDATE executors_running SET status = ?, local_pid = 0 WHERE session_id = ?", running.Status, f.row.ID)
	require.NoError(t, err)
	require.NoError(t, service.Reconcile(t.Context()))
	row, err = f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
}

func TestErrandTeardownLegacyRecoveryRequiresInitialAdmissionProof(t *testing.T) {
	for _, variant := range []string{
		"unknown", "missing submission", "conflicting submission", "missing sender", "conflicting sender",
		"missing state", "conflicting state", "initial", "later user turn",
	} {
		t.Run(variant, func(t *testing.T) {
			f := newErrandTeardownFixture(t)
			var err error
			admission := map[string]interface{}{
				"submission_id": f.row.SubmissionID, "sender_session_id": f.row.ParentSessionID,
				"state": string(models.ErrandAdmissionAccepted),
			}
			if strings.HasPrefix(variant, "missing ") || strings.HasPrefix(variant, "conflicting ") {
				field := strings.TrimPrefix(strings.TrimPrefix(variant, "missing "), "conflicting ")
				switch field {
				case "submission":
					field = "submission_id"
				case "sender":
					field = "sender_session_id"
				}
				if strings.HasPrefix(variant, "missing ") {
					delete(admission, field)
				} else {
					admission[field] = "foreign-authority"
				}
			}
			if variant != "unknown" {
				require.NoError(t, f.svc.turnService.PatchTurnMetadata(t.Context(), f.row.ID, f.row.FirstTurnID, map[string]interface{}{
					errandAdmissionTurnMetadataKey: admission,
				}))
			}
			if variant == "later user turn" {
				f.successor(t, true, false)
			}
			f.row, err = f.repo.GetErrand(t.Context(), f.row.ID)
			require.NoError(t, err)
			now := time.Now().UTC()
			f.row.Lifecycle, f.row.CancelSource, f.row.SettledAt = models.ErrandLifecycleCancelled, models.ErrandCancelSourceUser, &now
			require.NoError(t, f.repo.UpdateErrand(t.Context(), f.row))
			before, err := f.repo.GetErrand(t.Context(), f.row.ID)
			require.NoError(t, err)
			service, recorder := teardownIntegrationService(t, f, f.repo)
			err = service.Reconcile(t.Context())
			row, readErr := f.repo.GetErrand(t.Context(), f.row.ID)
			require.NoError(t, readErr)
			evidenceRefusal := variant == "unknown" || strings.HasPrefix(variant, "missing ") || strings.HasPrefix(variant, "conflicting ")
			if evidenceRefusal {
				require.ErrorContains(t, err, "legacy errand lacks positive initial admission authority")
				require.Nil(t, row.TeardownIntent)
				require.Empty(t, f.provider.stops)
				require.NoError(t, f.svc.turnService.PatchTurnMetadata(t.Context(), f.row.ID, f.row.FirstTurnID, map[string]interface{}{
					errandAdmissionTurnMetadataKey: map[string]interface{}{"submission_id": f.row.SubmissionID,
						"sender_session_id": f.row.ParentSessionID, "state": string(models.ErrandAdmissionAccepted)},
				}))
				require.NoError(t, service.Reconcile(t.Context()), "changed exact authority is retried")
				row, readErr = f.repo.GetErrand(t.Context(), f.row.ID)
				require.NoError(t, readErr)
				require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
				require.Equal(t, []string{"exec-1"}, f.provider.stops)
			} else {
				require.NoError(t, err)
				require.NotNil(t, row.TeardownIntent)
				require.Equal(t, f.row.FirstTurnID, row.TeardownIntent.Target.FirstTurnID)
				if variant == "initial" {
					require.Equal(t, []string{"exec-1"}, f.provider.stops)
				} else {
					require.Empty(t, f.provider.stops)
					require.Equal(t, models.ErrandTeardownIntentSuperseded, row.TeardownIntent.State)
				}
			}
			require.Equal(t, before.ResultRevision, row.ResultRevision)
			require.Empty(t, recorder.seen(), "legacy cleanup creates no new logical cancellation")
		})
	}
}

type teardownNoticeRecorder struct {
	mu      sync.Mutex
	parents []string
}

func (r *teardownNoticeRecorder) NotificationBlocked(context.Context, string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.parents) != 0
}

func (r *teardownNoticeRecorder) DeliverPending(_ context.Context, parent string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.parents = append(r.parents, parent)
}

func (*teardownNoticeRecorder) AdmitPrepared(context.Context, string, string, map[string]interface{}) error {
	return nil
}

func TestErrandTeardownServiceCrashAfterStopBeforeReceipt(t *testing.T) {
	f := newErrandTeardownFixture(t)
	f.row.AwaitingResult = true
	require.NoError(t, f.repo.UpdateErrand(t.Context(), f.row))
	fault := errors.New("receipt unavailable after physical stop")
	repo := &teardownReceiptFailureRepo{Repository: f.repo, fault: fault}
	f.svc.repo = repo
	service, _ := teardownIntegrationService(t, f, f.repo)
	notices := &teardownNoticeRecorder{}
	service.SetDeliverer(notices)
	_, err := service.Cancel(t.Context(), errands.CancelRequest{TaskID: f.row.TaskID, ErrandID: f.row.ID, Source: models.ErrandCancelSourceUser})
	require.ErrorIs(t, err, fault)
	before, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentPending, before.TeardownIntent.State)
	require.Equal(t, []string{f.row.ParentSessionID}, notices.parents)
	require.Equal(t, []string{"exec-1"}, f.provider.stops)
	repo.fault = nil
	fresh, recorder := teardownIntegrationService(t, f, f.repo)
	fresh.SetDeliverer(notices)
	require.NoError(t, fresh.Reconcile(t.Context()))
	count, err := fresh.CancelAll(t.Context(), f.row.TaskID)
	require.NoError(t, err)
	require.Zero(t, count)
	after, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentCompleted, after.TeardownIntent.State)
	require.Equal(t, before.TeardownIntent.ID, after.TeardownIntent.ID)
	require.Equal(t, before.ResultRevision, after.ResultRevision)
	require.Equal(t, before.ResultText, after.ResultText)
	require.Equal(t, []string{f.row.ParentSessionID}, notices.parents)
	require.Equal(t, []string{"exec-1"}, f.provider.stops)
	require.Empty(t, recorder.seen())
}

func TestErrandTeardownStartupDoesNotBypassPendingExactTarget(t *testing.T) {
	f := newErrandTeardownFixture(t)
	service, _ := teardownIntegrationService(t, f, f.repo)
	fault := errors.New("physical stop failed before restart")
	f.provider.stopErr = fault
	_, err := service.Cancel(t.Context(), errands.CancelRequest{TaskID: f.row.TaskID, ErrandID: f.row.ID, Source: models.ErrandCancelSourceUser})
	require.ErrorIs(t, err, fault)
	f.provider.stopErr = nil
	f.svc.reconcileExecutorSessionsOnStartup(t.Context())
	require.Equal(t, []string{"exec-1"}, f.provider.stops, "ordinary startup cleanup must leave pending errands to their exact owner")
	turn, err := f.repo.GetTurn(t.Context(), f.row.FirstTurnID)
	require.NoError(t, err)
	require.Nil(t, turn.CompletedAt, "exact cancellation still owns its captured turn")
	f.svc.reconcileErrandsOnStartup(t.Context())
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandTeardownIntentCompleted, row.TeardownIntent.State)
	require.Equal(t, []string{"exec-1", "exec-1"}, f.provider.stops)
}

func TestErrandTeardownDeleteDoesNotReenterOwnedAdmissionGuard(t *testing.T) {
	f := newErrandTeardownFixture(t)
	teardownIntegrationService(t, f, f.repo)
	require.NoError(t, f.repo.UpdateTaskSessionState(t.Context(), f.row.ID, models.TaskSessionStateWaitingForInput, ""))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, f.svc.DeleteSession(ctx, f.row.ID))
	_, err := f.repo.GetTaskSession(t.Context(), f.row.ID)
	require.ErrorIs(t, err, models.ErrTaskSessionNotFound)
	require.Equal(t, []string{"exec-1"}, f.provider.stops)
}

type teardownDeleteInterleaving struct {
	*errands.Service
	beforeNotice func(context.Context)
}

func (s *teardownDeleteInterleaving) PreserveCancellationNotice(ctx context.Context, id string) error {
	s.beforeNotice(ctx)
	return s.Service.PreserveCancellationNotice(ctx, id)
}

func TestStaleErrandTeardownDeletePreservesNewUserAdmission(t *testing.T) {
	f := newErrandTeardownFixture(t)
	service, _ := teardownIntegrationService(t, f, f.repo)
	require.NoError(t, f.repo.UpdateTaskSessionState(t.Context(), f.row.ID, models.TaskSessionStateWaitingForInput, ""))
	f.svc.SetErrandController(&teardownDeleteInterleaving{Service: service, beforeNotice: func(ctx context.Context) {
		guard, err := f.svc.lockCancelInFlightGuardWithContext(ctx, f.row.ID)
		require.NoError(t, err, "deletion must hand off its admission guard")
		guard.release()
		f.successor(t, true, false)
	}})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.Error(t, f.svc.DeleteSession(ctx, f.row.ID))
	session, err := f.repo.GetTaskSession(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	require.Empty(t, f.provider.stops)
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleLaunched, row.Lifecycle)
	require.Nil(t, row.TeardownIntent)
}
