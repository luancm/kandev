package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type launchRecordBarrierStore struct {
	*sqliterepo.Repository
	admitted <-chan error
	reached  chan struct{}
	allow    chan struct{}
	once     sync.Once
}

func (r *launchRecordBarrierStore) UpdateErrand(ctx context.Context, row *models.Errand) error {
	if row.Lifecycle == models.ErrandLifecycleLaunched {
		r.once.Do(func() { <-r.admitted; close(r.reached); <-r.allow })
	}
	return r.Repository.UpdateErrand(ctx, row)
}

// @covers AC-TASKS-ERRANDS-007.1 AC-TASKS-ERRANDS-007.2
func TestErrandLaunchRecordFailureCancellationWins(t *testing.T) {
	f := newErrandLaunchFixture(t)
	admitted := installLaunchAdoptionProvider(t, f)
	store := &launchRecordBarrierStore{Repository: f.repo, admitted: admitted, reached: make(chan struct{}), allow: make(chan struct{})}
	service := launchRecoveryService(t, f, store)
	var once sync.Once
	release := func() { once.Do(func() { close(store.allow) }) }
	done := make(chan struct{})
	var result *errands.StartResult
	var startErr error
	go func() {
		defer close(done)
		result, startErr = service.Start(t.Context(), errands.StartRequest{TaskID: f.taskID, ParentSessionID: f.parent, Title: "cancel at commit", Objective: "same execution only"})
	}()
	t.Cleanup(func() { release(); coordinatorStopAwaitSignal(t, done, "launch commit barrier cleanup") })
	coordinatorStopAwaitSignal(t, store.reached, "admission before launch bookkeeping")
	rows, err := f.repo.ListErrandsByTask(t.Context(), f.taskID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	row := rows[0]
	_, err = service.Cancel(t.Context(), errands.CancelRequest{TaskID: f.taskID, ErrandID: row.ID, Source: models.ErrandCancelSourceUser})
	require.NoError(t, err)
	release()
	coordinatorStopAwaitSignal(t, done, "cancelled launch bookkeeping")
	require.NoError(t, startErr)
	require.Equal(t, models.ErrandStatusCancelled, result.Status)
	fresh := launchRecoveryService(t, f, f.repo)
	require.NoError(t, fresh.Reconcile(t.Context()))
	f.svc.ceilingSweepTick(t.Context())
	cancelled, err := f.repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleCancelled, cancelled.Lifecycle)
	require.Equal(t, models.ErrandTeardownIntentCompleted, cancelled.TeardownIntent.State)
	require.Equal(t, "execution-"+row.ID, cancelled.TeardownIntent.Target.ExecutionID)
	f.manager.captured.Lock()
	require.Len(t, f.manager.launchedWith, 1)
	f.manager.captured.Unlock()
	f.manager.mu.Lock()
	require.Len(t, f.manager.startAgentProcessCalls, 1)
	require.Len(t, f.manager.stopAgentWithReasonArgs, 1)
	require.Equal(t, "execution-"+row.ID, f.manager.stopAgentWithReasonArgs[0].ExecutionID)
	f.manager.mu.Unlock()
}

type launchInspectionFaultRepo struct {
	*sqliterepo.Repository
	failed atomic.Bool
}

func (r *launchInspectionFaultRepo) GetAgentDeliverySubmissionMetadata(ctx context.Context, id string) (*models.AgentDeliverySubmission, error) {
	if !r.failed.Swap(true) {
		return nil, errors.New("injected reserved submission read failure")
	}
	return r.Repository.GetAgentDeliverySubmissionMetadata(ctx, id)
}

func TestErrandLaunchRecoveryInspectionFailureRetainsReservation(t *testing.T) {
	f := newErrandLaunchFixture(t)
	req := f.request()
	f.svc.repo = &launchInspectionFaultRepo{Repository: f.repo}
	service := launchRecoveryService(t, f, f.repo)
	service.DrainQueued(t.Context())
	row, err := f.repo.GetErrand(t.Context(), req.ErrandID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleQueued, row.Lifecycle)
	require.True(t, row.AwaitingResult)
	require.Equal(t, models.ErrandResultNone, row.ResultState)
	require.Empty(t, f.manager.launchedWith)
	// The transient read is now healthy. The exact queued reservation launches.
	service.DrainQueued(t.Context())
	row, err = f.repo.GetErrand(t.Context(), req.ErrandID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleLaunched, row.Lifecycle)
	require.Len(t, f.manager.launchedWith, 1)
}

func TestLaunchErrandSessionConcurrentCallersAdoptOneAdmission(t *testing.T) {
	f := newErrandLaunchFixture(t)
	req := f.request()
	admitted := installLaunchAdoptionProvider(t, f)
	done := make(chan error, 2)
	for range 2 {
		go func() { _, err := f.svc.LaunchErrandSession(t.Context(), req); done <- err }()
	}
	require.NoError(t, <-done)
	require.NoError(t, <-done)
	require.NoError(t, <-admitted)
	assertOneErrandLaunch(t, f)
}

type launchInventoryUnknownRepo struct {
	*sqliterepo.Repository
	unknown atomic.Bool
}

func (r *launchInventoryUnknownRepo) GetExecutorRunningBySessionID(ctx context.Context, id string) (*models.ExecutorRunning, error) {
	if !r.unknown.Swap(true) {
		return nil, nil
	}
	return r.Repository.GetExecutorRunningBySessionID(ctx, id)
}

func TestErrandLaunchRecoveryNilInventoryIsUnknown(t *testing.T) {
	f := newErrandLaunchFixture(t)
	req := f.request()
	f.svc.repo = &launchInventoryUnknownRepo{Repository: f.repo}
	result, err := NewErrandAdapter(f.svc).LaunchErrand(t.Context(), req)
	require.Error(t, err)
	require.Equal(t, errands.LaunchBlocked, result.Disposition)
	require.Empty(t, f.manager.launchedWith)
	result, err = NewErrandAdapter(f.svc).LaunchErrand(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, errands.LaunchAdmitted, result.Disposition)
	require.Len(t, f.manager.launchedWith, 1)
}

func TestErrandLaunchRecoveryChecksTaskAuthorizationBeforeAdoption(t *testing.T) {
	f := newErrandLaunchFixture(t)
	req := f.request()
	admitted := installLaunchAdoptionProvider(t, f)
	_, err := f.svc.LaunchErrandSession(t.Context(), req)
	require.NoError(t, err)
	require.NoError(t, <-admitted)
	denied := errors.New("task access denied")
	f.svc.SetTaskAccessChecker(func(context.Context, string) error { return denied })
	_, err = f.svc.LaunchErrandSession(t.Context(), req)
	require.ErrorIs(t, err, denied, "adoption must apply the same task authorization as a fresh launch")
}

func TestErrandLaunchRecoveryAttemptedSuccessorBlocksAdoption(t *testing.T) {
	f := newErrandLaunchFixture(t)
	admitted := installLaunchAdoptionProvider(t, f)
	service := launchRecoveryService(t, f, &launchRecordFaultStore{Repository: f.repo, admitted: admitted})
	_, err := service.Start(t.Context(), errands.StartRequest{TaskID: f.taskID, ParentSessionID: f.parent, Title: "current successor", Objective: "retain initial proof"})
	require.Error(t, err)
	rows, err := f.repo.ListErrandsByTask(t.Context(), f.taskID)
	require.NoError(t, err)
	row := rows[0]
	require.NoError(t, f.svc.turnService.CompleteTurn(t.Context(), row.FirstTurnID))
	successor, err := f.svc.turnService.ReserveTurn(t.Context(), row.ID, nil)
	require.NoError(t, err)
	require.NoError(t, f.svc.turnService.MarkReservedTurnDispatchAttempted(t.Context(), successor))
	result, err := NewErrandAdapter(f.svc).LaunchErrand(t.Context(), errands.LaunchRequest{
		ErrandID: row.ID, TaskID: row.TaskID, ParentSessionID: row.ParentSessionID, AgentProfileID: row.AgentProfileID,
		Title: row.Title, Objective: row.Objective, TurnID: row.FirstTurnID, SubmissionID: row.SubmissionID,
	})
	require.Error(t, err, "a current attempted unpublished successor cannot be hidden by latest visible history")
	require.Equal(t, errands.LaunchBlocked, result.Disposition)
	current, err := f.repo.GetActiveTurnBySessionID(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, successor.ID, current.ID)
	f.manager.mu.Lock()
	require.Len(t, f.manager.startAgentProcessCalls, 1)
	require.Empty(t, f.manager.stopAgentWithReasonArgs)
	f.manager.mu.Unlock()
}

func TestErrandLaunchRecoveryPreparedHonorsNativeRecoveryBlock(t *testing.T) {
	f := newErrandLaunchFixture(t)
	req := f.request()
	prepareReservedErrand(t, f, req, true)
	launchRecoveryService(t, f, f.repo)
	require.NoError(t, f.svc.recordSessionRecoveryBlock(t.Context(), req.ErrandID, "agent_delivery",
		&sessionRecoveryRequiredError{Block: &models.SessionRecoveryBlock{Reason: "harness_generation_changed", ConsumerReference: "agent_delivery"}}))
	before, err := f.svc.GetOpenSessionRecoveryBlock(t.Context(), req.ErrandID)
	require.NoError(t, err)
	require.NotNil(t, before)
	result, err := NewErrandAdapter(f.svc).LaunchErrand(t.Context(), req)
	require.ErrorIs(t, err, ErrSessionRecoveryRequired, "prepared continuation must retain the existing native recovery gate")
	require.Equal(t, errands.LaunchBlocked, result.Disposition)
	require.Empty(t, f.manager.launchedWith)
	f.manager.mu.Lock()
	require.Empty(t, f.manager.startAgentProcessCalls)
	require.Empty(t, f.manager.capturedPromptCalls)
	require.Empty(t, f.manager.stopAgentWithReasonArgs)
	f.manager.mu.Unlock()
	blocks, err := f.repo.ListOpenSessionRecoveryBlocks(t.Context(), req.ErrandID, before.IncarnationID, before.ExpectedGeneration)
	require.NoError(t, err)
	require.Equal(t, []*models.SessionRecoveryBlock{before}, blocks, "retain native generation proof without adding another blocker")
}
