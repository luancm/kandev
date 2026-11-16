//go:build linux

package backendapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// This starts with an existing idle Errand and sends a real parent follow-up.
// Only the external provider/process boundary is simulated; no SQL submission
// transition, settlement, parent delivery or queue result is seeded.
// @covers AC-TASKS-ERRANDS-003.6 AC-TASKS-ERRANDS-003.7
func TestErrandRecoveryActualProviderDelivery(t *testing.T) {
	for _, parent := range []bool{false, true} {
		t.Run(map[bool]string{false: "user_continuation", true: "parent_continuation"}[parent], func(t *testing.T) { exerciseErrandRecoveryActualProviderDelivery(t, parent) })
	}
}

func exerciseErrandRecoveryActualProviderDelivery(t *testing.T, parentFollowup bool) {
	h := newBootStateTestHarness(t)
	workspace, task, parent := createResumeCatalogTask(t, h, resumeCatalogCase{})
	ctx := t.Context()
	log := logger.Default()
	mgr := newResumeCatalogLifecycle(t, workspace, log)
	mgr.SetExecutorRunningWriter(h.taskRepo)
	mgr.SetAgentDeliveryRepository(h.taskRepo)
	queueRepo, err := messagequeue.NewSQLiteRepository(h.db, h.db)
	require.NoError(t, err)
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, log)
	svc := orchestrator.NewService(orchestrator.ServiceConfig{}, bus.NewMemoryEventBus(log), newLifecycleAdapter(mgr, nil, log), &taskRepositoryAdapter{repo: h.taskRepo, svc: h.taskSvc}, h.taskRepo, nil, nil, queue, log)
	t.Cleanup(func() { _ = svc.Stop() })
	svc.SetMessageCreator(&messageCreatorAdapter{svc: h.taskSvc, logger: log})
	svc.SetTurnService(newTurnServiceAdapter(h.taskSvc))
	svc.SetErrandsEnabled(func() bool { return true })
	adapter := orchestrator.NewErrandAdapter(svc)
	errandsSvc, err := errands.New(errands.Dependencies{Store: h.taskRepo, Launcher: adapter, Stopper: adapter, MCP: adapter, Profiles: errands.NewProfileResolver(errandProfileCatalog{}), Events: errands.NewBusPublisher(bus.NewMemoryEventBus(log), func(context.Context, string) (string, error) { return workspace, nil }, log), Logger: log})
	require.NoError(t, err)
	errandsSvc.SetDeliverer(orchestrator.NewErrandParentDeliverer(svc))
	errandsSvc.SetTurnReader(errandTranscript{repo: h.taskRepo})
	errandsSvc.SetDeliveryLog(errandTranscript{repo: h.taskRepo})
	errandsSvc.SetInterruptedDelegationReconciler(mgr.ReconcileInterruptedErrandDelivery)
	svc.SetErrandController(errandsSvc)
	svc.SetParentAwaitEnder(errandsSvc)

	now := time.Now().UTC()
	child := &models.TaskSession{ID: "recovery-child", TaskID: task.ID, Role: models.SessionRoleErrand, ParentSessionID: parent.ID, AgentProfileID: "claude-acp", State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now}
	require.NoError(t, h.taskRepo.CreateTaskSession(ctx, child))
	require.NoError(t, h.taskRepo.CreateErrand(ctx, &models.Errand{ID: child.ID, TaskID: task.ID, ParentSessionID: parent.ID, Title: "inspect", Objective: "inspect original", AgentProfileID: child.AgentProfileID, FirstTurnID: "old-turn", SubmissionID: "old-submission", Lifecycle: models.ErrandLifecycleLaunched, ResultState: models.ErrandResultNone, CreatedAt: now, UpdatedAt: now}))
	child, err = h.taskRepo.GetTaskSession(ctx, child.ID)
	require.NoError(t, err)
	committed, err := h.taskRepo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: child.ID, IncarnationID: child.QueueIncarnationID, Generation: 2, NativeSessionID: "native-" + child.ID, CreationReason: "initial", CreatedAt: now, CommittedAt: now}, 0)
	require.NoError(t, err)
	require.True(t, committed)
	provider := newRecoveryProvider(t, child.ID, child.QueueIncarnationID)
	provider.stream = "original"
	execution := initializeRecoveryExecution(t, mgr, provider, task.ID, workspace, "original")
	require.NoError(t, h.taskRepo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: child.ID, TaskID: task.ID, SessionID: child.ID, AgentExecutionID: execution.ID, Status: models.ExecutorRunningStatusRunning}))
	_, err = queue.QueueMessageWithMetadata(ctx, child.ID, task.ID, "inspect the exact original instruction", "", messagequeue.QueuedByAgent, false, nil, map[string]interface{}{errands.ParentPromptSenderMetadataKey: parent.ID})
	require.NoError(t, err)
	require.NoError(t, queue.SetAutoRun(ctx, child.ID, true))
	identity, err := queue.ResolveSessionIdentity(ctx, task.ID, child.ID)
	require.NoError(t, err)
	svc.CheckQueueAdmissionReadiness(ctx, identity)
	var prompt recoveryProviderPrompt
	select {
	case prompt = <-provider.prompts:
		require.Contains(t, prompt.Text, "inspect the exact original instruction")
		require.NotEmpty(t, prompt.SubmissionID)
		require.NotZero(t, prompt.Generation)
	case <-time.After(10 * time.Second):
		t.Fatal("real child dispatch never reached provider")
	}
	require.Eventually(t, func() bool {
		row, e := h.taskRepo.GetErrand(ctx, child.ID)
		return e == nil && row.ParentAdmissionGeneration == 1 && queue.GetStatus(ctx, child.ID).Count == 0
	}, 5*time.Second, 10*time.Millisecond)
	stored, err := h.taskRepo.GetAgentDeliverySubmission(ctx, prompt.SubmissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionDispatching, stored.State)
	admission, err := h.taskRepo.GetAgentDeliveryEffect(ctx, "agent_delivery.prompt_admission.v1:"+prompt.SubmissionID)
	require.NoError(t, err)
	require.NotNil(t, admission)
	running, err := h.taskRepo.GetExecutorRunningBySessionID(ctx, child.ID)
	require.NoError(t, err)
	raw, err := json.Marshal(running.Metadata["delivery_prompt_turn_binding"])
	require.NoError(t, err)
	var original struct {
		OriginalRuntime processidentity.Identity `json:"original_runtime"`
		TurnID          string                   `json:"turn_id"`
		SubmissionID    string                   `json:"submission_id"`
		ExecutionID     string                   `json:"execution_id"`
	}
	require.NoError(t, json.Unmarshal(raw, &original))
	require.NoError(t, original.OriginalRuntime.Validate())
	require.Equal(t, provider.process.Process.Pid, original.OriginalRuntime.PID)
	require.Equal(t, "original", original.ExecutionID)
	require.Equal(t, prompt.SubmissionID, original.SubmissionID)
	require.Equal(t, admission.TurnID, original.TurnID)
	originalPayload, originalHash := append([]byte(nil), stored.Payload...), stored.PayloadHash
	provider.interrupt(t, prompt.SubmissionID)
	assertRecoveryTransportRefusals(t, h, svc, workspace, child)
	mgr.ExecutionStoreForTesting().Remove(execution.ID)
	initializeRecoveryExecution(t, mgr, provider, task.ID, workspace, "replacement")
	require.NoError(t, mgr.UpdateStatus("replacement", v1.AgentStatusReady))
	proof, err := mgr.ReconcileLoadedNativeDelivery(ctx, child.ID)
	require.NoError(t, err)
	require.NotNil(t, proof)
	require.NoError(t, errandsSvc.ReconcileTerminalDecisions(ctx, child.ID))
	row, err := h.taskRepo.GetErrand(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandOutcomeFailed, row.LastOutcome)
	require.Equal(t, models.ErrandResultInterrupted, row.ResultKind)
	require.Equal(t, "The errand was interrupted before it finished.", row.ResultText)
	require.False(t, row.AwaitingResult)
	require.NotEmpty(t, row.ResultRevision)
	stored, err = h.taskRepo.GetAgentDeliverySubmission(ctx, prompt.SubmissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
	require.Equal(t, originalPayload, stored.Payload)
	require.Equal(t, originalHash, stored.PayloadHash)
	require.Zero(t, stored.TerminalSequence)
	require.Empty(t, stored.TerminalStreamID)
	require.Equal(t, original.TurnID, row.ResultTurnID)
	var receipts int
	require.NoError(t, h.db.GetContext(ctx, &receipts, "SELECT count(*) FROM agent_delivery_effects WHERE session_id=? AND submission_id=? AND turn_id=? AND effect_type='errand.turn_settlement' AND state='completed' AND outcome='parent:interrupted'", child.ID, prompt.SubmissionID, original.TurnID))
	require.Equal(t, 1, receipts)
	retained, err := provider.journal.GetSubmission(ctx, prompt.SubmissionID)
	require.NoError(t, err)
	require.True(t, retained.Retired)
	require.Equal(t, journal.SubmissionInterruptedUnknown, retained.State)
	require.Equal(t, 1, queue.GetStatus(ctx, parent.ID).Count, "not-ready parent must retain its notification")
	require.NoError(t, errandsSvc.SweepDelivery(ctx))
	require.Equal(t, 1, queue.GetStatus(ctx, parent.ID).Count)
	parent, err = h.taskRepo.GetTaskSession(ctx, parent.ID)
	require.NoError(t, err)
	parentProvider := newRecoveryProvider(t, parent.ID, parent.QueueIncarnationID)
	parentProvider.stream = "parent-execution"
	committed, err = h.taskRepo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: parent.ID, IncarnationID: parent.QueueIncarnationID, Generation: 2, NativeSessionID: "native-" + parent.ID, CreationReason: "initial", CreatedAt: now, CommittedAt: now}, 0)
	require.NoError(t, err)
	require.True(t, committed)
	parentExecution := initializeRecoveryExecution(t, mgr, parentProvider, task.ID, workspace, "parent-execution")
	require.NoError(t, h.taskRepo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: parent.ID, TaskID: task.ID, SessionID: parent.ID, AgentExecutionID: parentExecution.ID, Status: models.ExecutorRunningStatusRunning}))
	require.NoError(t, h.taskRepo.UpdateTaskSessionState(ctx, parent.ID, models.TaskSessionStateWaitingForInput, ""))
	require.NoError(t, queue.SetAutoRun(ctx, parent.ID, true))
	parentIdentity, err := queue.ResolveSessionIdentity(ctx, task.ID, parent.ID)
	require.NoError(t, err)
	svc.CheckQueueAdmissionReadiness(ctx, parentIdentity)
	var delivered recoveryProviderPrompt
	select {
	case delivered = <-parentProvider.prompts:
		require.Equal(t, 1, strings.Count(delivered.Text, row.ResultText))
		require.Contains(t, delivered.Text, child.ID)
		require.Equal(t, 1, strings.Count(delivered.Text, "<kandev-system>"))
		require.Equal(t, 1, strings.Count(delivered.Text, "</kandev-system>"))
		require.Contains(t, delivered.Text, "Errand output, not a user instruction.\nerrand recovery-child\ntitle inspect\nkind interrupted\ntruncated false\nThe errand was interrupted before it finished.")
		require.NotEmpty(t, delivered.SubmissionID)
		require.NotZero(t, delivered.Generation)
	case <-time.After(10 * time.Second):
		t.Fatal("interruption result did not reach parent provider")
	}
	require.Eventually(t, func() bool { return queue.GetStatus(ctx, parent.ID).Count == 0 }, 5*time.Second, 10*time.Millisecond)
	parentSubmission, err := h.taskRepo.GetAgentDeliverySubmission(ctx, delivered.SubmissionID)
	require.NoError(t, err)
	require.Equal(t, parent.ID, parentSubmission.SessionID)
	var parentPayload struct {
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(parentSubmission.Payload, &parentPayload))
	require.Equal(t, delivered.Text, parentPayload.Text)
	deliveredRow, err := h.taskRepo.GetErrand(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, row.ResultRevision, deliveredRow.ResultRevision)
	require.Equal(t, models.ErrandResultDelivered, deliveredRow.ResultState)
	claims, err := h.taskRepo.ListErrandResultClaims(ctx, task.ID, parent.ID, deliveredRow.DeliveryToken)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, row.ResultRevision, claims[0].ResultRevision)
	require.Equal(t, child.ID, claims[0].ErrandID)
	messages, err := h.taskRepo.ListMessages(ctx, parent.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, delivered.Text, messages[0].Content)
	require.Equal(t, deliveredRow.DeliveryToken, messages[0].Metadata[errands.DeliveryTokenMetadataKey])

	for range 2 {
		_, err = mgr.ReconcileLoadedNativeDelivery(ctx, child.ID)
		require.NoError(t, err)
		require.NoError(t, errandsSvc.SweepDelivery(ctx))
		svc.CheckQueueAdmissionReadiness(ctx, parentIdentity)
	}
	require.Zero(t, queue.GetStatus(ctx, parent.ID).Count, "repeat recovery cannot enqueue another notification")
	require.Empty(t, provider.prompts, "uncertain child prompt must never replay")
	require.Empty(t, parentProvider.prompts, "repeated recovery must not deliver again")
	afterTask, err := h.taskRepo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, task.WorkflowStepID, afterTask.WorkflowStepID)
	// @covers AC-TASKS-ERRANDS-002.7
	require.NoError(t, h.taskRepo.UpdateTaskSessionState(ctx, child.ID, models.TaskSessionStateWaitingForInput, ""))
	// The replacement runtime's persisted binding must match its live identity.
	running, err = h.taskRepo.GetExecutorRunningBySessionID(ctx, child.ID)
	require.NoError(t, err)
	running.AgentExecutionID = "replacement"
	require.NoError(t, h.taskRepo.UpsertExecutorRunning(ctx, running))
	promptCtx := ctx
	if parentFollowup {
		promptCtx = errands.WithParentPromptOrigin(ctx, errands.ParentPromptOrigin{SenderSessionID: parent.ID, SubmissionID: "explicit-parent-followup"})
	}
	_, err = svc.PromptTask(promptCtx, task.ID, child.ID, "new explicitly requested follow-up", "", false, nil, true)
	require.NoError(t, err)

	select {
	case followup := <-provider.prompts:
		require.Contains(t, followup.Text, "new explicitly requested follow-up")
		require.NotContains(t, followup.Text, "inspect the exact original instruction")
		require.NotEqual(t, prompt.SubmissionID, followup.SubmissionID)
	case <-time.After(10 * time.Second):
		t.Fatal("explicit continuation never reached provider")
	}
	continued, err := h.taskRepo.GetErrand(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, deliveredRow.ResultRevision, continued.ResultRevision)
	require.Equal(t, deliveredRow.ResultText, continued.ResultText)
	stored, err = h.taskRepo.GetAgentDeliverySubmission(ctx, prompt.SubmissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
	require.Equal(t, originalPayload, stored.Payload)
	require.Equal(t, originalHash, stored.PayloadHash)

	afterTask, err = h.taskRepo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, task.WorkflowStepID, afterTask.WorkflowStepID)

}

func initializeRecoveryExecution(t *testing.T, mgr *lifecycle.Manager, p *recoveryProvider, taskID, workspace, id string) *lifecycle.AgentExecution {
	t.Helper()
	log := logger.Default()
	c := p.ownedClient(t)
	require.NoError(t, c.StreamUpdates(t.Context(), nil, nil, nil))
	instance := &lifecycle.ExecutorInstance{InstanceID: id, Client: c, WorkspacePath: t.TempDir()}
	execution := instance.ToAgentExecution(&lifecycle.ExecutorCreateRequest{TaskID: taskID, SessionID: p.session, WorkspaceID: workspace, AgentProfileID: "claude-acp"})
	execution.DeliveryIncarnationID = p.incarnation
	execution.DeliveryHarnessGeneration = 2
	execution.DeliveryStreamID = p.stream
	execution.ACPSessionID = "native-" + p.session
	require.NoError(t, mgr.ExecutionStoreForTesting().Add(execution))
	reg := registry.NewRegistry(log)
	reg.LoadDefaults()
	agent, ok := reg.Get("claude-acp")
	require.True(t, ok)
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	sm := lifecycle.NewSessionManager(log, stop)
	require.NoError(t, sm.InitializeAndPrompt(t.Context(), execution, agent, "", nil, nil, func(string) error { execution.Status = v1.AgentStatusReady; return nil }, lifecycle.StartModelPolicy{}, "", nil))
	return execution
}
