package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type unreadableParentQueueStore struct{ *sqliterepo.Repository }

func (s unreadableParentQueueStore) ListErrandsByParent(context.Context, string) ([]*models.Errand, error) {
	return nil, errors.New("parent inventory unreadable")
}

func (s unreadableParentQueueStore) GetParentErrandState(context.Context, string) (bool, bool, error) {
	return false, false, errors.New("parent inventory unreadable")
}

func TestParentOutstandingQueueDrainUnreadableInventoryDoesNotWriteReview(t *testing.T) {
	f := newParentSettlementFailureFixture(t, "unreadable-inventory-queue")
	adapter := NewErrandAdapter(f.service)
	controller, err := errands.New(errands.Dependencies{Store: unreadableParentQueueStore{f.repo},
		Launcher: adapter, Stopper: adapter, MCP: adapter, Profiles: errandProfileStub{},
		Events: &errandEventRecorder{}, Logger: testLogger()})
	require.NoError(t, err)
	f.service.SetErrandController(controller)
	dispatched := make(chan string, 1)
	f.agentManager.promptAgentFunc = func(_ context.Context, _ string, prompt string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
		dispatched <- prompt
		return &executor.PromptResult{}, nil
	}
	_, err = f.service.messageQueue.QueueMessage(t.Context(), f.parentID, f.taskID, "ordinary parent successor", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	f.service.handleAgentReady(t.Context(), f.event)
	select {
	case prompt := <-dispatched:
		require.Contains(t, prompt, "ordinary parent successor")
	case <-time.After(5 * time.Second):
		t.Fatal("parent successor did not dispatch")
	}
	f.taskRepo.mu.Lock()
	history := append([]v1.TaskState(nil), f.taskRepo.stateHistory[f.taskID]...)
	f.taskRepo.mu.Unlock()
	require.NotContains(t, history, v1.TaskStateReview)
}
