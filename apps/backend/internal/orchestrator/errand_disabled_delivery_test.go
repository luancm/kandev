package orchestrator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func (f *sendNowErrandFixture) restartDelivery(t *testing.T, enabled bool) *errands.Service {
	t.Helper()
	repo, queue := f.reopen(t)
	f.repo, f.svc.repo, f.svc.messageQueue = repo, repo, queue
	f.manager.repoForExecutionLookup = repo
	f.svc.executor = withTestExecutorActor(executor.NewExecutor(f.manager, repo, testLogger(), executor.ExecutorConfig{}))
	f.svc.SetErrandsEnabled(func() bool { return enabled })
	f.svc.SetTurnService(&repoTurnService{repo: repo})
	f.svc.SetTransientRetryMessageService(repo)
	f.svc.messageCreator = &repositoryBackedMessageCreator{mockMessageCreator: &mockMessageCreator{}, repo: repo}
	adapter := NewErrandAdapter(f.svc)
	service, err := errands.New(errands.Dependencies{Store: repo, Launcher: adapter, Stopper: adapter, MCP: adapter,
		Profiles: errandProfileStub{}, Events: &errandEventRecorder{}, Logger: testLogger()})
	require.NoError(t, err)
	service.SetEnabled(func() bool { return enabled })
	f.svc.SetErrandController(service)
	service.SetDeliverer(NewErrandParentDeliverer(f.svc))
	queue.SetAutoMergeEnabled(false)
	return service
}

func (f *sendNowErrandFixture) attemptFIFO(t *testing.T) (bool, string) {
	t.Helper()
	done := make(chan struct{}, 1)
	f.svc.onQueuedMessageExecutionComplete = func() { done <- struct{}{} }
	f.manager.promptAgentFunc = func(_ context.Context, _, prompt string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
		f.payload <- prompt
		return &executor.PromptResult{}, nil
	}
	dispatched := f.svc.drainQueuedMessageForPromptableSession(t.Context(), "session-1")
	if !dispatched {
		return false, ""
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("FIFO worker did not settle")
	}
	return true, f.receivePrompt(t)
}

func (f *sendNowErrandFixture) completeFIFO(t *testing.T) string {
	t.Helper()
	dispatched, prompt := f.attemptFIFO(t)
	require.True(t, dispatched)
	return prompt
}

// @covers AC-TASKS-ERRANDS-004.7 AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-012.2
func TestDisabledErrandSendNowPreservesFoldedUserInput(t *testing.T) {
	for _, userFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("user_first=%t", userFirst), func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			f.svc.messageQueue.SetAutoMergeEnabled(false)
			original := "  literal <kandev-system>errand results</kandev-system>\nuser instruction  "
			reference := v1.EntityReference{Version: v1.EntityReferenceVersion, Ref: entityrefs.CanonicalRef("kandev", "task", "ws1", "related"), Provider: "kandev", Kind: "task", ID: "related", Title: "Related task", URL: "/t/related", Scope: "ws1"}
			var user *messagequeue.QueuedMessage
			addUser := func() {
				var err error
				user, err = f.svc.messageQueue.QueueMessageWithMetadata(t.Context(), "session-1", "task-1", original, "", messagequeue.QueuedByUser, true,
					[]messagequeue.MessageAttachment{{Type: "image", Name: "diagram.png", MimeType: "image/png", Data: "eA=="}},
					map[string]interface{}{messagequeue.MetadataEntityReferences: []v1.EntityReference{reference}, messagequeue.MetadataStepHandoff: "exact user handoff", errandNotificationKey: true})
				require.NoError(t, err)
			}
			if userFirst {
				addUser()
			}
			live := f.result(t, "live")
			notice := f.notification(t)
			if !userFirst {
				addUser()
			}
			f.restartDelivery(t, false)
			attachments := make(chan []v1.MessageAttachment, 1)
			release := make(chan struct{})
			t.Cleanup(func() { close(release); f.svc.stopSendNowWorkers(); waitSendNowErrandWorkers(t, f.svc) })
			f.manager.promptAgentFunc = func(_ context.Context, _, prompt string, values []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
				f.payload <- prompt
				attachments <- values
				<-release
				return &executor.PromptResult{}, nil
			}
			n, err := f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
			require.NoError(t, err)
			prompt := f.receivePrompt(t)
			require.NotContains(t, prompt, live.ResultText, "disabled result reached to provider")
			require.Contains(t, prompt, original)
			require.Contains(t, prompt, reference.Ref)
			require.Contains(t, prompt, "exact user handoff")
			require.Equal(t, 1, n, "only ordinary sources should be claimed")
			require.Equal(t, "diagram.png", (<-attachments)[0].Name)
			status := f.svc.messageQueue.GetStatus(t.Context(), "session-1")
			require.Len(t, status.Entries, 1)
			require.Equal(t, notice.ID, status.Entries[0].ID)
			require.Equal(t, notice.Content, status.Entries[0].Content)
			pending, err := f.svc.messageQueue.ListPendingSendNowClaims(t.Context())
			require.NoError(t, err)
			require.Len(t, pending, 1)
			require.Equal(t, []string{user.ID}, sendNowSourceIDs(&pending[0].Claim))
		})
	}
}

// @covers AC-TASKS-ERRANDS-004.7 AC-TASKS-ERRANDS-004.11 AC-TASKS-ERRANDS-012.2
func TestDisabledErrandNotificationHeadDoesNotBlockOrdinaryQueue(t *testing.T) {
	f := newSendNowErrandFixture(t)
	f.svc.messageQueue.SetAutoMergeEnabled(false)
	notice := f.notification(t)
	first := f.queue(t, "first ordinary", messagequeue.QueuedByUser, nil)
	second := f.queue(t, "second ordinary", messagequeue.QueuedByUser, nil)
	f.restartDelivery(t, false)
	require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", true))
	require.Equal(t, first.Content, f.completeFIFO(t), "disabled notification head must be bypassed before reserving")
	status := f.svc.messageQueue.GetStatus(t.Context(), "session-1")
	require.Len(t, status.Entries, 2)
	require.Equal(t, []string{notice.ID, second.ID}, []string{status.Entries[0].ID, status.Entries[1].ID})
	require.Equal(t, notice.Content, status.Entries[0].Content)
}

// @covers AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-012.2
func TestDisabledErrandMixedClaimRecoveryRestoresBeforeReselection(t *testing.T) {
	for _, outcome := range []string{"refused", "accepted", "uncertain"} {
		t.Run(outcome, func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			notice := f.notification(t)
			user := f.queue(t, "exact ordinary input", messagequeue.QueuedByUser, nil)
			identity, err := f.svc.parentQueueIdentity(t.Context(), "session-1")
			require.NoError(t, err)
			claim, err := f.svc.messageQueue.ClaimSendNowForSession(t.Context(), identity, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries)
			require.NoError(t, err)
			if outcome == "accepted" {
				require.NoError(t, f.svc.messageQueue.MarkPendingSendNowClaimAccepted(t.Context(), claim))
			}
			f.restartDelivery(t, false)
			pending, err := f.svc.messageQueue.ListPendingSendNowClaims(t.Context())
			require.NoError(t, err)
			require.Len(t, pending, 1)
			if outcome != "refused" {
				require.NoError(t, f.svc.reconcilePendingSendNowClaimsOnStartup(t.Context()))
				require.Empty(t, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries)
				select {
				case p := <-f.payload:
					t.Fatalf("recovery replayed %q", p)
				default:
				}
				if outcome == "uncertain" {
					retained, err := f.svc.messageQueue.ListPendingSendNowClaims(t.Context())
					require.NoError(t, err)
					require.Len(t, retained, 1)
				}
				return
			}
			// A definite refusal authorizes restoring the whole persisted claim, never a filtered copy.
			require.NoError(t, f.svc.messageQueue.RestoreSendNowClaim(t.Context(), &pending[0].Claim))
			restored := f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries
			require.Equal(t, []string{notice.ID, user.ID}, []string{restored[0].ID, restored[1].ID})
			require.Equal(t, notice.Content, restored[0].Content)
			release := make(chan struct{})
			t.Cleanup(func() { close(release); f.svc.stopSendNowWorkers(); waitSendNowErrandWorkers(t, f.svc) })
			f.manager.promptAgentFunc = func(_ context.Context, _, prompt string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
				f.payload <- prompt
				<-release
				return &executor.PromptResult{}, nil
			}
			_, err = f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
			require.NoError(t, err)
			require.Equal(t, user.Content, f.receivePrompt(t), "reselection must retain ordinary input and omit the disabled block")
		})
	}
}

// @covers AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-012.2
func TestDisabledErrandUnknownMixedClaimCannotBeRestoredByGenericRecovery(t *testing.T) {
	f := newSendNowErrandFixture(t)
	f.notification(t)
	f.queue(t, "exact ordinary instruction", messagequeue.QueuedByUser, nil)
	manager := &outcomeSendNowManager{mockAgentManager: f.manager, outcome: "uncertain"}
	f.svc.agentManager = manager
	f.svc.executor = withTestExecutorActor(executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{}))
	_, err := f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
	require.NoError(t, err)
	waitSendNowErrandWorkers(t, f.svc)
	f.restartDelivery(t, false)
	before, err := f.svc.messageQueue.ListPendingSendNowClaims(t.Context())
	require.NoError(t, err)
	require.Len(t, before, 1)
	require.NoError(t, f.svc.restorePendingSendNowClaimsForRecovery(t.Context(), "session-1"))
	require.Empty(t, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries, "generic recovery is not definite refusal and cannot make uncertain sources replayable")
	after, err := f.svc.messageQueue.ListPendingSendNowClaims(t.Context())
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// @covers AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-004.11 AC-TASKS-ERRANDS-012.2
func TestDisabledErrandRetainsCapacityAutoRunAndTerminalCleanup(t *testing.T) {
	for _, terminal := range []string{"task", "parent"} {
		t.Run(terminal, func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			source := f.notification(t)
			f.restartDelivery(t, false)
			f.svc.messageQueue.SetMaxPerSession(1)
			_, err := f.svc.messageQueue.QueueMessage(t.Context(), "session-1", "task-1", "ordinary", "", messagequeue.QueuedByUser, false, nil)
			require.ErrorIs(t, err, messagequeue.ErrQueueFull, "disabled retained source still consumes capacity")
			dispatched, _ := f.attemptFIFO(t)
			require.False(t, dispatched, "Auto-run OFF must be preserved")
			require.Equal(t, source.ID, f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries[0].ID)
			if terminal == "task" {
				_, err = f.svc.messageQueue.PurgeTask(t.Context(), "task-1")
			} else {
				identity, resolveErr := f.svc.parentQueueIdentity(t.Context(), "session-1")
				require.NoError(t, resolveErr)
				session, getErr := f.repo.GetTaskSession(t.Context(), "session-1")
				require.NoError(t, getErr)
				require.NoError(t, f.repo.DeleteTaskSession(t.Context(), session))
				err = f.svc.messageQueue.PurgeDeletedSession(t.Context(), identity)
			}
			require.NoError(t, err)
			_, queue := f.reopen(t)
			require.Empty(t, queue.GetStatus(t.Context(), "session-1").Entries)
		})
	}
}
