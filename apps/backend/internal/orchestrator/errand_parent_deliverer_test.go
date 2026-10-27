package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func newErrandDelivererFixture(t *testing.T) (*Service, *ErrandParentDeliverer) {
	t.Helper()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	return svc, NewErrandParentDeliverer(svc)
}

func TestErrandParentDelivererBlocksWhileQueueHoldsEntry(t *testing.T) {
	ctx := context.Background()
	svc, deliverer := newErrandDelivererFixture(t)

	require.False(t, deliverer.NotificationBlocked(ctx, "s1"), "an idle parent with an empty queue is not blocked")

	require.NoError(t, deliverer.AdmitPrepared(ctx, "s1", "status block", map[string]interface{}{
		"errand_delivery": string(models.ErrandDeliveryNotification),
	}))
	require.True(t, deliverer.NotificationBlocked(ctx, "s1"), "a waiting entry blocks a second notification")

	status := svc.messageQueue.GetStatus(ctx, "s1")
	require.Equal(t, 1, status.Count)
	require.Equal(t, "status block", status.Entries[0].Content)
	require.Equal(t, messagequeue.QueuedByServer, status.Entries[0].QueuedBy)
	require.Equal(t, string(models.ErrandDeliveryNotification), status.Entries[0].Metadata["errand_delivery"])
}

func TestErrandParentDelivererBlocksWhileTurnRuns(t *testing.T) {
	ctx := context.Background()
	svc, deliverer := newErrandDelivererFixture(t)
	svc.activeTurns.Store("s1", "turn-1")

	require.True(t, deliverer.NotificationBlocked(ctx, "s1"), "a running parent turn blocks notification")
}

func TestErrandParentDelivererRejectsUnknownParent(t *testing.T) {
	ctx := context.Background()
	_, deliverer := newErrandDelivererFixture(t)

	require.Error(t, deliverer.AdmitPrepared(ctx, "missing", "block", nil))
}
