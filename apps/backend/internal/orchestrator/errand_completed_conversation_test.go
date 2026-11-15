package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-ERRAND-COMPLETION-001.1 AC-TASKS-ERRAND-COMPLETION-001.2 AC-TASKS-ERRAND-COMPLETION-001.3
func TestErrandCompletedConversationContract(t *testing.T) {
	for _, origin := range []string{"user", "parent"} {
		t.Run(origin, func(t *testing.T) {
			ctx := t.Context()
			seed := newErrandLaunchFixture(t)
			f := newCompletedConversationOwner(t, seed.repo)
			require.NoError(t, f.svc.messageQueue.SetAutoRun(ctx, seed.parent, false))
			now := time.Now().UTC()
			seedExecutorRow(t, f.repo, "contract-local", models.ExecutorTypeLocal)
			child := &models.TaskSession{ID: "contract-child", TaskID: seed.taskID, ParentSessionID: seed.parent,
				Role: models.SessionRoleErrand, State: models.TaskSessionStateRunning, AgentProfileID: "errand-profile",
				ExecutorID: "contract-local", DownstreamACPSessionID: contractNativeConversation, StartedAt: now, UpdatedAt: now}
			require.NoError(t, f.repo.CreateTaskSession(ctx, child))
			require.NoError(t, f.repo.CreateErrand(ctx, &models.Errand{ID: child.ID, TaskID: seed.taskID, ParentSessionID: seed.parent,
				Title: "capture", Objective: "original objective", AgentProfileID: child.AgentProfileID,
				FirstTurnID: "contract-first", SubmissionID: "prompt:errand:contract-first", Lifecycle: models.ErrandLifecycleLaunched,
				AwaitingResult: true, ResultState: models.ErrandResultNone, CreatedAt: now, UpdatedAt: now}))
			require.NoError(t, f.repo.CreateTurn(ctx, &models.Turn{ID: "contract-first", TaskID: seed.taskID,
				TaskSessionID: child.ID, StartedAt: now, CreatedAt: now, UpdatedAt: now}))
			require.NoError(t, f.repo.CreateMessage(ctx, &models.Message{ID: "contract-first-output", TaskID: seed.taskID,
				TaskSessionID: child.ID, TurnID: "contract-first", AuthorType: models.MessageAuthorAgent,
				Content: "first delegated answer", CreatedAt: now}))
			sibling := &models.TaskSession{ID: "ordinary-sibling", TaskID: seed.taskID,
				State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now}
			require.NoError(t, f.repo.CreateTaskSession(ctx, sibling))
			f.retainExecution(t, child.ID, "contract-first-execution")
			f.retainExecution(t, seed.parent, "contract-parent-execution")
			f.retainExecution(t, sibling.ID, "contract-sibling-execution")
			taskBefore, err := f.repo.GetTask(ctx, seed.taskID)
			require.NoError(t, err)
			parentBefore, err := f.repo.GetTaskSession(ctx, seed.parent)
			require.NoError(t, err)
			siblingBefore, err := f.repo.GetTaskSession(ctx, sibling.ID)
			require.NoError(t, err)
			gate := f.observeClose(t, child.ID)

			f.publishReady(t, child.ID, "contract-first-execution", "contract-first")
			firstStop := f.receiveClose(t, gate, child.ID)
			require.Equal(t, "contract-first-execution", firstStop.call.ExecutionID)
			require.Equal(t, completedExecutionCleanupReason, firstStop.call.Reason)
			require.Equal(t, "first delegated answer", firstStop.errand.ResultText)
			require.Equal(t, models.ErrandResultPending, firstStop.errand.ResultState)
			requireConversationDecision(t, f, child, "contract-first", "parent:done")
			beforeClose, err := f.repo.GetTaskSession(ctx, child.ID)
			require.NoError(t, err)
			require.NotEqual(t, models.TaskSessionStateCompleted, beforeClose.State, "stop entry is not acknowledgement")
			require.True(t, f.manager.IsAgentRunningForSession(ctx, child.ID))
			gate.allow()
			f.requireCompleted(t, child.ID, "contract-first")
			require.False(t, f.manager.IsAgentRunningForSession(ctx, child.ID))
			taskAfter, err := f.repo.GetTask(ctx, seed.taskID)
			require.NoError(t, err)
			require.Equal(t, taskBefore, taskAfter, "child close leaves task and workflow state intact")
			parentAfter, err := f.repo.GetTaskSession(ctx, seed.parent)
			require.NoError(t, err)
			require.Equal(t, parentBefore, parentAfter, "parent receipt is not needed for child close")
			siblingAfter, err := f.repo.GetTaskSession(ctx, sibling.ID)
			require.NoError(t, err)
			require.Equal(t, siblingBefore, siblingAfter)
			require.True(t, f.manager.IsAgentRunningForSession(ctx, sibling.ID))

			beforeDelivery := parentTerminalIdentityNotificationMetric(t)
			f.deliverOne(t, seed.parent, "first delegated answer")
			require.Equal(t, beforeDelivery+1, parentTerminalIdentityNotificationMetric(t))
			firstResult, err := f.repo.GetErrand(ctx, child.ID)
			require.NoError(t, err)
			require.Equal(t, models.ErrandResultDelivered, firstResult.ResultState)
			require.Equal(t, models.ErrandDeliveryNotification, firstResult.DeliveredRoute)
			parentTurns, err := f.repo.ListTurnsBySession(ctx, seed.parent)
			require.NoError(t, err)
			require.Len(t, parentTurns, 1)
			f.publishReady(t, seed.parent, "contract-parent-execution", parentTurns[0].ID)
			require.NoError(t, f.svc.messageQueue.SetAutoRun(ctx, seed.parent, false))
			f.continueCompleted(t, child, origin, firstResult, gate)
			require.True(t, f.manager.IsAgentRunningForSession(ctx, seed.parent))
			require.True(t, f.manager.IsAgentRunningForSession(ctx, sibling.ID))
		})
	}
}

func (f *completedConversationOwner) continueCompleted(t *testing.T, child *models.TaskSession, origin string, firstResult *models.Errand, gate *errandCompletionStopGate) {
	t.Helper()
	ctx := t.Context()
	taskBefore, err := f.repo.GetTask(ctx, child.TaskID)
	require.NoError(t, err)
	if origin == "parent" {
		ctx = errands.WithParentPromptOrigin(ctx, errands.ParentPromptOrigin{
			SenderSessionID: child.ParentSessionID, SubmissionID: "contract-parent-follow-up",
		})
	}
	result, err := f.svc.PromptTask(ctx, child.TaskID, child.ID, "new instruction only", "", false, nil, false)
	require.NoError(t, err)
	require.NotEmpty(t, result.TurnID)
	require.NotEqual(t, "contract-first", result.TurnID)
	prompt := f.receivePrompt(t)
	require.Equal(t, "restored-"+child.ID, prompt.ExecutionID)
	require.Equal(t, "new instruction only", prompt.Prompt, "continuation must not replay the objective or earlier transcript")
	f.manager.captured.Lock()
	launches := append([]*executor.LaunchAgentRequest(nil), f.manager.launchedWith...)
	f.manager.captured.Unlock()
	require.Len(t, launches, 1)
	require.Equal(t, child.ID, launches[0].SessionID)
	require.Equal(t, contractNativeConversation, launches[0].ACPSessionID)
	require.Empty(t, launches[0].TaskDescription, "restore does not carry an extra initial prompt")
	resumed, err := f.repo.GetTaskSession(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, child.QueueIncarnationID, resumed.QueueIncarnationID)
	require.Equal(t, child.ParentSessionID, resumed.ParentSessionID)
	require.Equal(t, contractNativeConversation, resumed.DownstreamACPSessionID)
	require.NoError(t, f.repo.CreateMessage(ctx, &models.Message{ID: "contract-second-output", TaskID: child.TaskID,
		TaskSessionID: child.ID, TurnID: result.TurnID, AuthorType: models.MessageAuthorAgent,
		Content: "second conversation answer", CreatedAt: time.Now().UTC()}))
	f.publishReady(t, child.ID, prompt.ExecutionID, result.TurnID)
	requireConversationDecision(t, f, child, result.TurnID, origin+":done")
	secondStop := f.receiveClose(t, gate, child.ID)
	require.Equal(t, result.TurnID, secondStop.errand.LatestSettledTurnID)
	require.Equal(t, prompt.ExecutionID, secondStop.call.ExecutionID)
	f.requireCompleted(t, child.ID, result.TurnID)
	require.False(t, f.manager.IsAgentRunningForSession(ctx, child.ID))
	taskAfter, err := f.repo.GetTask(ctx, child.TaskID)
	require.NoError(t, err)
	require.Equal(t, taskBefore, taskAfter, "continuation leaves the parent task and workflow intact")
	if origin == "parent" {
		require.Equal(t, result.TurnID, secondStop.errand.ResultTurnID)
		require.Equal(t, "second conversation answer", secondStop.errand.ResultText)
		f.deliverOne(t, child.ParentSessionID, "second conversation answer")
	} else {
		require.Equal(t, firstResult.ResultTurnID, secondStop.errand.ResultTurnID)
		require.Equal(t, firstResult.ResultRevision, secondStop.errand.ResultRevision)
		require.Equal(t, models.ErrandResultDelivered, secondStop.errand.ResultState)
		require.NoError(t, f.errands.SweepDelivery(ctx))
		require.NoError(t, f.svc.messageQueue.SetAutoRun(ctx, child.ParentSessionID, true))
		require.Empty(t, f.svc.messageQueue.GetStatus(ctx, child.ParentSessionID).Entries, "user follow-up has no parent result")
	}
	messages, err := f.repo.ListMessages(ctx, child.ID)
	require.NoError(t, err)
	var contents []string
	for _, message := range messages {
		contents = append(contents, message.Content)
	}
	require.Contains(t, contents, "first delegated answer")
	require.Contains(t, contents, "second conversation answer")
	select {
	case extra := <-f.prompts:
		t.Fatalf("unexpected provider dispatch: %+v", extra)
	default:
	}
}

func (f *completedConversationOwner) observeClose(t *testing.T, childID string) *errandCompletionStopGate {
	t.Helper()
	gate := &errandCompletionStopGate{entered: make(chan errandCompletionStopObservation, 4),
		release: make(chan struct{}), returned: make(chan stopAgentCall, 4)}
	t.Cleanup(gate.allow)
	stop := f.manager.stopAgentWithReasonFunc
	f.manager.stopAgentWithReasonFunc = func(ctx context.Context, executionID, reason string, force bool) error {
		row, err := f.repo.GetErrand(ctx, childID)
		call := stopAgentCall{ExecutionID: executionID, Reason: reason, Force: force}
		gate.entered <- errandCompletionStopObservation{call: call, errand: row, errandErr: err}
		select {
		case <-gate.release:
		case <-ctx.Done():
			return ctx.Err()
		}
		if err := stop(ctx, executionID, reason, force); err != nil {
			return err
		}
		gate.returned <- call
		return nil
	}
	return gate
}

func (f *completedConversationOwner) receiveClose(t *testing.T, gate *errandCompletionStopGate, childID string) errandCompletionStopObservation {
	t.Helper()
	select {
	case observed := <-gate.entered:
		require.NoError(t, observed.errandErr)
		require.NotNil(t, observed.errand)
		require.NotNil(t, observed.errand.TeardownIntent)
		require.Equal(t, models.ErrandTeardownIntentPending, observed.errand.TeardownIntent.State)
		return observed
	case <-time.After(5 * time.Second):
		row, readErr := f.repo.GetErrand(t.Context(), childID)
		session, sessionErr := f.repo.GetTaskSession(t.Context(), childID)
		retryErr := f.errands.RetryErrandCompletion(t.Context(), childID)
		t.Fatalf("settled child did not request execution close: row=%+v read_error=%v session=%+v session_error=%v retry_error=%v",
			row, readErr, session, sessionErr, retryErr)
		return errandCompletionStopObservation{}
	}
}

func requireConversationDecision(t *testing.T, f *completedConversationOwner, child *models.TaskSession, turnID, outcome string) {
	t.Helper()
	decision, err := f.repo.GetErrandTurnSettlementDecision(t.Context(), child.ID, child.QueueIncarnationID, turnID)
	require.NoError(t, err)
	require.NotNil(t, decision)
	require.Equal(t, models.DeliveryEffectCompleted, decision.State)
	require.Equal(t, outcome, decision.Outcome)
}

func (f *completedConversationOwner) requireCompleted(t *testing.T, childID, turnID string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case completed := <-f.completed:
			if completed == turnID {
				session, err := f.repo.GetTaskSession(t.Context(), childID)
				require.NoError(t, err)
				require.Equal(t, models.TaskSessionStateCompleted, session.State)
				return
			}
		case <-timer.C:
			t.Fatal("acknowledged cleanup did not publish the completed conversation")
		}
	}
}

func (f *completedConversationOwner) deliverOne(t *testing.T, parentID, result string) {
	t.Helper()
	status := f.svc.messageQueue.GetStatus(t.Context(), parentID)
	require.Len(t, status.Entries, 1)
	require.Empty(t, status.Entries[0].Content)
	require.Equal(t, messagequeue.QueuedByServer, status.Entries[0].QueuedBy)
	require.True(t, isErrandNotificationEntry(&status.Entries[0]))
	require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), parentID, true))
	identity, err := f.svc.parentQueueIdentity(t.Context(), parentID)
	require.NoError(t, err)
	f.svc.CheckQueueAdmissionReadiness(t.Context(), identity)
	prompt := f.receivePrompt(t)
	f.waitQueue(t)
	require.Equal(t, "contract-parent-execution", prompt.ExecutionID)
	require.Equal(t, 1, strings.Count(prompt.Prompt, result))
	require.Empty(t, f.svc.messageQueue.GetStatus(t.Context(), parentID).Entries)
	pending, err := f.svc.messageQueue.ListPendingQueueDispatches(t.Context())
	require.NoError(t, err)
	require.Empty(t, pending)
}
