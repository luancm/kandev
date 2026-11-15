package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-004.11
func TestErrandNotificationAdmissionDoesNotReenterCompletionGuard(t *testing.T) {
	for _, kind := range []string{"workflow", "cancellation"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
			seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
			manager := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo, promptDone: make(chan struct{})}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			svc.executor = withTestExecutorActor(executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{}))
			svc.messageCreator = &mockMessageCreator{}
			identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task-1", "session-1")
			require.NoError(t, err)
			require.NoError(t, svc.messageQueue.SetAutoRunForSession(ctx, identity, true))
			svc.SetErrandController(&fakeErrandComposer{composition: errands.Composition{
				Token: "guarded-delivery", Block: "Completed work result.",
				Items: []errands.DeliveryItem{{ErrandID: "errand-1", Title: "Check", Status: models.ErrandStatusDone}},
			}})
			guarded := withWorkflowProfileSwitchGuardHeld(ctx, "session-1", "exec-1")
			if kind == "cancellation" {
				guarded = withCancelInFlightGuardHeldForSession(ctx, "session-1")
			}
			lock, release := svc.acquireCancelInFlightGuard("session-1")
			defer release()
			lock.Lock()
			done := make(chan struct{})
			go func() { NewErrandParentDeliverer(svc).DeliverPending(guarded, "session-1"); close(done) }()
			returned := false
			select {
			case <-done:
				returned = true
			case <-time.After(time.Second):
			}
			// Release even on regression so the blocked worker cannot leak into cleanup.
			lock.Unlock()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("admission did not finish after guard release")
			}
			require.True(t, returned, "notification admission reacquired its caller's completion guard")
			select {
			case <-manager.promptDone:
				t.Fatal("notification dispatched before the completion owner drained")
			default:
			}
			require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, "session-1").Count)
			svc.CheckQueueAdmissionReadiness(ctx, identity)
			select {
			case <-manager.promptDone:
			case <-time.After(2 * time.Second):
				t.Fatal("queued result did not dispatch after completion released the guard")
			}
			require.Zero(t, svc.messageQueue.GetStatus(ctx, "session-1").Count)
		})
	}
}

// @covers AC-TASKS-ERRANDS-004.11
func TestErrandNotificationAdmissionDoesNotBypassReadinessForOtherSessionGuard(t *testing.T) {
	for _, kind := range []string{"workflow", "cancellation"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
			seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
			manager := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo, promptDone: make(chan struct{})}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			svc.executor = withTestExecutorActor(executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{}))
			svc.messageCreator = &mockMessageCreator{}
			identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task-1", "session-1")
			require.NoError(t, err)
			require.NoError(t, svc.messageQueue.SetAutoRunForSession(ctx, identity, true))
			svc.SetErrandController(&fakeErrandComposer{composition: errands.Composition{
				Token: "unrelated-guard-delivery", Block: "Completed work result.",
				Items: []errands.DeliveryItem{{ErrandID: "errand-1", Title: "Check", Status: models.ErrandStatusDone}},
			}})

			guarded := withWorkflowProfileSwitchGuardHeld(ctx, "other-session", "other-exec")
			if kind == "cancellation" {
				guarded = withCancelInFlightGuardHeldForSession(ctx, "other-session")
			}
			lock, release := svc.acquireCancelInFlightGuard("other-session")
			lock.Lock()
			defer func() {
				lock.Unlock()
				release()
			}()

			NewErrandParentDeliverer(svc).DeliverPending(guarded, "session-1")
			select {
			case <-manager.promptDone:
			case <-time.After(2 * time.Second):
				t.Fatal("unrelated session guard bypassed the readiness recheck")
			}
			manager.mu.Lock()
			prompts := append([]string(nil), manager.capturedPrompts...)
			manager.mu.Unlock()
			require.Len(t, prompts, 1)
			require.Contains(t, prompts[0], "Completed work result.")
		})
	}
}
