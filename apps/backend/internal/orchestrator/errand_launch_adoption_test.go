package orchestrator

import (
	"context"
	"errors"
	"expvar"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type launchRecordFaultStore struct {
	*sqliterepo.Repository
	admitted <-chan error
	failed   atomic.Bool
}

func (r *launchRecordFaultStore) UpdateErrand(ctx context.Context, row *models.Errand) error {
	if row.Lifecycle == models.ErrandLifecycleLaunched && row.LastOutcome == "" && r.failed.CompareAndSwap(false, true) {
		if err := <-r.admitted; err != nil {
			return err
		}
		return errors.New("injected first launch bookkeeping write failure")
	}
	return r.Repository.UpdateErrand(ctx, row)
}

// The provider boundary runs the production dispatch callbacks. Its runtime
// receipt is built from the actual launch request, never from the reservation.
func installLaunchAdoptionProvider(t *testing.T, f *errandLaunchFixture) <-chan error {
	t.Helper()
	if f.svc.errands == nil {
		launchRecoveryService(t, f, f.repo)
	}
	admitted := make(chan error, 4)
	manager := &firstErrandAdmissionManager{errandLaunchManager: f.manager, durable: true}
	f.svc.agentManager = manager
	f.svc.executor = withTestExecutorActor(executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{}))
	register := f.manager.launchAgentFunc
	f.manager.launchAgentFunc = func(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		response, err := register(ctx, req)
		if err != nil {
			return nil, err
		}
		instance := &lifecycle.ExecutorInstance{InstanceID: response.AgentExecutionID}
		execution := instance.ToAgentExecution(&lifecycle.ExecutorCreateRequest{TaskID: req.TaskID,
			SessionID: req.SessionID, DeliveryIncarnationID: req.DeliveryIncarnationID,
			PromptTurnID: req.TurnID, Metadata: map[string]interface{}{
				"prompt_turn_id": req.TurnID, "initial_delivery_submission_id": req.InitialDeliverySubmissionID,
			}})
		f.manager.captured.Lock()
		f.manager.runtimes[response.AgentExecutionID] = execution
		f.manager.captured.Unlock()
		return response, nil
	}
	f.manager.startAgentProcessFunc = func(ctx context.Context, executionID string) error {
		f.manager.mu.Lock()
		before, accepted, failed := f.manager.initialPromptAdmissionCallback, f.manager.initialPromptDispatchCallback, f.manager.initialPromptFailureCallback
		f.manager.mu.Unlock()
		f.manager.captured.Lock()
		request := f.manager.launchedWith[len(f.manager.launchedWith)-1]
		f.manager.captured.Unlock()
		err := before(ctx, request.TaskDescription, request.Attachments)
		if err == nil {
			accepted()
		} else {
			failed()
		}
		admitted <- err
		return err
	}
	return admitted
}

func launchRecoveryService(t *testing.T, f *errandLaunchFixture, store errands.Store) *errands.Service {
	t.Helper()
	adapter := NewErrandAdapter(f.svc)
	service, err := errands.New(errands.Dependencies{Store: store, Launcher: adapter, Stopper: adapter,
		MCP: adapter, Profiles: errandProfileStub{}, Events: &errandEventRecorder{}, Logger: testLogger()})
	require.NoError(t, err)
	f.svc.SetErrandController(service)
	return service
}

// @covers AC-TASKS-ERRANDS-001.1 AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-008.2
func TestErrandLaunchRecordFailureAdoptsReservedSessionWithoutReplay(t *testing.T) {
	f := newErrandLaunchFixture(t)
	beforeCount := launchStartedCount()
	admitted := installLaunchAdoptionProvider(t, f)
	fault := &launchRecordFaultStore{Repository: f.repo, admitted: admitted}
	service := launchRecoveryService(t, f, fault)
	_, err := service.Start(t.Context(), errands.StartRequest{TaskID: f.taskID, ParentSessionID: f.parent,
		Title: "recover bookkeeping", Objective: "keep the original accepted prompt"})
	require.ErrorContains(t, err, "injected first launch bookkeeping")
	rows, err := f.repo.ListErrandsByTask(t.Context(), f.taskID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	before := rows[0]
	require.Equal(t, models.ErrandLifecycleQueued, before.Lifecycle)
	require.Equal(t, beforeCount, launchStartedCount(), "failed bookkeeping cannot count admission")
	receipt, err := f.repo.GetAgentDeliverySubmissionMetadata(t.Context(), before.SubmissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionDispatching, receipt.State)
	fresh := launchRecoveryService(t, f, f.repo)
	require.NoError(t, fresh.Reconcile(t.Context()))
	row, err := f.repo.GetErrand(t.Context(), before.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleLaunched, row.Lifecycle, "startup must adopt the exact admitted reservation")
	f.svc.ceilingSweepTick(t.Context())
	require.NoError(t, fresh.Reconcile(t.Context()))
	require.Equal(t, beforeCount+1, launchStartedCount(), "adoption must count the committed transition once")
	row, err = f.repo.GetErrand(t.Context(), before.ID)
	require.NoError(t, err)
	require.Equal(t, before.FirstTurnID, row.FirstTurnID)
	require.Equal(t, before.SubmissionID, row.SubmissionID)
	require.True(t, row.AwaitingResult)
	require.Equal(t, models.ErrandResultNone, row.ResultState)
	require.Empty(t, row.LastOutcome)
	assertOneErrandLaunch(t, f)
	_, lookupErr := f.manager.GetExecutionIDForSession(t.Context(), row.ID)
	require.NoError(t, lookupErr)
}

func launchStartedCount() int64 {
	value := expvar.Get("errands_started_total").(*expvar.Map).Get("outcome=started")
	if value == nil {
		return 0
	}
	return value.(*expvar.Int).Value()
}

func assertOneErrandLaunch(t *testing.T, f *errandLaunchFixture) {
	t.Helper()
	f.manager.captured.Lock()
	launches := len(f.manager.launchedWith)
	f.manager.captured.Unlock()
	f.manager.mu.Lock()
	starts, prompts, stops := len(f.manager.startAgentProcessCalls), len(f.manager.capturedPromptCalls), len(f.manager.stopAgentWithReasonArgs)
	f.manager.mu.Unlock()
	require.Equal(t, 1, launches, "physical allocation count")
	require.Equal(t, 1, starts, "physical process start count")
	require.Zero(t, prompts, "recovery must not send a follow-up provider prompt")
	require.Zero(t, stops, "adoption must not stop the retained execution")
}
