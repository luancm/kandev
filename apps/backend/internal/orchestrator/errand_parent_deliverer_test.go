package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
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

// @covers AC-TASKS-ERRANDS-004.11
func TestErrandParentDelivererBlocksWhileQueueHoldsEntry(t *testing.T) {
	ctx := context.Background()
	svc, deliverer := newErrandDelivererFixture(t)

	require.False(t, deliverer.NotificationBlocked(ctx, "s1"), "an idle parent with an empty queue is not blocked")
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "s1", false))

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

func TestErrandParentDelivererRequestsReadinessForAdmittedNotification(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	manager := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptDone:             make(chan struct{}),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task-1", "session-1")
	require.NoError(t, err)
	require.NoError(t, svc.messageQueue.SetAutoRunForSession(ctx, identity, true))
	svc.SetErrandController(&fakeErrandComposer{composition: errands.Composition{
		Token: "delivery-token",
		Block: "Completed work result.",
		Items: []errands.DeliveryItem{{ErrandID: "errand-1", Title: "Check", Status: models.ErrandStatusDone}},
	}})
	deliverer := NewErrandParentDeliverer(svc)

	deliverer.DeliverPending(ctx, "session-1")
	select {
	case <-manager.promptDone:
	case <-time.After(2 * time.Second):
		manager.mu.Lock()
		prompts := append([]string(nil), manager.capturedPrompts...)
		manager.mu.Unlock()
		t.Fatalf("idle Auto-run parent was not prompted after notification admission: queue=%+v prompts=%q routes=%v", svc.messageQueue.GetStatus(ctx, "session-1"), prompts, svc.errands.(*fakeErrandComposer).routes)
	}

	manager.mu.Lock()
	prompts := append([]string(nil), manager.capturedPrompts...)
	manager.mu.Unlock()
	require.Len(t, prompts, 1)
	require.Contains(t, prompts[0], "Completed work result.")
	require.Empty(t, svc.messageQueue.GetStatus(ctx, "session-1").Entries)
}

func TestErrandParentDelivererLeavesNotificationQueuedWhenAutoRunIsOff(t *testing.T) {
	ctx := context.Background()
	svc, deliverer := newErrandDelivererFixture(t)
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "s1", false))

	deliverer.DeliverPending(ctx, "s1")

	status := svc.messageQueue.GetStatus(ctx, "s1")
	require.False(t, status.AutoRun)
	require.Len(t, status.Entries, 1)
	require.True(t, isErrandNotificationEntry(&status.Entries[0]))
}

func TestSendNowClaimDetectsNotificationAmongFoldedSources(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-send-now", "session-send-now", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session-send-now", "task-send-now", "exec-send-now")
	manager := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}
	svc.SetErrandController(&fakeErrandComposer{composition: errands.Composition{
		Token: "send-now-token",
		Block: "Completed work result.",
		Items: []errands.DeliveryItem{{ErrandID: "errand-1", Title: "Check", Status: models.ErrandStatusDone}},
	}})
	ordinary, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "session-send-now", "task-send-now", "Please review this.", "", messagequeue.QueuedByUser, false, nil, nil,
	)
	require.NoError(t, err)
	notification, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "session-send-now", "task-send-now", "", "", messagequeue.QueuedByServer, false, nil,
		map[string]interface{}{errandNotificationKey: true},
	)
	require.NoError(t, err)
	claim, err := svc.messageQueue.ClaimSendNow(
		ctx, "session-send-now", []messagequeue.QueuedMessage{*ordinary, *notification},
	)
	require.NoError(t, err)
	reservation := svc.markQueuedDispatchInFlight("session-send-now", claim.Dispatch.ID)

	svc.executeSendNowClaimWithContext(ctx, claim, reservation)

	manager.mu.Lock()
	prompts := append([]string(nil), manager.capturedPrompts...)
	manager.mu.Unlock()
	require.Len(t, prompts, 1)
	require.Contains(t, prompts[0], "Please review this.")
	require.Contains(t, prompts[0], "Completed work result.")
	require.Len(t, svc.messageCreator.(*mockMessageCreator).userMessages, 1)
	userMessage := svc.messageCreator.(*mockMessageCreator).userMessages[0]
	require.Equal(t, "Please review this.", userMessage.content, "Send Now keeps the user's text as the visible message")
	require.Equal(t, "send-now-token", userMessage.metadata[ErrandDeliveryTokenMetadataKey])
	require.Empty(t, svc.messageQueue.GetStatus(ctx, "session-send-now").Entries)
}

func TestSendNowConsumesNotificationWithoutStartingEmptyTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-empty-notification", "session-empty-notification", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session-empty-notification", "task-empty-notification", "exec-empty-notification")
	manager := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	svc.SetErrandController(&fakeErrandComposer{})
	notification, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "session-empty-notification", "task-empty-notification", "", "", messagequeue.QueuedByServer, false, nil,
		map[string]interface{}{errandNotificationKey: true},
	)
	require.NoError(t, err)
	claim, err := svc.messageQueue.ClaimSendNow(
		ctx, "session-empty-notification", []messagequeue.QueuedMessage{*notification},
	)
	require.NoError(t, err)
	reservation := svc.markQueuedDispatchInFlight("session-empty-notification", claim.Dispatch.ID)

	svc.executeSendNowClaimWithContext(ctx, claim, reservation)

	manager.mu.Lock()
	prompts := append([]string(nil), manager.capturedPrompts...)
	manager.mu.Unlock()
	require.Empty(t, prompts)
	require.Zero(t, svc.messageQueue.GetStatus(ctx, "session-empty-notification").Count)
}
