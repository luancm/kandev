package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"expvar"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/pkg/api/v1"
)

func TestParentTerminalIdentityReleasesQueuedUserMessage(t *testing.T) {
	exerciseParentTerminalIdentityQueueRelease(t, parentTerminalIdentityFresh)
}

func TestParentTerminalIdentityRepairsIncompleteEffectBeforeQueuedRelease(t *testing.T) {
	exerciseParentTerminalIdentityQueueRelease(t, parentTerminalIdentityIncomplete)
}

func TestParentTerminalIdentityKeepsQueuedMessageWhenAdmissionIsMissing(t *testing.T) {
	exerciseParentTerminalIdentityQueueRelease(t, parentTerminalIdentityMissingAdmission)
}

func TestParentTerminalIdentityDoesNotOverwriteConflictingEffect(t *testing.T) {
	exerciseParentTerminalIdentityQueueRelease(t, parentTerminalIdentityConflictingEffect)
}

func TestParentTerminalIdentityRepairRollsBackOnSQLFailure(t *testing.T) {
	exerciseParentTerminalIdentityQueueRelease(t, parentTerminalIdentitySQLFailure)
}

type parentTerminalIdentityScenario int

const (
	parentTerminalIdentityFresh parentTerminalIdentityScenario = iota
	parentTerminalIdentityIncomplete
	parentTerminalIdentityMissingAdmission
	parentTerminalIdentityConflictingEffect
	parentTerminalIdentitySQLFailure
)

type parentTerminalIdentityObservation struct {
	scenario        parentTerminalIdentityScenario
	repo            *tasksqlite.Repository
	service         *Service
	dispatched      <-chan string
	queued          *messagequeue.QueuedMessage
	status          *messagequeue.QueueStatus
	terminal        *models.AgentDeliveryEffect
	sessionID       string
	turnID          string
	successorTurnID string
	resolvedTurn    string
	resolvedErrand  bool
	conflictingErr  error
	drained         bool
}

func exerciseParentTerminalIdentityQueueRelease(t *testing.T, scenario parentTerminalIdentityScenario) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	const (
		taskID                = "parent-terminal-identity-task"
		sessionID             = "parent-terminal-identity-session"
		executionID           = "parent-terminal-identity-execution"
		turnID                = "parent-terminal-identity-original-turn"
		successorTurnID       = "parent-terminal-identity-successor-turn"
		streamID              = "parent-terminal-identity-stream"
		submissionID          = "parent-terminal-identity-submission"
		successorSubmissionID = "parent-terminal-identity-successor-submission"
	)
	seedSession(t, repo, taskID, sessionID, "")
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load parent session: %v", err)
	}
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: turnID, TaskSessionID: sessionID, TaskID: taskID, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create admitted turn: %v", err)
	}
	if err := repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: sessionID, IncarnationID: incarnationID, Generation: 1,
		NativeSessionID: "parent-terminal-identity-native", CreationReason: "test",
	}); err != nil {
		t.Fatalf("create harness generation: %v", err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "parent-terminal-identity-attempt",
		PayloadHash: "parent-terminal-identity-hash", Payload: []byte("original accepted prompt"),
		State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatalf("prepare admitted prompt: %v", err)
	}
	firstBinding := map[string]any{
		"task_id": taskID, "session_id": sessionID, "execution_id": executionID,
		"incarnation_id": incarnationID, "harness_generation": 1, "prompt_generation": 7,
		"stream_id": streamID, "submission_id": submissionID, "turn_id": turnID,
		"native_session_id": "parent-terminal-identity-native",
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
		Status: "ready", Metadata: map[string]any{"delivery_prompt_turn_binding": firstBinding},
	}); err != nil {
		t.Fatalf("persist exact prompt admission: %v", err)
	}
	if err := repo.CompleteTurn(ctx, turnID); err != nil {
		t.Fatalf("complete original turn before successor admission: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: successorTurnID, TaskSessionID: sessionID, TaskID: taskID, StartedAt: time.Now().UTC().Add(time.Second),
	}); err != nil {
		t.Fatalf("create successor turn: %v", err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: successorSubmissionID, SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "parent-terminal-identity-successor-attempt",
		PayloadHash: "parent-terminal-identity-successor-hash", Payload: []byte("successor prompt"),
		State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatalf("prepare successor prompt: %v", err)
	}
	secondBinding := map[string]any{
		"task_id": taskID, "session_id": sessionID, "execution_id": executionID,
		"incarnation_id": incarnationID, "harness_generation": 1, "prompt_generation": 8,
		"stream_id": streamID, "submission_id": successorSubmissionID, "turn_id": successorTurnID,
		"native_session_id": "parent-terminal-identity-native",
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
		Status: "ready", Metadata: map[string]any{"delivery_prompt_turn_binding": secondBinding},
	}); err != nil {
		t.Fatalf("persist successor prompt admission: %v", err)
	}

	transcriptEvent := parentTerminalIdentityEvent(t, sessionID, taskID, executionID, incarnationID,
		streamID, submissionID, 1, streams.AgentEvent{
			Type: streams.EventTypeMessageChunk, Text: "prior assistant output", ProtocolMessageID: "prior-output",
			PromptGeneration: 7, TurnID: turnID,
		})
	projectParentTerminalIdentityEvent(t, repo, ctx, transcriptEvent)
	terminalEvent := parentTerminalIdentityEvent(t, sessionID, taskID, executionID, incarnationID,
		streamID, submissionID, 2, streams.AgentEvent{Type: streams.EventTypeComplete, PromptGeneration: 7})
	resolvedTurn, resolvedErrand, resolveErr := repo.ResolveAgentDeliveryEventTurn(ctx, &terminalEvent)
	if resolveErr != nil && scenario != parentTerminalIdentityMissingAdmission {
		t.Fatalf("resolve ordinary parent terminal from exact admission: %v", resolveErr)
	}
	var conflictingPayload streams.AgentEvent
	if err := json.Unmarshal(terminalEvent.Payload, &conflictingPayload); err != nil {
		t.Fatalf("decode terminal for conflicting identity control: %v", err)
	}
	conflictingPayload.TurnID = successorTurnID
	conflictingBytes, err := json.Marshal(conflictingPayload)
	if err != nil {
		t.Fatalf("marshal conflicting terminal identity: %v", err)
	}
	conflictingEvent := terminalEvent
	conflictingEvent.Payload = conflictingBytes
	_, _, conflictingErr := repo.ResolveAgentDeliveryEventTurn(ctx, &conflictingEvent)
	nativeAsyncPayload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "native async output"})
	if err != nil {
		t.Fatalf("marshal native asynchronous control: %v", err)
	}
	nativeAsyncEvent := terminalEvent
	nativeAsyncEvent.SubmissionID = ""
	nativeAsyncEvent.EventType = streams.EventTypeMessageChunk
	nativeAsyncEvent.Terminal = false
	nativeAsyncEvent.Payload = nativeAsyncPayload
	nativeTurn, nativeErrand, nativeErr := repo.ResolveAgentDeliveryEventTurn(ctx, &nativeAsyncEvent)
	if nativeErr != nil || nativeTurn != "" || nativeErrand {
		t.Fatalf("ordinary native asynchronous attribution = turn %q, errand %t, error %v; want no admission owner",
			nativeTurn, nativeErrand, nativeErr)
	}
	rawTerminalPayload := append([]byte(nil), terminalEvent.Payload...)
	projectParentTerminalIdentityEvent(t, repo, ctx, terminalEvent)
	if scenario != parentTerminalIdentityFresh {
		if _, err := repo.DB().ExecContext(ctx,
			`UPDATE agent_delivery_effects SET turn_id = '' WHERE effect_key = ?`,
			"agent_delivery.terminal:"+streamID+":2"); err != nil {
			t.Fatalf("simulate previously persisted incomplete effect: %v", err)
		}
	}
	if scenario == parentTerminalIdentityMissingAdmission {
		if _, err := repo.DB().ExecContext(ctx,
			`DELETE FROM agent_delivery_effects WHERE effect_type = ? AND submission_id = ?`,
			"agent_delivery.prompt_admission.v1", submissionID); err != nil {
			t.Fatalf("remove exact admission evidence: %v", err)
		}
	}
	if scenario == parentTerminalIdentityConflictingEffect {
		if _, err := repo.DB().ExecContext(ctx,
			`UPDATE agent_delivery_effects SET turn_id = ? WHERE effect_key = ?`,
			successorTurnID, "agent_delivery.terminal:"+streamID+":2"); err != nil {
			t.Fatalf("install conflicting stored identity: %v", err)
		}
	}
	if scenario == parentTerminalIdentitySQLFailure {
		if _, err := repo.DB().ExecContext(ctx, `
			CREATE TRIGGER fail_parent_terminal_identity_repair
			BEFORE UPDATE OF turn_id ON agent_delivery_effects
			WHEN OLD.effect_key = 'agent_delivery.terminal:parent-terminal-identity-stream:2'
			BEGIN SELECT RAISE(ABORT, 'injected identity repair failure'); END`); err != nil {
			t.Fatalf("install identity repair failure trigger: %v", err)
		}
	}

	terminal, err := repo.GetAgentDeliveryEffect(ctx, "agent_delivery.terminal:"+streamID+":2")
	if err != nil {
		t.Fatalf("load projected parent terminal: %v", err)
	}
	if scenario == parentTerminalIdentitySQLFailure {
		settled, err := repo.SettleAgentDeliveryTerminal(ctx, streamID, 2, models.DeliverySubmissionCompleted, time.Now().UTC())
		if err == nil {
			t.Fatalf("terminal settlement = %t, nil; want injected identity-repair SQL error", settled)
		}
		storedTerminal, readErr := repo.GetAgentDeliveryEffect(ctx, terminal.EffectKey)
		if readErr != nil {
			t.Fatalf("load terminal after failed identity repair: %v", readErr)
		}
		submission, readErr := repo.GetAgentDeliverySubmission(ctx, submissionID)
		if readErr != nil {
			t.Fatalf("load submission after failed identity repair: %v", readErr)
		}
		if settled || storedTerminal.TurnID != "" || submission.State != models.DeliverySubmissionDispatching {
			t.Fatalf("failed identity repair escaped transaction: settled=%t terminal_turn=%q submission_state=%q",
				settled, storedTerminal.TurnID, submission.State)
		}
		return
	}
	var retainedPayload []byte
	if err := repo.DB().QueryRowContext(ctx,
		`SELECT payload FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = 2`, streamID).Scan(&retainedPayload); err != nil {
		t.Fatalf("load retained terminal payload: %v", err)
	}
	messages, err := repo.ListMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("load transcript before successor release: %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "prior assistant output" || messages[0].TurnID != turnID {
		t.Fatalf("transcript before successor release = %+v", messages)
	}

	service := createEngineServiceWithTaskRepo(t, repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	dispatched := make(chan string, 2)
	manager := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	manager.promptAgentFunc = func(_ context.Context, _ string, message string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
		dispatched <- message
		return &executor.PromptResult{}, nil
	}
	service.agentManager = manager
	service.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	queued, err := service.messageQueue.QueueMessage(ctx, sessionID, taskID, "queued user command", "", messagequeue.QueuedByUser, false, nil)
	if err != nil {
		t.Fatalf("queue ordinary user message: %v", err)
	}
	completion := workflowEffectForEvent(watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, TurnID: terminal.TurnID,
		DeliveryStreamID: streamID, DeliveryIncarnationID: incarnationID,
		DeliveryHarnessGeneration: 1, DeliverySequence: 2, DeliverySubmissionID: submissionID,
	})
	drained := service.drainQueuedBeforeWorkflowTransition(
		withWorkflowEffect(ctx, completion), taskID, sessionID, session,
	)
	status := service.messageQueue.GetStatus(ctx, sessionID)
	if scenario == parentTerminalIdentityFresh || scenario == parentTerminalIdentityIncomplete {
		assertParentTerminalIdentityReleased(t, ctx, parentTerminalIdentityObservation{
			scenario: scenario, repo: repo, service: service, dispatched: dispatched,
			queued: queued, status: status, terminal: terminal, sessionID: sessionID,
			turnID: turnID, successorTurnID: successorTurnID, resolvedTurn: resolvedTurn,
			resolvedErrand: resolvedErrand, conflictingErr: conflictingErr, drained: drained,
		})
	} else {
		assertParentTerminalIdentityProtected(t, ctx, parentTerminalIdentityObservation{
			scenario: scenario, repo: repo, dispatched: dispatched, queued: queued,
			status: status, terminal: terminal, turnID: turnID, successorTurnID: successorTurnID,
			conflictingErr: conflictingErr, drained: drained,
		})
	}
	if string(retainedPayload) != string(rawTerminalPayload) {
		t.Fatalf("terminal payload changed during projection: got %s want %s", retainedPayload, rawTerminalPayload)
	}
	messages, err = repo.ListMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("load transcript after successor release: %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "prior assistant output" || messages[0].TurnID != turnID {
		t.Fatalf("transcript after successor release = %+v", messages)
	}
}

func assertParentTerminalIdentityReleased(t *testing.T, ctx context.Context, observation parentTerminalIdentityObservation) {
	t.Helper()
	var delivered string
	select {
	case delivered = <-observation.dispatched:
	case <-time.After(5 * time.Second):
		delivered = "<not dispatched>"
	}
	workflowKey := "workflow.on_turn_complete:" + observation.turnID
	workflowOutcome, err := observation.repo.GetAgentDeliveryEffect(ctx, workflowKey)
	if err != nil {
		t.Fatalf("load guarded workflow no-op: %v", err)
	}
	settledTerminal, err := observation.repo.GetAgentDeliveryEffect(ctx, observation.terminal.EffectKey)
	if err != nil {
		t.Fatalf("reload terminal after queue settlement: %v", err)
	}
	var successorCompletedAt sql.NullTime
	if err := observation.repo.DB().QueryRowContext(ctx,
		`SELECT completed_at FROM task_session_turns WHERE id = ?`, observation.successorTurnID).Scan(&successorCompletedAt); err != nil {
		t.Fatalf("load successor completion state: %v", err)
	}
	if observation.resolvedTurn != observation.turnID || observation.resolvedErrand || observation.conflictingErr == nil ||
		observation.terminal.TurnID != observation.turnID && observation.scenario == parentTerminalIdentityFresh ||
		!observation.drained || delivered != observation.queued.Content || len(observation.dispatched) != 0 || observation.status.Count != 0 ||
		workflowOutcome.TurnID != observation.turnID || workflowOutcome.Outcome != "not_required" || workflowOutcome.State != models.DeliveryEffectCompleted ||
		settledTerminal.TurnID != observation.turnID || successorCompletedAt.Valid {
		t.Fatalf("parent terminal handoff: resolved=%q errand=%t terminal=%q want=%q scenario=%d drained=%t delivered=%q duplicate=%t queue=%+v workflow=%+v successor_completed=%v",
			observation.resolvedTurn, observation.resolvedErrand, settledTerminal.TurnID, observation.turnID, observation.scenario,
			observation.drained, delivered, len(observation.dispatched) != 0, observation.status.Entries, workflowOutcome, successorCompletedAt)
	}
	if observation.scenario == parentTerminalIdentityIncomplete {
		assertParentTerminalIdentityRepeatStable(t, ctx, observation, settledTerminal)
	}
}

func assertParentTerminalIdentityRepeatStable(
	t *testing.T,
	ctx context.Context,
	observation parentTerminalIdentityObservation,
	settledTerminal *models.AgentDeliveryEffect,
) {
	t.Helper()
	repeated, err := observation.repo.RepairAgentDeliveryTerminalIdentity(ctx, settledTerminal)
	if err != nil || repeated == nil || repeated.TurnID != observation.turnID {
		t.Fatalf("repeat terminal identity reconciliation = %+v, %v", repeated, err)
	}
	if err := observation.service.reconcileAgentDeliverySettlements(ctx, observation.sessionID); err != nil {
		t.Fatalf("repeat terminal settlement reconciliation: %v", err)
	}
	select {
	case unexpected := <-observation.dispatched:
		t.Fatalf("repeat reconciliation dispatched another provider prompt: %q", unexpected)
	default:
	}
}

func assertParentTerminalIdentityProtected(t *testing.T, ctx context.Context, observation parentTerminalIdentityObservation) {
	t.Helper()
	select {
	case unexpected := <-observation.dispatched:
		t.Fatalf("queued message reached provider without safe terminal identity: %q", unexpected)
	default:
	}
	storedTerminal, err := observation.repo.GetAgentDeliveryEffect(ctx, observation.terminal.EffectKey)
	if err != nil {
		t.Fatalf("reload protected terminal: %v", err)
	}
	wantStoredTurn := ""
	if observation.scenario == parentTerminalIdentityConflictingEffect {
		wantStoredTurn = observation.successorTurnID
	}
	if observation.conflictingErr == nil || observation.drained || observation.status.Count != 1 ||
		len(observation.status.Entries) != 1 || observation.status.Entries[0].ID != observation.queued.ID ||
		storedTerminal.TurnID != wantStoredTurn {
		t.Fatalf("unsafe parent terminal repair: scenario=%d drained=%t queue=%+v stored_turn=%q want=%q",
			observation.scenario, observation.drained, observation.status.Entries, storedTerminal.TurnID, wantStoredTurn)
	}
	if _, err := observation.repo.GetAgentDeliveryEffect(ctx, "workflow.on_turn_complete:"+observation.turnID); err == nil {
		t.Fatal("workflow no-op was recorded without safe terminal identity")
	}
}

func parentTerminalIdentityEvent(
	t *testing.T,
	sessionID, taskID, executionID, incarnationID, streamID, submissionID string,
	sequence int64,
	agentEvent streams.AgentEvent,
) models.AgentDeliveryEvent {
	t.Helper()
	agentEvent.DeliverySubmissionID = ""
	payload, err := json.Marshal(agentEvent)
	if err != nil {
		t.Fatalf("marshal durable producer event: %v", err)
	}
	return models.AgentDeliveryEvent{
		TaskID: taskID, ExecutionID: executionID, OwnerGeneration: 1, ExpectedStreamID: streamID,
		SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 1,
		StreamID: streamID, Sequence: sequence, SubmissionID: submissionID,
		EventType: agentEvent.Type, Payload: payload, Terminal: agentEvent.Type == streams.EventTypeComplete,
	}
}

func projectParentTerminalIdentityEvent(t *testing.T, repo *tasksqlite.Repository, ctx context.Context, event models.AgentDeliveryEvent) {
	t.Helper()
	if _, err := repo.ReceiveAgentDeliveryEvent(ctx, &event, event.Sequence); err != nil {
		t.Fatalf("receive durable producer event sequence %d: %v", event.Sequence, err)
	}
	if event.EventType == streams.EventTypeMessageChunk || event.EventType == streams.EventTypeReasoning {
		if _, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, &event, nil); err != nil {
			t.Fatalf("project canonical durable producer event sequence %d: %v", event.Sequence, err)
		}
		return
	}
	if _, err := repo.ProjectAgentDeliveryEvent(ctx, &event, nil); err != nil {
		t.Fatalf("project durable producer event sequence %d: %v", event.Sequence, err)
	}
}

func TestParentTerminalIdentityAutoRunsEmptyErrandNotificationAfterLifecycleRepair(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		repairable bool
	}{
		{name: "fresh terminal"},
		{name: "incomplete terminal reconciled", repairable: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newParentTerminalIdentityNotificationFixture(t)
			beforeDelivery := parentTerminalIdentityNotificationMetric(t)
			admitted := make(chan []messagequeue.QueuedMessage, 1)
			workerDone := make(chan struct{}, 2)
			fixture.svc.onQueuedMessageExecutionComplete = func() { workerDone <- struct{}{} }
			fixture.manager.promptAgentFunc = func(_ context.Context, _, prompt string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
				fixture.payload <- prompt
				return &executor.PromptResult{}, nil
			}
			fixture.errands.SetDeliverer(&parentTerminalIdentityObservedDeliverer{
				ParentDeliverer: NewErrandParentDeliverer(fixture.svc),
				afterAdmission: func(ctx context.Context, sessionID string) {
					status := fixture.svc.messageQueue.GetStatus(ctx, sessionID)
					admitted <- append([]messagequeue.QueuedMessage(nil), status.Entries...)
				},
			})
			fixture.svc.SetErrandController(fixture.errands)

			terminal := fixture.terminal(t)
			if scenario.repairable {
				if _, err := fixture.repo.DB().ExecContext(t.Context(),
					`UPDATE agent_delivery_effects SET turn_id = '', state = ?, completed_at = NULL WHERE effect_key = ?`,
					models.DeliveryEffectPending, terminal.EffectKey); err != nil {
					t.Fatalf("make terminal effect incomplete: %v", err)
				}
				if err := fixture.svc.reconcileAgentDeliverySettlements(t.Context(), fixture.sessionID); err != nil {
					t.Fatalf("repair incomplete terminal through normal reconciliation: %v", err)
				}
			}
			terminal = fixture.terminal(t)
			if terminal.TurnID != fixture.turnID {
				t.Fatalf("terminal identity before completion callback = %q, want %q", terminal.TurnID, fixture.turnID)
			}
			fixture.svc.handleAgentCompleted(t.Context(), watcher.AgentEventData{
				TaskID: fixture.taskID, SessionID: fixture.sessionID, AgentExecutionID: fixture.executionID,
				TurnID: terminal.TurnID, PromptGeneration: 7,
				DeliveryStreamID: terminal.StreamID, DeliveryIncarnationID: terminal.IncarnationID,
				DeliveryHarnessGeneration: uint64(terminal.HarnessGeneration),
				DeliverySequence:          uint64(terminal.Sequence), DeliverySubmissionID: terminal.SubmissionID,
			})

			var admittedEntries []messagequeue.QueuedMessage
			select {
			case admittedEntries = <-admitted:
			case <-time.After(5 * time.Second):
				t.Fatal("normal parent settlement did not admit an Errand notification")
			}
			if len(admittedEntries) != 1 || admittedEntries[0].Content != "" ||
				admittedEntries[0].QueuedBy != messagequeue.QueuedByServer ||
				!isErrandNotificationEntry(&admittedEntries[0]) {
				t.Fatalf("normal notification admission = %+v; want one empty server notification", admittedEntries)
			}
			var prompt string
			select {
			case prompt = <-fixture.payload:
			case <-time.After(5 * time.Second):
				t.Fatalf("Auto-run did not reach the provider; queue=%+v", fixture.svc.messageQueue.GetStatus(t.Context(), fixture.sessionID).Entries)
			}
			select {
			case <-workerDone:
			case <-time.After(5 * time.Second):
				t.Fatal("Auto-run notification worker did not complete")
			}

			if got := strings.Count(prompt, fixture.result.ResultText); got != 1 {
				t.Fatalf("provider prompt contains Errand result %d times, want once: %q", got, prompt)
			}
			fixture.manager.mu.Lock()
			promptCalls := append([]string(nil), fixture.manager.capturedPrompts...)
			fixture.manager.mu.Unlock()
			if len(promptCalls) != 1 || promptCalls[0] != prompt {
				t.Fatalf("provider prompts = %q, want exactly the one observed composed prompt", promptCalls)
			}
			if status := fixture.svc.messageQueue.GetStatus(t.Context(), fixture.sessionID); status.Count != 0 || len(status.Entries) != 0 {
				t.Fatalf("notification queue after provider dispatch = %+v", status.Entries)
			}
			workflowOutcome, err := fixture.repo.GetAgentDeliveryEffect(t.Context(), "workflow.on_turn_complete:"+fixture.turnID)
			if err != nil {
				t.Fatalf("load original-turn workflow outcome: %v", err)
			}
			if workflowOutcome.TurnID != fixture.turnID || workflowOutcome.Outcome != "not_required" ||
				workflowOutcome.State != models.DeliveryEffectCompleted {
				t.Fatalf("original-turn workflow outcome = %+v", workflowOutcome)
			}
			storedTerminal := fixture.terminal(t)
			if storedTerminal.TurnID != fixture.turnID {
				t.Fatalf("stored terminal turn after Auto-run = %q, want %q", storedTerminal.TurnID, fixture.turnID)
			}
			originalTurn, err := fixture.repo.GetTurn(t.Context(), fixture.turnID)
			if err != nil || originalTurn.CompletedAt == nil {
				t.Fatalf("original turn completion = %+v, %v; want completed", originalTurn, err)
			}
			successorTurn, err := fixture.repo.GetTurn(t.Context(), fixture.successorTurnID)
			if err != nil || successorTurn.CompletedAt == nil || !successorTurn.CompletedAt.Equal(fixture.successorCompletedAt) {
				t.Fatalf("pre-existing successor state changed: turn=%+v error=%v want_completed_at=%s",
					successorTurn, err, fixture.successorCompletedAt)
			}
			messages, err := fixture.repo.ListMessages(t.Context(), fixture.sessionID)
			if err != nil {
				t.Fatalf("list parent transcript after Auto-run: %v", err)
			}
			if !containsParentTerminalIdentityMessage(messages, fixture.transcriptText, fixture.turnID) {
				t.Fatalf("prior transcript output was not preserved: %+v", messages)
			}
			storedResult, err := fixture.repo.GetErrand(t.Context(), fixture.result.ID)
			if err != nil || storedResult.ResultState != models.ErrandResultDelivered || storedResult.DeliveredRoute != models.ErrandDeliveryNotification {
				t.Fatalf("Errand result delivery state = %+v, %v; want delivered by notification", storedResult, err)
			}
			if got := parentTerminalIdentityNotificationMetric(t); got != beforeDelivery+1 {
				t.Fatalf("notification delivery metric = %d, want exactly one increment from %d", got, beforeDelivery)
			}
		})
	}
}

func TestParentTerminalIdentityKeepsErrandNotificationHeldWithoutSafeEvidence(t *testing.T) {
	for _, scenario := range []struct {
		name             string
		removeAdmission  bool
		conflictWithTurn bool
	}{
		{name: "missing admission", removeAdmission: true},
		{name: "conflicting terminal", conflictWithTurn: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newParentTerminalIdentityNotificationFixture(t)
			fixture.manager.promptAgentFunc = func(_ context.Context, _, prompt string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
				fixture.payload <- prompt
				return &executor.PromptResult{}, nil
			}
			terminal := fixture.terminal(t)
			if scenario.removeAdmission {
				if _, err := fixture.repo.DB().ExecContext(t.Context(), `DELETE FROM agent_delivery_effects WHERE effect_key = ?`,
					"agent_delivery.prompt_admission.v1:"+fixture.submissionID); err != nil {
					t.Fatalf("remove prompt admission evidence: %v", err)
				}
				if _, err := fixture.repo.DB().ExecContext(t.Context(),
					`UPDATE agent_delivery_effects SET turn_id = '' WHERE effect_key = ?`, terminal.EffectKey); err != nil {
					t.Fatalf("remove terminal turn evidence: %v", err)
				}
			} else {
				if _, err := fixture.repo.DB().ExecContext(t.Context(),
					`UPDATE agent_delivery_effects SET turn_id = ? WHERE effect_key = ?`, fixture.successorTurnID, terminal.EffectKey); err != nil {
					t.Fatalf("install conflicting terminal identity: %v", err)
				}
			}
			terminal = fixture.terminal(t)
			if err := fixture.repo.CompleteTurn(t.Context(), fixture.turnID); err != nil {
				t.Fatalf("complete parent turn before notification admission control: %v", err)
			}
			if err := fixture.svc.messageQueue.SetAutoRun(t.Context(), fixture.sessionID, false); err != nil {
				t.Fatalf("pause queue while admitting control notification: %v", err)
			}
			fixture.errands.SetDeliverer(NewErrandParentDeliverer(fixture.svc))
			fixture.svc.SetErrandController(fixture.errands)
			if _, err := fixture.errands.OnParentTurnSettled(t.Context(), fixture.sessionID, errands.SettleCauseTurnDone); err != nil {
				t.Fatalf("admit pending-result notification through normal settlement: %v", err)
			}
			status := fixture.svc.messageQueue.GetStatus(t.Context(), fixture.sessionID)
			if status.Count != 1 || len(status.Entries) != 1 || !isErrandNotificationEntry(&status.Entries[0]) || status.Entries[0].Content != "" {
				t.Fatalf("control notification admission = %+v; want one empty notification", status.Entries)
			}
			if err := fixture.svc.messageQueue.SetAutoRun(t.Context(), fixture.sessionID, true); err != nil {
				t.Fatalf("enable Auto-run for held-notification control: %v", err)
			}
			completion := workflowEffectForEvent(watcher.AgentEventData{
				TaskID: fixture.taskID, SessionID: fixture.sessionID, TurnID: terminal.TurnID,
				DeliveryStreamID: terminal.StreamID, DeliveryIncarnationID: terminal.IncarnationID,
				DeliveryHarnessGeneration: uint64(terminal.HarnessGeneration),
				DeliverySequence:          uint64(terminal.Sequence), DeliverySubmissionID: terminal.SubmissionID,
			})
			session, err := fixture.repo.GetTaskSession(t.Context(), fixture.sessionID)
			if err != nil {
				t.Fatalf("load parent session for held-notification control: %v", err)
			}
			drained := fixture.svc.drainQueuedBeforeWorkflowTransition(withWorkflowEffect(t.Context(), completion),
				fixture.taskID, fixture.sessionID, session)
			status = fixture.svc.messageQueue.GetStatus(t.Context(), fixture.sessionID)
			if drained || status.Count != 1 || len(status.Entries) != 1 || status.Entries[0].Content != "" {
				t.Fatalf("unsafe terminal released Errand notification: drained=%t queue=%+v", drained, status.Entries)
			}
			select {
			case prompt := <-fixture.payload:
				t.Fatalf("provider received a notification without safe terminal evidence: %q", prompt)
			default:
			}
			if _, err := fixture.repo.GetAgentDeliveryEffect(t.Context(), "workflow.on_turn_complete:"+terminal.TurnID); err == nil {
				t.Fatal("workflow no-op was committed without safe terminal evidence")
			}
			storedResult, err := fixture.repo.GetErrand(t.Context(), fixture.result.ID)
			if err != nil || storedResult.ResultState != models.ErrandResultPending {
				t.Fatalf("held notification changed result state: %+v, %v", storedResult, err)
			}
		})
	}
}

type parentTerminalIdentityNotificationFixture struct {
	*sendNowErrandFixture
	errands              *errands.Service
	result               *models.Errand
	taskID               string
	sessionID            string
	executionID          string
	incarnationID        string
	streamID             string
	submissionID         string
	turnID               string
	successorTurnID      string
	successorCompletedAt time.Time
	transcriptText       string
}

func newParentTerminalIdentityNotificationFixture(t *testing.T) *parentTerminalIdentityNotificationFixture {
	t.Helper()
	f := newSendNowErrandFixture(t)
	ctx := t.Context()
	const (
		taskID           = "task-1"
		sessionID        = "session-1"
		executionID      = "exec-1"
		turnID           = "parent-terminal-identity-notification-turn"
		successorTurnID  = "parent-terminal-identity-notification-successor"
		streamID         = "parent-terminal-identity-notification-stream"
		submissionID     = "parent-terminal-identity-notification-submission"
		transcriptOutput = "preserved transcript before Errand notification"
	)
	task, err := f.repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("load parent task: %v", err)
	}
	task.WorkflowStepID = ""
	task.WIPAdmitted = false
	if err := f.repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("seed parent without workflow step: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	session, err := f.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load parent session: %v", err)
	}
	session.State = models.TaskSessionStateRunning
	if err := f.repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("mark parent running before terminal: %v", err)
	}
	if err := f.repo.CreateTurn(ctx, &models.Turn{ID: turnID, TaskSessionID: sessionID, TaskID: taskID, StartedAt: now}); err != nil {
		t.Fatalf("create original parent turn: %v", err)
	}
	successorCompletedAt := now.Add(time.Second)
	if err := f.repo.CreateTurn(ctx, &models.Turn{
		ID: successorTurnID, TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: now.Add(-time.Minute), CompletedAt: &successorCompletedAt,
	}); err != nil {
		t.Fatalf("create existing completed successor state: %v", err)
	}
	if err := f.repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: sessionID, IncarnationID: session.QueueIncarnationID, Generation: 1,
		NativeSessionID: "parent-terminal-identity-notification-native", CreationReason: "test",
	}); err != nil {
		t.Fatalf("create harness generation: %v", err)
	}
	if _, err := f.repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: sessionID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "parent-terminal-identity-notification-attempt",
		PayloadHash: "parent-terminal-identity-notification-hash", Payload: []byte("original parent prompt"),
		State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatalf("prepare exact parent prompt submission: %v", err)
	}
	binding := map[string]any{
		"task_id": taskID, "session_id": sessionID, "execution_id": executionID,
		"incarnation_id": session.QueueIncarnationID, "harness_generation": 1, "prompt_generation": 7,
		"stream_id": streamID, "submission_id": submissionID, "turn_id": turnID,
		"native_session_id": "parent-terminal-identity-notification-native",
	}
	if err := f.repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
		Status: "ready", Metadata: map[string]any{"delivery_prompt_turn_binding": binding},
	}); err != nil {
		t.Fatalf("persist exact prompt admission: %v", err)
	}
	transcriptEvent := parentTerminalIdentityEvent(t, sessionID, taskID, executionID, session.QueueIncarnationID,
		streamID, submissionID, 1, streams.AgentEvent{
			Type: streams.EventTypeMessageChunk, Text: transcriptOutput, ProtocolMessageID: "prior-output",
			PromptGeneration: 7, TurnID: turnID,
		})
	projectParentTerminalIdentityEvent(t, f.repo, ctx, transcriptEvent)
	terminalEvent := parentTerminalIdentityEvent(t, sessionID, taskID, executionID, session.QueueIncarnationID,
		streamID, submissionID, 2, streams.AgentEvent{Type: streams.EventTypeComplete, PromptGeneration: 7})
	projectParentTerminalIdentityEvent(t, f.repo, ctx, terminalEvent)
	manager := f.manager
	manager.currentPromptExecutionID = executionID
	manager.currentPromptGeneration.Store(7)
	f.svc.scheduler = scheduler.NewScheduler(queue.NewTaskQueue(100), f.svc.executor, newMockTaskRepo(), testLogger(), scheduler.SchedulerConfig{})
	f.svc.messageCreator = &repositoryBackedMessageCreator{mockMessageCreator: &mockMessageCreator{}, repo: f.repo}
	realErrands, err := errands.New(errands.Dependencies{
		Store: f.repo, Launcher: NewErrandAdapter(f.svc), Stopper: NewErrandAdapter(f.svc), MCP: NewErrandAdapter(f.svc),
		Profiles: errandProfileStub{}, Events: &errandEventRecorder{}, Logger: testLogger(),
	})
	if err != nil {
		t.Fatalf("create SQLite-backed Errands service: %v", err)
	}
	result := f.result(t, "parent-terminal-identity-result")
	if err := f.svc.messageQueue.SetAutoRun(ctx, sessionID, true); err != nil {
		t.Fatalf("enable parent Auto-run: %v", err)
	}
	return &parentTerminalIdentityNotificationFixture{
		sendNowErrandFixture: f, errands: realErrands, result: result,
		taskID: taskID, sessionID: sessionID, executionID: executionID,
		incarnationID: session.QueueIncarnationID, streamID: streamID, submissionID: submissionID,
		turnID: turnID, successorTurnID: successorTurnID, successorCompletedAt: successorCompletedAt,
		transcriptText: transcriptOutput,
	}
}

func (f *parentTerminalIdentityNotificationFixture) terminal(t *testing.T) *models.AgentDeliveryEffect {
	t.Helper()
	terminal, err := f.repo.GetAgentDeliveryEffect(t.Context(), "agent_delivery.terminal:"+f.streamID+":2")
	if err != nil {
		t.Fatalf("load parent terminal effect: %v", err)
	}
	return terminal
}

type parentTerminalIdentityObservedDeliverer struct {
	ParentDeliverer errands.ParentDeliverer
	afterAdmission  func(context.Context, string)
}

func (d *parentTerminalIdentityObservedDeliverer) NotificationBlocked(ctx context.Context, sessionID string) bool {
	return d.ParentDeliverer.NotificationBlocked(ctx, sessionID)
}

func (d *parentTerminalIdentityObservedDeliverer) DeliverPending(ctx context.Context, sessionID string) {
	d.ParentDeliverer.DeliverPending(ctx, sessionID)
	d.afterAdmission(ctx, sessionID)
}

func (d *parentTerminalIdentityObservedDeliverer) AdmitPrepared(ctx context.Context, sessionID, content string, metadata map[string]interface{}) error {
	return d.ParentDeliverer.AdmitPrepared(ctx, sessionID, content, metadata)
}

func parentTerminalIdentityNotificationMetric(t *testing.T) int64 {
	t.Helper()
	metric, ok := expvar.Get("errands_result_delivery_total").(*expvar.Map)
	if !ok {
		t.Fatal("Errand result delivery expvar is unavailable")
	}
	value := metric.Get("route=notification")
	if value == nil {
		return 0
	}
	count, err := strconv.ParseInt(value.String(), 10, 64)
	if err != nil {
		t.Fatalf("parse notification delivery metric %q: %v", value.String(), err)
	}
	return count
}

func containsParentTerminalIdentityMessage(messages []*models.Message, content, turnID string) bool {
	for _, message := range messages {
		if message.Content == content && message.TurnID == turnID {
			return true
		}
	}
	return false
}
