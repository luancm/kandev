package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type unreadableParentQueueStore struct{ *sqliterepo.Repository }

func (s unreadableParentQueueStore) ListErrandsByParent(context.Context, string) ([]*models.Errand, error) {
	return nil, errors.New("parent inventory unreadable")
}

func TestParentOutstandingQueueDrainUnreadableInventoryDoesNotWriteReview(t *testing.T) {
	f := newQueueSettlementFixture(t, false)
	adapter := NewErrandAdapter(f.svc)
	controller, err := errands.New(errands.Dependencies{Store: unreadableParentQueueStore{f.repo},
		Launcher: adapter, Stopper: adapter, MCP: adapter, Profiles: errandProfileStub{},
		Events: &errandEventRecorder{}, Logger: testLogger()})
	require.NoError(t, err)
	f.svc.SetErrandController(controller)
	require.NoError(t, f.repo.UpdateTaskSessionState(t.Context(), f.parent, models.TaskSessionStateRunning, ""))
	require.NoError(t, f.repo.CreateTurn(t.Context(), &models.Turn{ID: "unreadable-parent-A", TaskID: f.taskID,
		TaskSessionID: f.parent, StartedAt: time.Now().UTC()}))
	seedExecutorRunning(t, f.repo, f.parent, f.taskID, "parent-execution")
	_, err = f.svc.messageQueue.QueueMessage(t.Context(), f.parent, f.taskID, "ordinary parent successor", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	f.svc.handleAgentReady(t.Context(), watcher.AgentEventData{TaskID: f.taskID, SessionID: f.parent,
		AgentExecutionID: "parent-execution", TurnID: "unreadable-parent-A"})
	f.awaitDispatch(t)
	f.taskRepo.mu.Lock()
	history := append([]v1.TaskState(nil), f.taskRepo.stateHistory[f.taskID]...)
	f.taskRepo.mu.Unlock()
	require.NotContains(t, history, v1.TaskStateReview)
}
