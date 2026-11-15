package orchestrator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-004.7 AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-012.2
func TestDisabledErrandSendNowCreatedAndFallbackKeepOrdinaryInput(t *testing.T) {
	for _, protocol := range []string{messagequeue.DeliveryProtocolLegacy, messagequeue.DeliveryProtocolV1} {
		for _, path := range []string{"created", "missing_execution"} {
			t.Run(protocol+"/"+path, func(t *testing.T) {
				f := newSendNowErrandFixture(t)
				prepared := f.prepared(t, "prepared-token", "deleted", "complete prepared output")
				f.restartDelivery(t, false)
				release := make(chan struct{})
				t.Cleanup(func() { close(release); f.svc.stopSendNowWorkers(); waitSendNowErrandWorkers(t, f.svc) })
				f.manager.promptAgentFunc = func(_ context.Context, _, prompt string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
					f.payload <- prompt
					<-release
					return &executor.PromptResult{}, nil
				}
				manager := installTask14CanonicalManager(t, f, protocol, path)
				attachment := messagequeue.MessageAttachment{Type: "image", Name: "diagram.png", MimeType: "image/png", Data: "eA=="}
				user, err := f.svc.messageQueue.QueueMessageWithMetadata(t.Context(), "session-1", "task-1", "  exact ordinary input\n ", "", messagequeue.QueuedByUser, false, []messagequeue.MessageAttachment{attachment}, nil)
				require.NoError(t, err)
				n, err := f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
				require.NoError(t, err)
				require.Equal(t, 1, n)
				prompt := f.receivePrompt(t)
				input := <-manager.incoming
				require.Contains(t, prompt, user.Content)
				require.NotContains(t, prompt, prepared.Content)
				require.Equal(t, queuedMessageAttachmentsToV1(user.Attachments), input.Attachments)
				_, queue := f.reopen(t)
				entries := queue.GetStatus(t.Context(), "session-1").Entries
				require.Equal(t, []messagequeue.QueuedMessage{*prepared}, entries)
				pending, err := queue.ListPendingSendNowClaims(t.Context())
				require.NoError(t, err)
				require.Len(t, pending, 1)
				require.Equal(t, []string{user.ID}, sendNowSourceIDs(&pending[0].Claim))
			})
		}
	}
}

// @covers AC-TASKS-ERRANDS-004.10 AC-TASKS-ERRANDS-012.2
func TestDisabledErrandNotificationOnlySendNowNeverInterruptsActiveTurn(t *testing.T) {
	for _, prepared := range []bool{false, true} {
		t.Run(fmt.Sprintf("prepared=%t", prepared), func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			if prepared {
				f.prepared(t, "token", "deleted", "full cancellation notice")
			} else {
				f.queue(t, "", messagequeue.QueuedByServer, map[string]interface{}{errandNotificationKey: true})
			}
			f.restartDelivery(t, false)
			require.NoError(t, f.repo.CreateTurn(t.Context(), &models.Turn{ID: "active-turn", TaskID: "task-1", TaskSessionID: "session-1", StartedAt: time.Now().UTC()}))
			f.manager.currentPromptExecutionID = "exec-1"
			f.manager.currentPromptGeneration.Store(3)
			f.manager.currentPromptActivityEpoch.Store(7)
			f.svc.activeTurns.Store("session-1", "active-turn")
			before := f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries
			for _, scope := range []string{QueueSendNowScopeEntry, QueueSendNowScopeAll} {
				id := ""
				if scope == QueueSendNowScopeEntry {
					id = before[0].ID
				}
				n, err := f.svc.SendQueuedNow(t.Context(), "session-1", scope, id)
				require.ErrorIs(t, err, ErrSendNowQueueEmpty)
				require.Zero(t, n)
			}
			require.Zero(t, f.manager.cancelAgentForPromptCalls.Load()+f.manager.cancelAgentCalls.Load())
			active, err := f.repo.GetActiveTurnBySessionID(t.Context(), "session-1")
			require.NoError(t, err)
			require.Equal(t, "active-turn", active.ID)
			require.Equal(t, before, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries)
			select {
			case p := <-f.payload:
				t.Fatalf("disabled source reached provider: %q", p)
			default:
			}
			// The same active-turn/provider setup accepts ordinary input and cancels once.
			f.queue(t, "ordinary replacement", messagequeue.QueuedByUser, nil)
			release := make(chan struct{})
			t.Cleanup(func() { close(release); f.svc.stopSendNowWorkers(); waitSendNowErrandWorkers(t, f.svc) })
			f.manager.promptAgentFunc = func(_ context.Context, _, prompt string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
				f.payload <- prompt
				<-release
				return &executor.PromptResult{}, nil
			}
			n, err := f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
			require.NoError(t, err)
			require.Equal(t, 1, n)
			require.Equal(t, "ordinary replacement", f.receivePrompt(t))
			require.EqualValues(t, 1, f.manager.cancelAgentForPromptCalls.Load()+f.manager.cancelAgentCalls.Load())
		})
	}
}

// @covers AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-012.2
func TestDisabledErrandDefiniteRefusalRestoresWholeClaimBeforeReselection(t *testing.T) {
	f := newSendNowErrandFixture(t)
	prepared := f.prepared(t, "token", "deleted", "full prepared output")
	user := f.queue(t, "  exact ordinary input\n ", messagequeue.QueuedByUser, nil)
	manager := &outcomeSendNowManager{mockAgentManager: f.manager, outcome: "refused"}
	f.svc.agentManager = manager
	f.svc.executor = withTestExecutorActor(executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{}))
	f.executeWithAutoRunPaused(t)
	f.restartDelivery(t, false)
	restored := f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries
	require.Len(t, restored, 2)
	require.Equal(t, prepared.ID, restored[0].ID)
	require.Equal(t, prepared.Content, restored[0].Content)
	require.Equal(t, user.ID, restored[1].ID)
	release := make(chan struct{})
	t.Cleanup(func() { close(release); f.svc.stopSendNowWorkers(); waitSendNowErrandWorkers(t, f.svc) })
	f.manager.promptAgentFunc = func(_ context.Context, _, prompt string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
		f.payload <- prompt
		<-release
		return &executor.PromptResult{}, nil
	}
	n, err := f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, user.Content, f.receivePrompt(t))
	require.Equal(t, prepared.ID, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries[0].ID)
}

// @covers AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-012.2
func TestDisabledErrandGenericRecoveryPreservesOrdinaryOnlyClaimBehavior(t *testing.T) {
	for _, owner := range []string{messagequeue.QueuedByUser, messagequeue.QueuedByServer, messagequeue.QueuedByAgent, messagequeue.QueuedByWorkflow} {
		t.Run(owner, func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			source := f.queue(t, "exact ordinary input", owner, nil)
			manager := &outcomeSendNowManager{mockAgentManager: f.manager, outcome: "uncertain"}
			f.svc.agentManager = manager
			f.svc.executor = withTestExecutorActor(executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{}))
			_, err := f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
			require.NoError(t, err)
			waitSendNowErrandWorkers(t, f.svc)
			f.restartDelivery(t, false)
			require.NoError(t, f.svc.restorePendingSendNowClaimsForRecovery(t.Context(), "session-1"))
			entries := f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries
			require.Len(t, entries, 1, "an ordinary-only snapshot must keep generic recovery behavior")
			require.Equal(t, source.ID, entries[0].ID)
			require.Equal(t, source.Content, entries[0].Content)
		})
	}
}
