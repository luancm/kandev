package orchestrator

import (
	"context"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestSendNowCreatedClaimWaitsForPreparedAdmission(t *testing.T) {
	for _, outcome := range []string{"accepted", "sql_failure", "shutdown_before_admission", "shutdown_after_admission"} {
		t.Run(outcome, func(t *testing.T) {
			ctx := t.Context()
			f := newSendNowErrandFixture(t)
			repo, svc, queue := f.repo, f.svc, f.svc.messageQueue
			session, err := repo.GetTaskSession(ctx, "session-1")
			require.NoError(t, err)
			session.State, session.AgentProfileID = models.TaskSessionStateCreated, "profile"
			require.NoError(t, repo.UpdateTaskSession(ctx, session))
			f.manager.isAgentRunning = false
			manager := &retentionInitialPromptManager{durableDeliveryTestAgentManager: &durableDeliveryTestAgentManager{
				mockAgentManager: f.manager, advertised: true,
				capability: agentruntime.DurableDeliveryCapability{Version: journal.CurrentVersion, Durable: true},
			}}
			steps := newMockStepGetter()
			steps.steps["step-1"] = &wfmodels.WorkflowStep{ID: "step-1", WorkflowID: "wf1", Name: "Step"}
			steps.workflowAgentProfileID = "profile"
			tasks := newMockTaskRepo()
			seedMockTaskState(tasks, "task-1", v1.TaskStateInProgress)
			configured := createTestServiceWithScheduler(repo, steps, tasks, manager)
			svc.agentManager, svc.executor, svc.scheduler = manager, configured.executor, configured.scheduler
			svc.workflowStepGetter, svc.taskRepo = steps, tasks
			f.realComposer(t)
			live := f.result(t, "completed-result")
			started := make(chan struct{}, 1)
			manager.startAgentProcessFunc = func(context.Context, string) error { started <- struct{}{}; return nil }
			identity, err := svc.parentQueueIdentity(ctx, "session-1")
			require.NoError(t, err)
			source := f.queue(t, "queued instruction", messagequeue.QueuedByUser, nil)
			claim, err := queue.ClaimSendNowForSession(ctx, identity, []messagequeue.QueuedMessage{*source})
			require.NoError(t, err)
			_, submissionID, _ := claim.DeliverySubmission()
			if outcome == "sql_failure" {
				installRetentionInitialPromptFailure(t, repo, "SQLite", normalizeDeliverySubmissionID(submissionID))
			}
			reservation := svc.markQueuedDispatchInFlight("session-1", claim.Dispatch.ID)
			workerCtx, cancel := context.WithCancel(ctx)
			done := make(chan struct{})
			t.Cleanup(func() {
				cancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("Send Now worker did not stop")
				}
			})
			go func() { defer close(done); svc.executeSendNowClaimWithContext(workerCtx, claim, reservation) }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("initial process did not start")
			}
			cached, ok := svc.lastTurnPrompt.Load("session-1")
			require.True(t, ok, "created-session transient retry must retain its prompt")
			cachedPrompt := cached.(capturedPrompt)
			require.Contains(t, cachedPrompt.text, live.ResultText)
			require.True(t, cachedPrompt.promptAlreadyComposed)
			require.True(t, cachedPrompt.skipErrandComposition)
			pending, err := queue.ListPendingSendNowClaims(ctx)
			require.NoError(t, err)
			require.Len(t, pending, 1, "startup is not prompt acceptance")
			snapshot, _, err := errands.DecodeSendNowDelivery(pending[0].Claim.Dispatch.Metadata)
			require.NoError(t, err)
			require.Len(t, snapshot.Receipts, 1)
			token := snapshot.Receipts[0].Envelope.Token
			select {
			case <-done:
				t.Fatal("claim worker finished before admission")
			default:
			}
			if outcome == "shutdown_before_admission" {
				cancel()
			} else {
				manager.mu.Lock()
				admit, accepted, failed := manager.initialPromptAdmissionCallback, manager.initialPromptDispatchCallback, manager.initialPromptFailureCallback
				manager.mu.Unlock()
				require.NotNil(t, admit)
				err = admit(context.Background(), "runtime prepared prefix\n"+cachedPrompt.text, []v1.MessageAttachment{{Type: "resource", AttachmentID: "file", Name: "bundle-2.zip", MimeType: "application/zip", SizeBytes: 16, DeliveryMode: "prompt"}})
				switch outcome {
				case "sql_failure":
					require.Error(t, err)
					failed()
				case "shutdown_after_admission":
					require.NoError(t, err)
					cancel()
				default:
					require.NoError(t, err)
					accepted()
				}
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("claim worker did not settle")
			}
			messages, err := repo.ListMessages(ctx, "session-1")
			require.NoError(t, err)
			require.Len(t, messages, 1)
			require.Equal(t, "queued instruction", messages[0].Content)
			require.Equal(t, token, messages[0].Metadata[ErrandDeliveryTokenMetadataKey])
			row, err := repo.GetErrand(ctx, live.ID)
			require.NoError(t, err)
			require.Equal(t, live.ResultRevision, row.ResultRevision)
			if outcome == "accepted" || outcome == "shutdown_after_admission" {
				require.Equal(t, models.ErrandResultDelivered, row.ResultState, "accepted or uncertain provider admission retains ownership")
			} else {
				require.Equal(t, models.ErrandResultPending, row.ResultState, "definite refusal releases the exact live result")
			}
			pending, err = queue.ListPendingSendNowClaims(ctx)
			require.NoError(t, err)
			if outcome == "shutdown_after_admission" {
				require.Len(t, pending, 1)
				block, err := svc.GetOpenSessionRecoveryBlock(ctx, "session-1")
				require.NoError(t, err)
				require.Equal(t, "unknown_send_now_outcome", block.Reason)
			} else {
				require.Empty(t, pending)
			}
			if outcome == "shutdown_before_admission" || outcome == "shutdown_after_admission" {
				manager.mu.Lock()
				late := manager.initialPromptAdmissionCallback
				manager.mu.Unlock()
				require.ErrorIs(t, late(context.Background(), "late prompt", nil), context.Canceled)
				submissions, err := repo.ListAgentDeliverySubmissions(ctx, "session-1")
				require.NoError(t, err)
				if outcome == "shutdown_before_admission" {
					require.Empty(t, submissions)
				} else {
					require.Len(t, submissions, 1, "stopped worker cannot admit another initial prompt")
				}
			}
			status := queue.GetStatus(ctx, "session-1")
			if outcome == "sql_failure" || outcome == "shutdown_before_admission" {
				require.Len(t, status.Entries, 1)
				require.Equal(t, source.ID, status.Entries[0].ID)
			} else {
				require.Empty(t, status.Entries)
			}
		})
	}
}

func TestSendNowMissingExecutionFallbackPropagatesAdmissionOutcome(t *testing.T) {
	for _, outcome := range []string{"accepted", "refused", "uncertain"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "fallback-task", "fallback-session", models.TaskSessionStateWaitingForInput)
			session, err := repo.GetTaskSession(ctx, "fallback-session")
			require.NoError(t, err)
			session.AgentProfileID = "profile-1"
			require.NoError(t, repo.UpdateTaskSession(ctx, session))
			seedExecutorRunning(t, repo, "fallback-session", "fallback-task", "stale-execution")
			manager := &retentionInitialPromptManager{durableDeliveryTestAgentManager: &durableDeliveryTestAgentManager{
				mockAgentManager: &mockAgentManager{repoForExecutionLookup: repo}, advertised: true,
				capability: agentruntime.DurableDeliveryCapability{Version: journal.CurrentVersion, Durable: true},
			}}
			started := make(chan struct{}, 1)
			manager.startAgentProcessFunc = func(context.Context, string) error { started <- struct{}{}; return nil }
			tasks := newMockTaskRepo()
			seedMockTaskState(tasks, "fallback-task", v1.TaskStateInProgress)
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), tasks, manager)
			queue, queueDB := newWorkflowTransferQueue(t, t.TempDir()+"/queue.db")
			t.Cleanup(func() { require.NoError(t, queueDB.Close()) })
			svc.messageQueue = queue
			require.NoError(t, queue.SetAutoRun(ctx, "fallback-session", false))
			source, err := queue.QueueMessage(ctx, "fallback-session", "fallback-task", "Inspect this result.", "", messagequeue.QueuedByUser, false, nil)
			require.NoError(t, err)
			claim, err := queue.ClaimSendNow(ctx, "fallback-session", []messagequeue.QueuedMessage{*source})
			require.NoError(t, err)
			protocol, submissionID, payloadHash := claim.DeliverySubmission()
			options := startCreatedSessionOptions{
				deliveryProtocol:     protocol,
				deliverySubmissionID: submissionID,
				deliveryPayloadHash:  payloadHash,
				deliveryClaimUpdater: func(updateCtx context.Context, protocol, submissionID, payloadHash string) error {
					return queue.SetPendingSendNowClaimDelivery(updateCtx, claim, protocol, submissionID, payloadHash)
				},
				cacheEffectivePromptForTransientRetry: true,
			}
			waitForAdmission := svc.configureSendNowInitialAdmission(ctx, claim, &options)
			fallbackStarted := false
			const composedPrompt = "Inspect this result.\n\nCompleted fallback errand."
			err = svc.fallbackFreshLaunchOnMissingExecutionWithStartOptions(
				ctx, "fallback-task", "fallback-session", composedPrompt, true, composedPrompt, false,
				"", false, nil, nil, nil, false, false, &options, func() { fallbackStarted = true },
			)
			require.NoError(t, err)
			require.True(t, fallbackStarted)
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("fresh fallback launch did not reach initial prompt admission")
			}
			manager.mu.Lock()
			admit, accepted, failed := manager.initialPromptAdmissionCallback, manager.initialPromptDispatchCallback, manager.initialPromptFailureCallback
			manager.mu.Unlock()
			require.NotNil(t, admit)
			require.NotNil(t, accepted)
			require.NotNil(t, failed)
			cached, ok := svc.lastTurnPrompt.Load("fallback-session")
			require.True(t, ok)
			cachedPrompt := cached.(capturedPrompt)
			require.Contains(t, cachedPrompt.text, "Completed fallback errand.")
			require.True(t, cachedPrompt.promptAlreadyComposed)
			require.True(t, cachedPrompt.skipErrandComposition)
			switch outcome {
			case "accepted":
				require.NoError(t, admit(ctx, cachedPrompt.text, nil))
				accepted()
			case "refused":
				failed()
			case "uncertain":
				require.NoError(t, admit(ctx, cachedPrompt.text, nil))
				cancel()
			}
			admissionErr := waitForAdmission()
			switch outcome {
			case "accepted":
				require.NoError(t, admissionErr)
			case "refused":
				require.Error(t, admissionErr)
				require.NotErrorIs(t, admissionErr, errSendNowInitialPromptUncertain)
			case "uncertain":
				require.ErrorIs(t, admissionErr, errSendNowInitialPromptUncertain)
			}
		})
	}
}
