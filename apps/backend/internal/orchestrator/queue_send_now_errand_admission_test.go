package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type task14CanonicalManager struct {
	*retentionInitialPromptManager
	initial          agentDeliveryPromptPayload
	incoming         chan agentDeliveryPromptPayload
	missingExecution bool
	resumeReady      func(context.Context) error
}

func (m *task14CanonicalManager) LaunchAgent(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	m.mu.Lock()
	m.initial = agentDeliveryPromptPayload{Text: req.TaskDescription, Attachments: req.Attachments}
	m.mu.Unlock()
	return m.mockAgentManager.LaunchAgent(ctx, req)
}

func (m *task14CanonicalManager) SetExecutionDescription(ctx context.Context, id, prompt string) error {
	m.mu.Lock()
	m.initial.Text = prompt
	m.mu.Unlock()
	return m.mockAgentManager.SetExecutionDescription(ctx, id, prompt)
}

func (m *task14CanonicalManager) StartAgentProcess(ctx context.Context, id string) error {
	m.mu.Lock()
	before, accepted, failed := m.initialPromptAdmissionCallback, m.initialPromptDispatchCallback, m.initialPromptFailureCallback
	input := m.initial
	m.mu.Unlock()
	if before == nil {
		return m.resumeReady(ctx)
	}
	final := "post-runtime-prefix\n" + input.Text + "\npost-runtime-suffix"
	m.incoming <- input
	// Runtime callbacks deliberately have no Send Now context values.
	if err := before(context.Background(), final, input.Attachments); err != nil {
		failed()
		return err
	}
	_, err := m.PromptAgent(ctx, id, final, input.Attachments, false)
	if err == nil {
		accepted()
	} else {
		failed()
	}
	return err
}

func (m *task14CanonicalManager) PromptAgentWithAdmissionCallback(ctx context.Context, id, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, before func(context.Context, string, []v1.MessageAttachment) error, accepted func()) (*executor.PromptResult, error) {
	if m.missingExecution {
		return nil, executor.ErrExecutionNotFound
	}
	final := "post-runtime-prefix\n" + prompt + "\npost-runtime-suffix"
	m.incoming <- agentDeliveryPromptPayload{Text: prompt, Attachments: attachments}
	if err := before(context.Background(), final, attachments); err != nil {
		return nil, err
	}
	return m.PromptAgentWithDispatchCallback(ctx, id, final, attachments, dispatchOnly, accepted)
}

func (m *task14CanonicalManager) PromptAgentWithAdmissionCallbackAndSubmissionID(ctx context.Context, id, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, before func(context.Context, string, []v1.MessageAttachment) error, accepted func(), _ string) (*executor.PromptResult, error) {
	return m.PromptAgentWithAdmissionCallback(ctx, id, prompt, attachments, dispatchOnly, before, accepted)
}

func installTask14CanonicalManager(t *testing.T, f *sendNowErrandFixture, protocol, path string) *task14CanonicalManager {
	t.Helper()
	manager := &task14CanonicalManager{retentionInitialPromptManager: &retentionInitialPromptManager{durableDeliveryTestAgentManager: &durableDeliveryTestAgentManager{
		mockAgentManager: f.manager, advertised: protocol == messagequeue.DeliveryProtocolV1,
		capability: agentruntime.DurableDeliveryCapability{Version: journal.CurrentProtocolVersion, Durable: true},
	}}, incoming: make(chan agentDeliveryPromptPayload, 1), missingExecution: path == "missing_execution"}
	manager.resumeReady = func(ctx context.Context) error {
		return f.repo.UpdateTaskSessionState(ctx, "session-1", models.TaskSessionStateWaitingForInput, "")
	}
	f.svc.agentManager = manager
	f.svc.executor = executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{})
	steps := newMockStepGetter()
	steps.steps["step-1"] = &wfmodels.WorkflowStep{ID: "step-1", WorkflowID: "wf1", Name: "Task14 step"}
	steps.workflowAgentProfileID = "profile"
	tasks := newMockTaskRepo()
	seedMockTaskState(tasks, "task-1", v1.TaskStateInProgress)
	f.svc.scheduler = createTestServiceWithScheduler(f.repo, steps, tasks, manager).scheduler
	f.svc.workflowStepGetter, f.svc.taskRepo = steps, tasks
	session, err := f.repo.GetTaskSession(t.Context(), "session-1")
	require.NoError(t, err)
	session.AgentProfileID = "profile"
	if path == "created" {
		session.State = models.TaskSessionStateCreated
		session.AgentExecutionID = ""
		require.NoError(t, f.repo.DeleteExecutorRunningBySessionID(t.Context(), session.ID))
	}
	require.NoError(t, f.repo.UpdateTaskSession(t.Context(), session))
	if path != "established" {
		f.manager.isAgentRunning = false
		f.manager.isAgentReadyFn = func(context.Context, string) bool { return true }
	}
	return manager
}

// @covers AC-TASKS-ERRANDS-004.7 AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-004.11
func TestSendNowErrandCanonicalAdmissionPayload(t *testing.T) {
	for _, protocol := range []string{messagequeue.DeliveryProtocolLegacy, messagequeue.DeliveryProtocolV1} {
		for _, path := range []string{"established", "created", "missing_execution"} {
			t.Run(protocol+"/"+path, func(t *testing.T) {
				f := newSendNowErrandFixture(t)
				manager := installTask14CanonicalManager(t, f, protocol, path)
				prepared := f.prepared(t, "prepared-token", "deleted", "complete prepared output")
				attachment := messagequeue.MessageAttachment{Type: "image", Name: "diagram.png", MimeType: "image/png", Data: "eA=="}
				user, err := f.svc.messageQueue.QueueMessageWithMetadata(t.Context(), "session-1", "task-1", "pre-runtime ordinary input", "", messagequeue.QueuedByUser, false, []messagequeue.MessageAttachment{attachment}, nil)
				require.NoError(t, err)
				_, err = f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
				require.NoError(t, err)
				final := f.receivePrompt(t)
				input := <-manager.incoming
				require.Equal(t, "post-runtime-prefix\n"+input.Text+"\npost-runtime-suffix", final)
				require.Contains(t, input.Text, "pre-runtime ordinary input")
				require.Contains(t, input.Text, prepared.Content)
				require.Equal(t, queuedMessageAttachmentsToV1(user.Attachments), input.Attachments)
				reopened, queue := f.reopen(t)
				pending, err := queue.ListPendingSendNowClaims(t.Context())
				require.NoError(t, err)
				require.Len(t, pending, 1)
				claim := pending[0].Claim
				require.Equal(t, protocol, claim.DeliveryProtocol)
				snapshot, present, err := errands.DecodeSendNowDelivery(claim.Dispatch.Metadata)
				require.NoError(t, err)
				require.True(t, present)
				require.Equal(t, final, snapshot.EffectivePrompt, "final runtime prompt must be durable before provider I/O")
				require.Equal(t, []string{prepared.ID, user.ID}, snapshot.SourceIDs)
				require.Equal(t, user.Attachments, claim.Sources[1].Attachments)
				messages, err := reopened.ListMessages(t.Context(), "session-1")
				require.NoError(t, err)
				require.Len(t, messages, 1)
				require.Equal(t, queuedTranscriptMessageID(&claim.Dispatch, snapshot.SourceIDs), messages[0].ID)
				stored, _, err := errands.DecodeSendNowDelivery(messages[0].Metadata)
				require.NoError(t, err)
				require.Equal(t, snapshot, stored)
				if protocol == messagequeue.DeliveryProtocolV1 {
					submission, err := reopened.GetAgentDeliverySubmission(t.Context(), normalizeDeliverySubmissionID(claim.DeliverySubmissionID))
					require.NoError(t, err)
					var payload agentDeliveryPromptPayload
					require.NoError(t, json.Unmarshal(submission.Payload, &payload))
					require.Equal(t, agentDeliveryPromptPayload{Text: final, Attachments: input.Attachments}, payload)
				}
			})
		}
	}
}

func TestSendNowErrandCanonicalPersistenceFailurePreventsProviderIO(t *testing.T) {
	for _, protocol := range []string{messagequeue.DeliveryProtocolLegacy, messagequeue.DeliveryProtocolV1} {
		for _, path := range []string{"established", "created", "missing_execution"} {
			for _, target := range []string{"claim", "transcript"} {
				t.Run(protocol+"/"+path+"/"+target, func(t *testing.T) {
					f := newSendNowErrandFixture(t)
					installTask14CanonicalManager(t, f, protocol, path)
					f.manager.promptAgentFunc = nil
					prepared := f.prepared(t, "prepared-token", "deleted", "complete prepared output")
					user := f.queue(t, "pre-runtime ordinary input", messagequeue.QueuedByUser, nil)
					_, err := f.svc.messageQueue.ListPendingSendNowClaims(t.Context())
					require.NoError(t, err)
					sql := `CREATE TRIGGER task14_reject_final BEFORE UPDATE OF claim_json ON queue_send_now_claims WHEN NEW.claim_json LIKE '%post-runtime-prefix%' BEGIN SELECT RAISE(ABORT, 'task14 final claim failure'); END`
					if target == "transcript" {
						sql = `CREATE TRIGGER task14_reject_final BEFORE UPDATE OF metadata ON task_session_messages WHEN NEW.metadata LIKE '%post-runtime-prefix%' BEGIN SELECT RAISE(ABORT, 'task14 final transcript failure'); END`
					}
					_, err = f.repo.DB().ExecContext(t.Context(), sql)
					require.NoError(t, err)
					_, err = f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
					require.NoError(t, err)
					waitSendNowErrandWorkers(t, f.svc)
					f.manager.mu.Lock()
					prompts := append([]string(nil), f.manager.capturedPrompts...)
					f.manager.mu.Unlock()
					require.Empty(t, prompts, "failed durable persistence must prevent provider I/O")
					_, queue := f.reopen(t)
					status := queue.GetStatus(t.Context(), "session-1")
					require.Len(t, status.Entries, 2)
					require.Equal(t, []string{prepared.ID, user.ID}, []string{status.Entries[0].ID, status.Entries[1].ID})
					submissions, err := f.repo.ListAgentDeliverySubmissions(t.Context(), "session-1")
					require.NoError(t, err)
					require.Empty(t, submissions)
				})
			}
		}
	}
}

// Keep the common admission boundary's ordinary callbacks, payload and refusal contract.
func TestPrepareAgentDeliverySubmissionOrdinaryClaimUpdater(t *testing.T) {
	for _, protocol := range []string{messagequeue.DeliveryProtocolLegacy, messagequeue.DeliveryProtocolV1} {
		for _, refused := range []bool{false, true} {
			t.Run(protocol+map[bool]string{false: "/success", true: "/refused"}[refused], func(t *testing.T) {
				f := newSendNowErrandFixture(t)
				installTask14CanonicalManager(t, f, protocol, "established")
				session, err := f.repo.GetTaskSession(t.Context(), "session-1")
				require.NoError(t, err)
				sentinel := errors.New("ordinary claim persistence refused")
				calls := 0
				attachments := []v1.MessageAttachment{{Type: "image", Name: "ordinary.png", MimeType: "image/png", Data: "eA=="}}
				runtime, err := f.svc.prepareAgentDeliverySubmission(t.Context(), session, "exec-1", "ordinary exact prompt", attachments, promptTaskOptions{
					deliveryProtocol: protocol, deliverySubmissionID: "ordinary",
					deliveryClaimUpdater: func(ctx context.Context, p, id, hash string) error {
						calls++
						require.Equal(t, protocol, p)
						require.Equal(t, "ordinary", id)
						require.NoError(t, ctx.Err())
						if protocol == messagequeue.DeliveryProtocolV1 {
							payload, marshalErr := marshalAgentDeliveryPromptPayload("ordinary exact prompt", attachments)
							require.NoError(t, marshalErr)
							require.Equal(t, journal.SubmissionHash(payload), hash)
						} else {
							require.Empty(t, hash)
						}
						if refused {
							return sentinel
						}
						return nil
					},
				})
				require.Equal(t, 1, calls)
				if refused {
					require.ErrorIs(t, err, sentinel)
					require.Nil(t, runtime)
				} else {
					require.NoError(t, err)
				}
				submissions, listErr := f.repo.ListAgentDeliverySubmissions(t.Context(), session.ID)
				require.NoError(t, listErr)
				if refused || protocol == messagequeue.DeliveryProtocolLegacy {
					require.Empty(t, submissions)
				} else {
					require.Len(t, submissions, 1)
					var payload agentDeliveryPromptPayload
					stored, err := f.repo.GetAgentDeliverySubmission(t.Context(), submissions[0].ID)
					require.NoError(t, err)
					require.NoError(t, json.Unmarshal(stored.Payload, &payload))
					require.Equal(t, agentDeliveryPromptPayload{Text: "ordinary exact prompt", Attachments: attachments}, payload)
				}
			})
		}
	}
}

func TestSendNowErrandProducerPreservesNonzeroGenerations(t *testing.T) {
	f := newSendNowErrandFixture(t)
	_, err := f.svc.messageQueue.PurgeTask(t.Context(), "task-1")
	require.NoError(t, err)
	identity, err := f.svc.parentQueueIdentity(t.Context(), "session-1")
	require.NoError(t, err)
	require.NoError(t, f.svc.messageQueue.SetAutoRunForSession(t.Context(), identity, false))
	for range 2 {
		_, err = f.svc.messageQueue.CancelAll(t.Context(), "session-1")
		require.NoError(t, err)
	}
	require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", false))
	sourceGeneration, err := f.svc.messageQueue.LifecycleGeneration(t.Context(), "task-1")
	require.NoError(t, err)
	require.Positive(t, sourceGeneration)
	sessionGeneration, err := f.svc.messageQueue.SessionGeneration(t.Context(), "session-1")
	require.NoError(t, err)
	require.Greater(t, sessionGeneration, sourceGeneration)
	row := &models.Errand{ID: "advanced-live", TaskID: "task-1", ParentSessionID: "session-1", Title: "Advanced live result", Objective: "check", AgentProfileID: "profile",
		FirstTurnID: "first", SubmissionID: "prompt:first", Lifecycle: models.ErrandLifecycleLaunched, LastOutcome: models.ErrandOutcomeDone,
		ResultText: "old output", ResultKind: models.ErrandResultOutput, ResultState: models.ErrandResultPending, ResultTurnID: "first", LatestSettledTurnID: "first"}
	require.NoError(t, f.repo.CreateErrand(t.Context(), row))
	for _, turn := range []string{"second", "third"} {
		admission := models.ErrandParentAdmission{TaskID: row.TaskID, ParentSessionID: row.ParentSessionID, ErrandID: row.ID, SenderSessionID: row.ParentSessionID, TurnID: turn, SubmissionID: "prompt:" + turn}
		_, err = f.repo.ReserveErrandParentAdmission(t.Context(), admission)
		require.NoError(t, err)
		require.NoError(t, f.repo.DecideErrandParentAdmission(t.Context(), admission, models.ErrandAdmissionAccepted))
	}
	row, err = f.repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, row.ParentAdmissionGeneration)
	row.AwaitingResult = false
	row.ResultText, row.ResultTurnID, row.LatestSettledTurnID = "new output after accepted successors", "third", "third"
	row.ResultState = models.ErrandResultPending
	require.NoError(t, f.repo.UpdateErrand(t.Context(), row))
	prepared := f.prepared(t, "prepared-token", "deleted", "full prepared result")
	user := f.queue(t, "ordinary input at advanced queue generation", messagequeue.QueuedByUser, nil)
	f.realComposer(t)
	_, err = f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
	require.NoError(t, err)
	prompt := f.receivePrompt(t)
	_, queue := f.reopen(t)
	pending, err := queue.ListPendingSendNowClaims(t.Context())
	require.NoError(t, err)
	require.Len(t, pending, 1)
	claim := pending[0].Claim
	snapshot, present, err := errands.DecodeSendNowDelivery(claim.Dispatch.Metadata)
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, prompt, snapshot.EffectivePrompt)
	require.Equal(t, claim.SessionGeneration, snapshot.QueueGeneration)
	require.Equal(t, claim.OperationGeneration, snapshot.OperationGeneration)
	require.Greater(t, snapshot.OperationGeneration, sessionGeneration)
	require.Equal(t, snapshot.QueueGeneration, snapshot.OperationGeneration)
	require.Equal(t, []string{prepared.ID, user.ID}, snapshot.SourceIDs)
	for _, source := range snapshot.Sources {
		require.Equal(t, sourceGeneration, source.QueueGeneration)
	}
	require.Len(t, snapshot.Receipts, 2)
	require.Equal(t, row.ResultRevision, snapshot.Receipts[1].Claims[0].ResultRevision)
	require.EqualValues(t, 2, snapshot.Receipts[1].Claims[0].ParentAdmissionGeneration)
}

func TestSendNowErrandOrdinaryInputModes(t *testing.T) {
	for _, mode := range []string{"plan", "config", "both"} {
		t.Run(mode, func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			f.realComposer(t)
			const original = "  ordinary mode input\n "
			expected := original
			if mode != "plan" {
				require.NoError(t, f.repo.UpdateSessionMetadata(t.Context(), "session-1", map[string]interface{}{"config_mode": true}))
				expected = sysprompt.InjectConfigContext("session-1", expected)
			}
			plan := mode != "config"
			if plan {
				expected = sysprompt.InjectPlanMode(expected)
			}
			source, err := f.svc.messageQueue.QueueMessageWithMetadata(t.Context(), "session-1", "task-1", original, "", messagequeue.QueuedByUser, plan, nil, nil)
			require.NoError(t, err)
			_, err = f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
			require.NoError(t, err)
			require.Equal(t, expected, f.receivePrompt(t))
			_, queue := f.reopen(t)
			pending, err := queue.ListPendingSendNowClaims(t.Context())
			require.NoError(t, err)
			require.Len(t, pending, 1)
			snapshot, _, err := errands.DecodeSendNowDelivery(pending[0].Claim.Dispatch.Metadata)
			require.NoError(t, err)
			require.Equal(t, expected, snapshot.EffectivePrompt)
			require.Equal(t, original, snapshot.Sources[0].Content)
			require.Equal(t, source.PlanMode, pending[0].Claim.Sources[0].PlanMode)
		})
	}
}

func TestSendNowErrandCanonicalAcceptedAndUncertainOwnership(t *testing.T) {
	for _, protocol := range []string{messagequeue.DeliveryProtocolLegacy, messagequeue.DeliveryProtocolV1} {
		for _, path := range []string{"established", "created", "missing_execution"} {
			for _, outcome := range []string{"accepted", "uncertain"} {
				t.Run(protocol+"/"+path+"/"+outcome, func(t *testing.T) {
					f := newSendNowErrandFixture(t)
					installTask14CanonicalManager(t, f, protocol, path)
					f.manager.promptAgentFunc = func(_ context.Context, _, prompt string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
						f.payload <- prompt
						if outcome == "uncertain" {
							return nil, agentruntime.ErrUncertainPromptDelivery
						}
						return &executor.PromptResult{}, nil
					}
					f.prepared(t, "prepared-token", "deleted", "complete prepared result")
					f.queue(t, "ordinary input", messagequeue.QueuedByUser, nil)
					live := f.result(t, "live")
					f.realComposer(t)
					_, err := f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
					require.NoError(t, err)
					final := f.receivePrompt(t)
					waitSendNowErrandWorkers(t, f.svc)
					reopened, queue := f.reopen(t)
					require.Empty(t, queue.GetStatus(t.Context(), "session-1").Entries)
					pending, err := queue.ListPendingSendNowClaims(t.Context())
					require.NoError(t, err)
					if outcome == "uncertain" {
						require.Len(t, pending, 1)
						snapshot, _, err := errands.DecodeSendNowDelivery(pending[0].Claim.Dispatch.Metadata)
						require.NoError(t, err)
						require.Equal(t, final, snapshot.EffectivePrompt)
						require.Len(t, snapshot.Receipts, 2)
						require.Equal(t, live.ResultRevision, snapshot.Receipts[1].Claims[0].ResultRevision)
						block, err := f.svc.GetOpenSessionRecoveryBlock(t.Context(), "session-1")
						require.NoError(t, err)
						require.NotNil(t, block)
						_, err = f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
						require.Error(t, err, "uncertain source ownership must not be replayable")
						f.manager.mu.Lock()
						require.Len(t, f.manager.capturedPrompts, 1)
						f.manager.mu.Unlock()
					} else {
						require.Empty(t, pending)
					}
					row, err := reopened.GetErrand(t.Context(), live.ID)
					require.NoError(t, err)
					require.Equal(t, models.ErrandResultDelivered, row.ResultState)
					require.Equal(t, live.ResultRevision, row.ResultRevision)
				})
			}
		}
	}
}

func TestSendNowErrandMissingTranscriptWriterPreventsProviderIO(t *testing.T) {
	f := newSendNowErrandFixture(t)
	f.realComposer(t)
	f.manager.promptAgentFunc = nil
	f.svc.messageCreator = nil
	source := f.queue(t, "ordinary input with errand source metadata", messagequeue.QueuedByUser, nil)
	_, err := f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
	require.NoError(t, err)
	waitSendNowErrandWorkers(t, f.svc)
	f.manager.mu.Lock()
	prompts := append([]string(nil), f.manager.capturedPrompts...)
	f.manager.mu.Unlock()
	require.Empty(t, prompts, "a structured Send Now snapshot needs a persisted transcript before provider I/O")
	_, queue := f.reopen(t)
	status := queue.GetStatus(t.Context(), "session-1")
	require.Len(t, status.Entries, 1)
	require.Equal(t, source.ID, status.Entries[0].ID)
}
