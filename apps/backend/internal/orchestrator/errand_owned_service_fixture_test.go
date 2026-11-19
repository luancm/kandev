package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type ownedSettlementTranscript struct {
	errands.TurnReader
	repo *sqliterepo.Repository
}

func (r ownedSettlementTranscript) ReadTurn(ctx context.Context, errandID, turnID string) (errands.TurnView, error) {
	turn, err := r.repo.GetErrandTurn(ctx, errandID, turnID)
	if err != nil {
		return errands.TurnView{}, err
	}
	sender, _ := turn.Metadata["sender_session_id"].(string)
	return errands.TurnView{Completed: turn.CompletedAt != nil, EarliestUserSender: sender}, nil
}

type errandServiceRuntime struct {
	runtimeapi.Runtime
	executions sync.Map
	stopped    chan string
	workspace  string
	stopErr    error
}

func (r *errandServiceRuntime) Launch(ctx context.Context, spec runtimeapi.LaunchSpec) (runtimeapi.ExecutionRef, error) {
	if err := spec.OwnerAdmission.AdmitExecution(ctx, spec.Owner); err != nil {
		return runtimeapi.ExecutionRef{}, err
	}
	launch, ok := spec.Metadata["launch_request"].(*runtimeapi.LaunchRequest)
	if !ok || launch.CandidateExecutionID == "" {
		return runtimeapi.ExecutionRef{}, errors.New("fixture expected the reserved owned execution")
	}
	execution := &runtimeapi.Execution{ID: launch.CandidateExecutionID, Owner: spec.Owner}
	if err := launch.OnExecutionAllocated(ctx, execution.ID); err != nil {
		return runtimeapi.ExecutionRef{}, err
	}
	r.executions.Store(execution.ID, execution)
	return runtimeapi.ExecutionRef{ID: execution.ID}, nil
}

func (r *errandServiceRuntime) GetExecution(_ context.Context, id string) (*runtimeapi.Execution, error) {
	execution, ok := r.executions.Load(id)
	if !ok {
		return nil, runtimeapi.ErrNotFound
	}
	return execution.(*runtimeapi.Execution), nil
}

func (r *errandServiceRuntime) StartExecution(context.Context, string) error {
	return nil
}

func (r *errandServiceRuntime) Stop(_ context.Context, id, reason string) error {
	if r.stopErr != nil {
		return r.stopErr
	}
	if _, exists := r.executions.LoadAndDelete(id); !exists {
		return runtimeapi.ErrNotFound
	}
	r.stopped <- reason
	return nil
}

func configureOwnedErrandServiceRuntime(t *testing.T, adapter *ErrandAdapter) *errandServiceRuntime {
	t.Helper()
	runtime := &errandServiceRuntime{stopped: make(chan string, errands.MaxLiveErrands), workspace: t.TempDir()}
	runtime.attach(adapter)
	return runtime
}

func (r *errandServiceRuntime) attach(adapter *ErrandAdapter) {
	adapter.SetOwnedLaunchRuntime(r, func(_ context.Context, taskID, parentID string) (*runtimeapi.WorkspaceInfo, error) {
		return &runtimeapi.WorkspaceInfo{
			TaskID: taskID, SessionID: parentID, WorkspaceID: "ws1",
			WorkspacePath: r.workspace, ExecutorType: string(models.ExecutorTypeWorktree),
			TaskDirName: "attached-worktree",
		}, nil
	})
}

func newOwnedErrandServiceOnRealStack(t *testing.T, f *errandLaunchFixture) (*errands.Service, *errandServiceRuntime, *errandEventRecorder) {
	t.Helper()
	adapter := NewErrandAdapter(f.svc)
	runtime := configureOwnedErrandServiceRuntime(t, adapter)
	events := &errandEventRecorder{}
	service, err := errands.New(errands.Dependencies{
		Store: f.repo, Launcher: adapter, Stopper: adapter, MCP: adapter,
		Profiles: errandProfileStub{}, Events: events, Logger: testLogger(),
	})
	require.NoError(t, err)
	f.svc.SetErrandController(service)
	service.StartWorkers(t.Context())
	t.Cleanup(service.StopWorkers)
	return service, runtime, events
}
