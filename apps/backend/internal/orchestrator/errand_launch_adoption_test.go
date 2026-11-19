package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type ownedReceiptLauncher struct {
	errands.Launcher
	fixture *errandLaunchFixture
}

func (l ownedReceiptLauncher) LaunchErrand(ctx context.Context, request errands.LaunchRequest) (errands.LaunchResult, error) {
	seedOwnedLaunchReceipt(l.fixture.t, l.fixture, request, models.DeliverySubmissionAccepted, "execution", false)
	return l.Launcher.LaunchErrand(ctx, request)
}

func TestErrandServiceRecoversAcceptedOwnedLaunchAfterBookkeepingFailure(t *testing.T) {
	ctx := t.Context()
	f := newErrandLaunchFixture(t)
	adapter := NewErrandAdapter(f.svc)
	adapter.SetOwnedLaunchRuntime(ownedErrandLaunchFixtureRuntime{
		launch: func(context.Context, runtimeapi.LaunchSpec) (runtimeapi.ExecutionRef, error) {
			t.Fatal("an accepted reservation must never allocate another execution")
			return runtimeapi.ExecutionRef{}, nil
		},
	}, nil)
	deps := errands.Dependencies{
		Store:    &launchRecordFaultStore{Repository: f.repo},
		Launcher: ownedReceiptLauncher{Launcher: adapter, fixture: f}, Stopper: adapter, MCP: adapter,
		Profiles: errandProfileStub{}, Events: &errandEventRecorder{}, Logger: testLogger(),
	}
	service, err := errands.New(deps)
	require.NoError(t, err)
	_, err = service.Start(ctx, errands.StartRequest{
		TaskID: f.taskID, ParentSessionID: f.parent, Title: "retry bookkeeping", Objective: "use the accepted receipt",
	})
	require.ErrorContains(t, err, "injected first launch bookkeeping")
	rows, err := f.repo.ListErrandsByTask(ctx, f.taskID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	before := rows[0]
	require.Equal(t, models.ErrandLifecycleQueued, before.Lifecycle)

	events := &errandEventRecorder{}
	deps.Store, deps.Launcher, deps.Events = f.repo, adapter, events
	fresh, err := errands.New(deps)
	require.NoError(t, err)
	require.NoError(t, fresh.Reconcile(ctx))
	require.NoError(t, fresh.Reconcile(ctx))
	row, err := f.repo.GetErrand(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleLaunched, row.Lifecycle)
	require.Equal(t, before.FirstTurnID, row.FirstTurnID)
	require.Equal(t, before.SubmissionID, row.SubmissionID)
	require.Equal(t, []models.ErrandStatus{models.ErrandStatusRunning}, events.seen())
	sessions, err := f.repo.ListTaskSessions(ctx, f.taskID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, f.parent, sessions[0].ID)
}

type launchRecordFaultStore struct {
	*sqliterepo.Repository
	failed atomic.Bool
}

func (r *launchRecordFaultStore) UpdateErrand(ctx context.Context, row *models.Errand) error {
	if row.Lifecycle == models.ErrandLifecycleLaunched && row.LastOutcome == "" && r.failed.CompareAndSwap(false, true) {
		return errors.New("injected first launch bookkeeping write failure")
	}
	return r.Repository.UpdateErrand(ctx, row)
}
