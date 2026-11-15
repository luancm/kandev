package lifecycle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestParentTerminalIdentityResolvesSQLiteAdmissionThroughLifecycleIntake(t *testing.T) {
	fixture := newParentTerminalIdentityLifecycleFixture(t)
	callbacks := make(chan agentctl.AgentEvent, 1)
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) { callbacks <- event },
	}, nil, nil)

	if err := manager.processAgentEvent(t.Context(), fixture.execution, nil, fixture.repo, fixture.event, 0); err != nil {
		t.Fatalf("process retained terminal: %v", err)
	}
	var callback agentctl.AgentEvent
	select {
	case callback = <-callbacks:
	default:
		t.Fatal("lifecycle completion callback was not emitted")
	}
	if callback.TurnID != fixture.turnID {
		t.Fatalf("completion callback turn = %q, want exact admitted turn %q", callback.TurnID, fixture.turnID)
	}

	terminal, err := fixture.repo.GetAgentDeliveryEffect(t.Context(), fixture.terminalEffectKey())
	if err != nil {
		t.Fatalf("load projected terminal effect: %v", err)
	}
	if terminal.TurnID != fixture.turnID {
		t.Fatalf("projected terminal turn = %q, want exact admitted turn %q", terminal.TurnID, fixture.turnID)
	}
	var retainedPayload []byte
	if err := fixture.repo.DB().QueryRowContext(t.Context(),
		`SELECT payload FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?`,
		fixture.streamID, fixture.event.DeliverySequence).Scan(&retainedPayload); err != nil {
		t.Fatalf("load retained producer payload: %v", err)
	}
	if !bytes.Equal(retainedPayload, fixture.expectedReceiptPayload) {
		t.Fatalf("lifecycle attribution changed retained payload: got %s want %s", retainedPayload, fixture.expectedReceiptPayload)
	}
	var producerPayload streams.AgentEvent
	if err := json.Unmarshal(retainedPayload, &producerPayload); err != nil {
		t.Fatalf("decode immutable producer payload: %v", err)
	}
	if producerPayload.TurnID != "" {
		t.Fatalf("retained producer supplied turn %q, want it empty", producerPayload.TurnID)
	}

	resolved, errand, err := fixture.repo.ResolveAgentDeliveryEventTurn(t.Context(), fixture.retainedEvent())
	if err != nil || errand || resolved != fixture.turnID {
		t.Fatalf("SQLite admission resolution = turn %q, errand %t, error %v; want ordinary parent turn %q",
			resolved, errand, err, fixture.turnID)
	}
}

func TestParentTerminalIdentityFaultInjectedResolverLossIsObservable(t *testing.T) {
	fixture := newParentTerminalIdentityLifecycleFixture(t)
	callbacks := make(chan agentctl.AgentEvent, 1)
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) { callbacks <- event },
	}, nil, nil)

	// Hide the optional admission resolver while retaining the real SQLite
	// receive and projection methods. This local fault proves the callback
	// assertion above detects the original missing-attribution condition.
	withoutResolver := parentTerminalIdentityNoResolver{AgentDeliveryRepository: fixture.repo}
	if err := manager.processAgentEvent(t.Context(), fixture.execution, nil, withoutResolver, fixture.event, 0); err != nil {
		t.Fatalf("process terminal with injected resolver loss: %v", err)
	}
	select {
	case callback := <-callbacks:
		if callback.TurnID != "" {
			t.Fatalf("fault-injected callback turn = %q, want missing identity", callback.TurnID)
		}
	default:
		t.Fatal("fault-injected lifecycle completion callback was not emitted")
	}
}

type parentTerminalIdentityNoResolver struct {
	AgentDeliveryRepository
}

type parentTerminalIdentityLifecycleFixture struct {
	repo                   *sqliterepo.Repository
	execution              *AgentExecution
	event                  agentctl.AgentEvent
	expectedReceiptPayload []byte
	taskID                 string
	sessionID              string
	incarnationID          string
	streamID               string
	submissionID           string
	turnID                 string
}

func newParentTerminalIdentityLifecycleFixture(t *testing.T) parentTerminalIdentityLifecycleFixture {
	t.Helper()
	ctx := t.Context()
	repo := newDurableStreamSQLiteRepository(t)
	now := time.Now().UTC()
	const (
		workspaceID   = "parent-terminal-identity-workspace"
		workflowID    = "parent-terminal-identity-workflow"
		taskID        = "parent-terminal-identity-task"
		sessionID     = "parent-terminal-identity-session"
		incarnationID = "parent-terminal-identity-incarnation"
		executionID   = "parent-terminal-identity-execution"
		streamID      = "parent-terminal-identity-stream"
		submissionID  = "parent-terminal-identity-submission"
		turnID        = "parent-terminal-identity-turn"
		nativeID      = "parent-terminal-identity-native"
	)
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "identity test"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "identity test"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: workspaceID, WorkflowID: workflowID, Title: "identity test",
		State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, QueueIncarnationID: incarnationID,
		State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create parent session: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: turnID, TaskID: taskID, TaskSessionID: sessionID, StartedAt: now}); err != nil {
		t.Fatalf("create admitted prompt turn: %v", err)
	}
	if err := repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: sessionID, IncarnationID: incarnationID, Generation: 1,
		NativeSessionID: nativeID, CreationReason: "test",
	}); err != nil {
		t.Fatalf("create harness generation: %v", err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "parent-terminal-identity-attempt",
		PayloadHash: "parent-terminal-identity-payload-hash", Payload: []byte("accepted prompt"),
		State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatalf("persist prompt submission: %v", err)
	}
	binding := map[string]any{
		"task_id": taskID, "session_id": sessionID, "execution_id": executionID,
		"incarnation_id": incarnationID, "harness_generation": 1, "prompt_generation": 7,
		"stream_id": streamID, "submission_id": submissionID, "turn_id": turnID,
		"native_session_id": nativeID,
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: executionID,
		Status: "ready", Metadata: map[string]any{"delivery_prompt_turn_binding": binding},
	}); err != nil {
		t.Fatalf("persist immutable prompt admission: %v", err)
	}
	execution := &AgentExecution{
		ID: executionID, TaskID: taskID, SessionID: sessionID,
		DeliveryMode: DurableDeliveryV1, DeliveryStreamID: streamID,
		DeliveryIncarnationID: incarnationID, DeliveryHarnessGeneration: 1,
		promptTurnID: turnID, promptGeneration: 7,
	}
	event := agentctl.AgentEvent{
		Type: streams.EventTypeComplete, PromptGeneration: 7, DeliverySubmissionID: submissionID,
		DeliveryStreamID: streamID, DeliveryIncarnationID: incarnationID,
		DeliveryHarnessGeneration: 1, DeliverySequence: 1,
	}
	retained := durableAgentEvent(execution, event)
	if retained == nil || len(retained.Payload) == 0 {
		t.Fatal("durable lifecycle event did not produce a retained receipt")
	}
	return parentTerminalIdentityLifecycleFixture{
		repo: repo, execution: execution, event: event, expectedReceiptPayload: append([]byte(nil), retained.Payload...),
		taskID: taskID, sessionID: sessionID, incarnationID: incarnationID, streamID: streamID,
		submissionID: submissionID, turnID: turnID,
	}
}

func (f parentTerminalIdentityLifecycleFixture) retainedEvent() *models.AgentDeliveryEvent {
	return &models.AgentDeliveryEvent{
		TaskID: f.taskID, ExecutionID: f.execution.ID, OwnerGeneration: 1, ExpectedStreamID: f.streamID,
		SessionID: f.sessionID, IncarnationID: f.incarnationID, HarnessGeneration: 1,
		StreamID: f.streamID, Sequence: int64(f.event.DeliverySequence), SubmissionID: f.submissionID,
		EventType: f.event.Type, Payload: append([]byte(nil), f.expectedReceiptPayload...), Terminal: true,
	}
}

func (f parentTerminalIdentityLifecycleFixture) terminalEffectKey() string {
	return fmt.Sprintf("agent_delivery.terminal:%s:%d", f.streamID, f.event.DeliverySequence)
}
