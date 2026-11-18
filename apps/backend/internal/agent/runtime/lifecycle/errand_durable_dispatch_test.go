package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/agent"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestErrandInitialPromptPersistsOwnerBeforeDispatch(t *testing.T) {
	ctx := context.Background()
	repo, db := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "task"}))
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{ID: "child", TaskID: "task", ParentSessionID: "parent", Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone}))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "child", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "child", 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, repo.CreateErrandTurn(ctx, "child", &models.Turn{ID: "turn", TaskID: "task"}))
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.prompt" {
			response, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{
				"success": true, "submission_id": "prompt:first",
			})
			return response
		}
		return mock.defaultHandler(msg)
	}
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	require.NoError(t, client.StreamUpdates(ctx, nil, nil, nil))
	waitForWSConnected(t, mock)
	stop := newTestStopCh(t)
	sm := NewSessionManager(newSessionTestLogger(), stop)
	stream := NewStreamManager(newSessionTestLogger(), StreamCallbacks{}, nil, stop)
	cleanupStreamManager(t, stop, stream)
	stream.setAgentDeliveryRepository(repo)
	sm.SetDependencies(nil, stream, nil, nil)
	execution := &AgentExecution{ID: "execution", Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "child", Generation: 1}, Status: v1.AgentStatusReady, DeliveryMode: DurableDeliveryV1, DeliveryStreamID: "stream", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 3, agentctl: client, promptDoneCh: make(chan PromptCompletionSignal, 1), promptTurnID: "turn"}
	execution.setMetadataValue(initialDeliverySubmissionIDMetadataKey, "first")
	require.NoError(t, sm.bindErrandDelivery(ctx, execution))
	execution.ACPSessionID = "native"
	completer, ok := any(sm).(interface {
		completeErrandNativeSession(context.Context, *AgentExecution) error
	})
	require.True(t, ok, "native identity must be durably completed")
	require.NoError(t, completer.completeErrandNativeSession(ctx, execution))
	reached := false
	sm.beforePromptDispatchHook = func() {
		reached = true
		stored, e := repo.GetAgentDeliverySubmission(ctx, "prompt:first")
		require.NoError(t, e)
		require.Equal(t, "errand:child", stored.SessionID)
		require.Equal(t, models.DeliverySubmissionDispatching, stored.State)
	}
	_, err = sm.sendPrompt(ctx, execution, "hello", false, nil, true, sendPromptCallbacks{deliverySubmissionID: "first"}, false)
	require.NoError(t, err)
	require.True(t, reached)
	started := agentctl.AgentEvent{
		Type: streams.EventTypeTurnStarted, SessionID: execution.ACPSessionID,
		DeliveryStreamID: "stream", DeliveryIncarnationID: "incarnation",
		DeliveryHarnessGeneration: 3, DeliverySequence: 1,
		DeliverySubmissionID: "prompt:first", TurnID: "turn", PromptGeneration: 1,
	}
	foreign := started
	foreign.TurnID = "other-turn"
	require.Error(t, stream.processAgentEvent(ctx, execution, client, repo, foreign, 0))
	require.NoError(t, stream.processAgentEvent(ctx, execution, client, repo, started, 0))
	cursor, err := repo.GetAgentDeliveryCursor(ctx, "stream")
	require.NoError(t, err)
	require.EqualValues(t, 1, cursor.ProjectedSequence)
	var count int
	require.NoError(t, db.Get(&count, `SELECT COUNT(*) FROM task_sessions`))
	require.Zero(t, count)
}

func TestErrandPromptRejectsForeignAcknowledgement(t *testing.T) {
	ctx := t.Context()
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.prompt" {
			response, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{
				"success": true, "submission_id": "prompt:sibling",
			})
			return response
		}
		return mock.defaultHandler(msg)
	}
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	require.NoError(t, client.StreamUpdates(ctx, nil, nil, nil))
	waitForWSConnected(t, mock)
	sm := NewSessionManager(newSessionTestLogger(), nil)
	execution := &AgentExecution{Owner: ExecutionOwner{Kind: ExecutionOwnerErrand}, agentctl: client}
	err := sm.callAgentctlPrompt(ctx, execution, "hello", nil, 1, false, "first")
	require.ErrorIs(t, err, ErrUncertainPromptDelivery)
	require.Empty(t, execution.deliverySubmissionIDSnapshot())
}

func TestErrandDeliveryAcknowledgementRequiresExactTransportOwner(t *testing.T) {
	execution := &AgentExecution{
		Owner:            ExecutionOwner{Kind: ExecutionOwnerErrand, ErrandID: "child", Generation: 1},
		DeliveryStreamID: "stream", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 3,
	}
	retained := journal.Stream{
		StreamID: "stream", SessionID: "errand:child", IncarnationID: "incarnation",
		HarnessGeneration: 3, HighWater: 4,
	}
	status := &agentctl.DeliveryStatus{
		StorageCapability: journal.StorageCapability{Durable: true, Version: journal.CurrentProtocolVersion},
		SessionID:         retained.SessionID, StreamID: retained.StreamID, IncarnationID: retained.IncarnationID,
		HarnessGeneration: retained.HarnessGeneration, Stream: &retained,
	}
	require.True(t, deliveryAckDescriptorMatches(status, execution, retained.StreamID))
	require.NoError(t, validateRecoveredReplayStream(retained, execution, retained.StreamID, 4))
	require.Error(t, validateRecoveredReplayStream(retained, execution, retained.StreamID, 5))
	status.SessionID = "errand:sibling"
	require.False(t, deliveryAckDescriptorMatches(status, execution, retained.StreamID))
	retained.SessionID = ""
	require.Error(t, validateRecoveredReplayStream(retained, execution, retained.StreamID, 4))
}

func TestErrandDurableEventUsesOwnerTransportIdentity(t *testing.T) {
	execution := &AgentExecution{ID: "execution", Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "child", Generation: 1}, DeliveryMode: DurableDeliveryV1, DeliveryStreamID: "stream", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 3}
	event := durableAgentEvent(execution, agentctl.AgentEvent{Type: streams.EventTypeMessageChunk, SessionID: "native", DeliveryStreamID: "stream", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 3, DeliverySequence: 1, DeliverySubmissionID: "prompt:first"})
	require.Equal(t, "errand:child", event.SessionID)
	require.Equal(t, "task", event.TaskID)
	require.Equal(t, "execution", event.ExecutionID)
	require.Equal(t, "stream", event.ExpectedStreamID)
}

func TestErrandInitialSubmissionUsesDurableHandoff(t *testing.T) {
	execution := &AgentExecution{ID: "execution", Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, ErrandID: "child", Generation: 1}, DeliveryMode: DurableDeliveryV1, DeliveryHarnessGeneration: 3}
	execution.setMetadataValue(initialDeliverySubmissionIDMetadataKey, "first")
	require.Equal(t, "first", initialPromptDeliverySubmissionID(execution))
}

func TestErrandLaunchStartBindsBeforeAgentProcess(t *testing.T) {
	ctx := context.Background()
	repo, db := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-start", Title: "task"}))
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{ID: "child-start", TaskID: "task-start", ParentSessionID: "parent", Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone}))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "child-start", 0)
	require.NoError(t, err)
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(repo)
	mgr.streamManager.setAgentDeliveryRepository(repo)
	backend := &createInstanceExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
	mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
	mgr.executorRegistry.Register(backend)
	req := &LaunchRequest{WorkspaceID: "ws", AgentProfileID: "profile-1", ExecutorType: "local_pc", WorkspacePath: t.TempDir(), Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, WorkspaceID: "ws", TaskID: "task-start", ErrandID: "child-start", Generation: 1}, OwnerAdmission: allowRunOwner{}, DeliveryStreamID: "stream-start", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1}
	execution, err := mgr.Launch(ctx, req)
	require.NoError(t, err)
	require.Error(t, mgr.StartAgentProcess(ctx, execution.ID))
	var count int
	require.NoError(t, db.Get(&count, `SELECT COUNT(*) FROM errand_harness_bindings`))
	require.Zero(t, count)
	bound, err := repo.BindErrandExecution(ctx, "child-start", 1, execution.ID)
	require.NoError(t, err)
	require.True(t, bound)
	require.Error(t, mgr.StartAgentProcess(ctx, execution.ID))
	require.NoError(t, db.Get(&count, `SELECT COUNT(*) FROM errand_harness_bindings`))
	require.Equal(t, 1, count)
	require.Empty(t, execution.SessionID)
}

func TestErrandDurableEventUsesExactRetainedBinding(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-binding", Title: "task"}))
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{ID: "child-binding", TaskID: "task-binding", ParentSessionID: "parent", Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone}))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "child-binding", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "child-binding", 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	binding := &models.ErrandHarnessBinding{ErrandID: "child-binding", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 3, StreamID: "stream-binding"}
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	execution := &AgentExecution{ID: "execution", Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task-binding", ErrandID: "child-binding", Generation: 1}, DeliveryMode: DurableDeliveryV1, DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 3, DeliveryStreamID: "stream-binding"}
	event := &models.AgentDeliveryEvent{SessionID: "errand:child-binding", TaskID: "task-binding", ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 3, OwnerGeneration: 3, StreamID: "stream-binding", ExpectedStreamID: "stream-binding"}
	stale, err := deliveryEventIsStale(ctx, execution, event, repo, nil)
	require.NoError(t, err)
	require.False(t, stale)
	_, err = repo.ReserveErrandExecutionGeneration(ctx, "child-binding", 1)
	require.NoError(t, err)
	stale, err = deliveryEventIsStale(ctx, execution, event, repo, nil)
	require.NoError(t, err)
	require.True(t, stale)
	event.StreamID = "foreign"
	event.ExpectedStreamID = "foreign"
	_, err = deliveryEventIsStale(ctx, execution, event, repo, nil)
	require.Error(t, err)
}

func TestErrandNativeInitializationCompletesBindingWithoutPrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	repo, db := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-native", Title: "task"}))
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{ID: "child-native", TaskID: "task-native", ParentSessionID: "parent", Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone}))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "child-native", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "child-native", 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	server := newStartupStreamServer(t)
	server.earlyEvent.Type = streams.EventTypeSessionMode
	server.release()
	stream := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	stream.setAgentDeliveryRepository(repo)
	t.Cleanup(stream.Wait)
	sm := NewSessionManager(newSessionTestLogger(), nil)
	sm.SetDependencies(nil, stream, nil, nil)
	execution := &AgentExecution{ID: "execution", Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task-native", ErrandID: "child-native", Generation: 1}, WorkspacePath: t.TempDir(), DeliveryStreamID: startupStreamID, DeliveryIncarnationID: startupStreamIncarnation, DeliveryHarnessGeneration: 1, agentctl: server.client}
	require.NoError(t, sm.bindErrandDelivery(ctx, execution))
	config := &testAgent{id: "test-agent", enabled: true, runtimeConfig: &agents.RuntimeConfig{Cmd: agents.NewCommand("test-agent"), Protocol: agent.ProtocolACP, SessionConfig: agents.SessionConfig{}}}
	require.NoError(t, sm.InitializeAndPromptWithLayers(ctx, execution, config, "", nil, nil, func(string) error { return nil }, "", "", nil, "", "", nil, StartModelPolicy{}))
	var native string
	require.NoError(t, db.GetContext(ctx, &native, db.Rebind(`SELECT native_session_id FROM errand_harness_bindings WHERE errand_id=?`), "child-native"))
	require.Equal(t, startupStreamNativeID, native)
	require.NotContains(t, server.actionLog(), "agent.prompt")
}

func TestErrandLaunchStartDispatchesDurably(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	repo, db := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-vertical", Title: "task"}))
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{ID: "child-vertical", TaskID: "task-vertical", ParentSessionID: "parent", Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone}))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "child-vertical", 0)
	require.NoError(t, err)
	require.NoError(t, repo.CreateErrandTurn(ctx, "child-vertical", &models.Turn{ID: "turn-vertical", TaskID: "task-vertical"}))
	mock := newMockAgentServer(t)
	mock.deliveryStatus = &agentctl.DeliveryStatus{
		StorageCapability: journal.StorageCapability{Durable: true, Version: journal.CurrentProtocolVersion},
		SessionID:         "errand:child-vertical", IncarnationID: "incarnation",
		HarnessGeneration: 1, StreamID: "stream-vertical",
	}
	t.Cleanup(mock.Close)
	dispatch := make(chan error, 1)
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.initialize" {
			response, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{"success": true, "agent_info": map[string]string{"name": "test-agent", "version": "1"}, "durable_delivery": map[string]any{"version": journal.CurrentProtocolVersion, "durable": true}})
			return response
		}
		if msg.Action == "agent.prompt" {
			stored, e := repo.GetAgentDeliverySubmission(ctx, "prompt:first-vertical")
			if e == nil && (stored.SessionID != "errand:child-vertical" || stored.State != models.DeliverySubmissionDispatching) {
				e = fmt.Errorf("wrong admitted submission")
			}
			dispatch <- e
			response, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{
				"success": true, "submission_id": "prompt:first-vertical",
			})
			return response
		}
		return mock.defaultHandler(msg)
	}
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(repo)
	mgr.streamManager.setAgentDeliveryRepository(repo)
	backend := &createInstanceExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}, client: client}
	mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
	mgr.executorRegistry.Register(backend)
	req := &LaunchRequest{WorkspaceID: "ws", AgentProfileID: "profile-1", ExecutorType: "local_pc", WorkspacePath: t.TempDir(), Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, WorkspaceID: "ws", TaskID: "task-vertical", ErrandID: "child-vertical", Generation: 1}, OwnerAdmission: allowRunOwner{}, DeliveryStreamID: "stream-vertical", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1, TurnID: "turn-vertical", InitialDeliverySubmissionID: "first-vertical", TaskDescription: "hello"}
	inherited := mcpprofile.New(mcpprofile.SurfaceConfiguration, nil, nil)
	req.McpProfile = &inherited
	req.McpMode = "config"
	execution, err := mgr.Launch(ctx, req)
	require.NoError(t, err)
	require.Equal(t, mcpprofile.SurfaceConfiguration, inherited.Surface)
	require.Equal(t, "config", req.McpMode)
	require.Equal(t, mcpprofile.SurfaceErrand, backend.lastRequest.McpProfile.Surface)
	require.Equal(t, "errand", backend.lastRequest.McpMode)
	require.Equal(t, errandMCPOwner(req.Owner), backend.lastRequest.ErrandMCPOwner)
	require.Empty(t, execution.TaskID)
	require.Empty(t, execution.SessionID)
	require.Equal(t, req.Owner, execution.Owner)
	bound, err := repo.BindErrandExecution(ctx, "child-vertical", 1, execution.ID)
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, mgr.StartAgentProcess(ctx, execution.ID))
	require.Equal(t, DurableDeliveryV1, execution.DeliveryMode)
	require.True(t, execution.matchesErrandPermissionTarget(ErrandPermissionTarget{Owner: req.Owner, ExecutionID: execution.ID, IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream-vertical"}))
	select {
	case err = <-dispatch:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("prompt not dispatched")
	}
	accepted, err := repo.GetAgentDeliverySubmissionMetadata(ctx, "prompt:first-vertical")
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionAccepted, accepted.State)
	require.Equal(t, execution.ID, accepted.DispatchAttemptID)
	message := agentctl.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "answer", SessionID: execution.ACPSessionID, DeliveryStreamID: "stream-vertical", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1, DeliverySequence: 1, DeliverySubmissionID: "prompt:first-vertical", PromptGeneration: 1, TurnID: "turn-vertical"}
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, message, 0))
	tool := message
	tool.Type = streams.EventTypeToolCall
	tool.Text = ""
	tool.DeliverySequence = 2
	tool.ToolCallID = "tool-read"
	tool.ProtocolMessageID = "source-first"
	tool.ToolTitle = "Read file"
	tool.ToolStatus = "pending"
	tool.NormalizedPayload = streams.NewReadFile("/repo/a.go", 0, 0)
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, tool, 0))
	update := tool
	update.Type = streams.EventTypeToolUpdate
	update.DeliverySequence = 3
	update.ToolStatus = "completed"
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, update, 0))
	terminal := message
	terminal.Type = streams.EventTypeComplete
	terminal.Text = ""
	terminal.DeliverySequence = 4
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, terminal, 0))
	require.NoError(t, mgr.sessionManager.recordErrandPromptAccepted(ctx, execution, accepted))
	messages, _, err := repo.ListErrandMessages(ctx, "child-vertical", "", 10)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, "answer", messages[0].Content)
	require.Equal(t, models.MessageTypeToolRead, messages[1].Type)
	require.Equal(t, "completed", messages[1].Metadata["status"])
	submission, err := repo.GetAgentDeliverySubmission(ctx, "prompt:first-vertical")
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionCompleted, submission.State)
	turn, err := repo.GetErrandTurn(ctx, "child-vertical", "turn-vertical")
	require.NoError(t, err)
	require.NotNil(t, turn.CompletedAt, "durable terminal evidence must close the owned transcript turn")
	secondPayload := []byte(`{"text":"second"}`)
	second := &models.AgentDeliverySubmission{ID: "prompt:second-vertical", Payload: secondPayload, PayloadHash: journal.SubmissionHash(secondPayload), State: models.DeliverySubmissionDispatching, DispatchAttemptID: execution.ID}
	admitted, err := repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{ErrandID: "child-vertical", Generation: 1, ExecutionID: execution.ID, IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream-vertical", NativeSessionID: execution.ACPSessionID, PromptGeneration: 2, TurnID: "turn-vertical", Submission: second})
	require.NoError(t, err)
	require.True(t, admitted)
	execution.promptLifecycleMu.Lock()
	execution.promptGeneration = 2
	execution.promptTurnID = "turn-vertical"
	execution.promptLifecycleMu.Unlock()
	execution.setDeliverySubmissionID(second.ID)
	callbacks := 0
	var notification agentctl.AgentEvent
	mgr.streamManager.callbacks.OnAgentEventWithGeneration = func(_ *AgentExecution, event agentctl.AgentEvent, _ uint64) { callbacks++; notification = event }
	delayed := tool
	delayed.ToolCallID = "old-delayed"
	delayed.ProtocolMessageID = "source-delayed"
	delayed.DeliverySequence = 5
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, delayed, 0))
	require.Zero(t, callbacks, "historical tool start must not update successor activity")
	next := tool
	next.ToolCallID = "tool-next"
	next.ProtocolMessageID = "source-next"
	next.DeliverySequence = 6
	next.DeliverySubmissionID = second.ID
	next.PromptGeneration = 2
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, next, 0))
	require.Equal(t, 1, callbacks)
	late := update
	late.DeliverySequence = 7
	late.DeliverySubmissionID = ""
	late.PromptGeneration = 0
	late.TurnID = ""
	late.ToolTitle = "late first"
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, late, 0))
	require.Equal(t, 1, callbacks, "historical tool must not update successor activity")
	current := next
	current.Type = streams.EventTypeToolUpdate
	current.DeliverySequence = 8
	current.ToolStatus = "completed"
	current.DeliverySubmissionID = ""
	current.PromptGeneration = 0
	current.TurnID = ""
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, current, 0))
	require.Equal(t, 2, callbacks, "current tool must still notify")
	require.Equal(t, second.ID, notification.DeliverySubmissionID)
	require.EqualValues(t, 2, notification.PromptGeneration)
	require.Equal(t, "turn-vertical", notification.TurnID)
	var raw []byte
	require.NoError(t, db.GetContext(ctx, &raw, db.Rebind(`SELECT payload FROM agent_delivery_inbox WHERE stream_id=? AND sequence=?`), "stream-vertical", 8))
	var retained streams.AgentEvent
	require.NoError(t, json.Unmarshal(raw, &retained))
	require.Empty(t, retained.TurnID)
	require.Zero(t, retained.PromptGeneration)
	require.Empty(t, retained.DeliverySubmissionID)
	var active string
	require.NoError(t, db.GetContext(ctx, &active, db.Rebind(`SELECT active_submission_id FROM errand_execution_generations WHERE errand_id=? AND generation=1`), "child-vertical"))
	require.Equal(t, second.ID, active)
	messages, _, err = repo.ListErrandMessages(ctx, "child-vertical", "", 10)
	require.NoError(t, err)
	require.Equal(t, "late first", messages[1].Content)
	historicalShell := tool
	historicalShell.DeliverySequence = 9
	historicalShell.ToolCallID = "shell-old"
	historicalShell.ProtocolMessageID = "shell-old-source"
	historicalShell.ToolTitle = "Earlier command"
	historicalShell.NormalizedPayload = streams.NewShellExec("echo old", "/tmp", "", 0, false)
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, historicalShell, 0))
	oldPatch, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: "old shell output"}, "old-shell-epoch", true)
	require.NoError(t, err)
	historicalShell.Type = streams.EventTypeShellOutputUpdate
	historicalShell.DeliverySequence = 10
	historicalShell.DeliverySubmissionID = ""
	historicalShell.PromptGeneration = 0
	historicalShell.TurnID = ""
	historicalShell.ShellOutput = oldPatch
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, historicalShell, 0))
	require.Equal(t, 2, callbacks, "historical shell output must not update successor activity")
	shell := next
	shell.DeliverySequence = 11
	shell.ToolCallID = "shell-next"
	shell.ProtocolMessageID = "shell-source"
	shell.ToolTitle = "Run command"
	shell.NormalizedPayload = streams.NewShellExec("echo hello", "/tmp", "", 0, false)
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, shell, 0))
	patch, err := streams.EncodeShellOutputPatch(nil, streams.ShellExecOutput{Stdout: "hello from shell"}, "shell-epoch", true)
	require.NoError(t, err)
	shell.Type = streams.EventTypeShellOutputUpdate
	shell.DeliverySequence = 12
	shell.DeliverySubmissionID = ""
	shell.PromptGeneration = 0
	shell.TurnID = ""
	shell.ShellOutput = patch
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, shell, 0))
	require.Equal(t, 4, callbacks)
	require.Equal(t, second.ID, notification.DeliverySubmissionID)
	require.EqualValues(t, 2, notification.PromptGeneration)
	messages, _, err = repo.ListErrandMessages(ctx, "child-vertical", "", 10)
	require.NoError(t, err)
	var shellMessage *models.Message
	var oldShellMessage *models.Message
	for _, message := range messages {
		if message.Type == models.MessageTypeToolExecute && message.Metadata["tool_call_id"] == "shell-next" {
			shellMessage = message
		}
		if message.Type == models.MessageTypeToolExecute && message.Metadata["tool_call_id"] == "shell-old" {
			oldShellMessage = message
		}
	}
	require.NotNil(t, shellMessage)
	require.NotNil(t, oldShellMessage)
	oldDetail, err := repo.GetErrandMessageDetail(ctx, "child-vertical", oldShellMessage.ID)
	require.NoError(t, err)
	oldOutput, ok := models.ExtractShellExecOutput(oldDetail.Metadata)
	require.True(t, ok)
	require.Equal(t, "old shell output", oldOutput.Stdout)
	detail, err := repo.GetErrandMessageDetail(ctx, "child-vertical", shellMessage.ID)
	require.NoError(t, err)
	output, ok := models.ExtractShellExecOutput(detail.Metadata)
	require.True(t, ok)
	require.Equal(t, "hello from shell", output.Stdout)
	callbacks = 0
	mgr.streamManager.callbacks.OnAgentEventWithGeneration = func(_ *AgentExecution, event agentctl.AgentEvent, _ uint64) {
		callbacks++
		notification = event
		stored, _, readErr := repo.ListErrandMessages(ctx, "child-vertical", "", 20)
		require.NoError(t, readErr)
		found := false
		for _, message := range stored {
			if message.Type == models.MessageTypePermissionRequest && message.Metadata["request_id"] == event.RequestID {
				found = true
			}
		}
		require.True(t, found, "permission must be committed before callback")
	}
	permission := next
	permission.Type = streams.EventTypePermissionRequest
	permission.DeliverySequence = 13
	permission.PromptGeneration, permission.TurnID = 0, ""
	permission.ToolCallID, permission.NormalizedPayload = "", nil
	permission.RequestID, permission.PendingID = "current-request", "current-pending"
	permission.PermissionHumanInput = false
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, permission, 0))
	require.Equal(t, 1, callbacks)
	require.Equal(t, second.ID, notification.DeliverySubmissionID)
	require.EqualValues(t, 2, notification.PromptGeneration)
	require.Equal(t, "turn-vertical", notification.TurnID)
	historicalPermission := permission
	historicalPermission.DeliverySequence = 14
	historicalPermission.DeliverySubmissionID = "prompt:first-vertical"
	historicalPermission.RequestID, historicalPermission.PendingID = "old-request", "old-pending"
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, historicalPermission, 0))
	require.Equal(t, 1, callbacks, "historical permission must not notify successor")
	pending, err := repo.LatestErrandPendingPermissions(ctx, []string{"child-vertical"})
	require.NoError(t, err)
	require.Equal(t, "current-request", pending["child-vertical"].Metadata["request_id"])
	var count int
	require.NoError(t, db.GetContext(ctx, &count, `SELECT COUNT(*) FROM task_sessions`))
	require.Zero(t, count)
}

func TestErrandToolNotificationCarriesCapturedOrigin(t *testing.T) {
	mgr, bus := createTestManagerWithTracking()
	event := agentctl.AgentEvent{Type: streams.EventTypeToolUpdate, DeliverySubmissionID: "prompt:origin", PromptGeneration: 3, TurnID: "turn-origin"}
	execution := &AgentExecution{ID: "execution", Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, ErrandID: "child", TaskID: "task", Generation: 2}}
	mgr.eventPublisher.PublishAgentStreamEvent(execution, event)
	records := bus.getStreamEvents()
	require.Len(t, records, 1)
	raw, err := json.Marshal(records[0])
	require.NoError(t, err)
	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	require.Equal(t, "task", envelope["task_id"])
	require.Equal(t, "prompt:origin", envelope["delivery_submission_id"])
	require.EqualValues(t, 3, envelope["prompt_generation"])
	require.Equal(t, "turn-origin", envelope["turn_id"])
	mgr2, bus2 := createTestManagerWithTracking()
	ordinary := &AgentExecution{ID: "ordinary", SessionID: "session"}
	mgr2.eventPublisher.PublishAgentStreamEvent(ordinary, event)
	records = bus2.getStreamEvents()
	require.Len(t, records, 1)
	raw, err = json.Marshal(records[0])
	require.NoError(t, err)
	envelope = map[string]interface{}{}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	require.NotContains(t, envelope, "delivery_submission_id")
	require.NotContains(t, envelope, "prompt_generation")
	require.NotContains(t, envelope, "turn_id")
}

func TestErrandToolCallbackDoesNotMutateSuccessorAfterOriginCheck(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{ID: "execution", Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "child", Generation: 1}, promptGeneration: 1, promptTurnID: "turn"}
	execution.setDeliverySubmissionID("first")
	old := agentctl.AgentEvent{Type: streams.EventTypeToolCall, ToolCallID: "old", ToolTitle: "old", DeliverySubmissionID: "first", PromptGeneration: 1, TurnID: "turn"}
	require.True(t, errandToolOriginCurrent(execution, &models.ErrandToolOrigin{SubmissionID: "first", PromptGeneration: 1, TurnID: "turn"}))
	execution.promptLifecycleMu.Lock()
	execution.promptGeneration = 2
	execution.setDeliverySubmissionID("second")
	execution.setActiveTool(activeTopLevelTool{ToolCallID: "successor"})
	execution.promptLifecycleMu.Unlock()
	execution.messageMu.Lock()
	execution.messageBuffer.WriteString("successor buffer")
	execution.messageMu.Unlock()
	mgr.handleAgentEventAtContextResetBoundaryInternal(execution, old, false, "", false)
	require.Equal(t, "successor", execution.activeToolSnapshot().ToolCallID)
	execution.messageMu.Lock()
	buffer := execution.messageBuffer.String()
	execution.messageMu.Unlock()
	require.Equal(t, "successor buffer", buffer)
	current := old
	current.ToolCallID = "current"
	current.DeliverySubmissionID = "second"
	current.PromptGeneration = 2
	mgr.handleAgentEventAtContextResetBoundaryInternal(execution, current, false, "", false)
	require.Equal(t, "current", execution.activeToolSnapshot().ToolCallID)
	old.Type = streams.EventTypeToolUpdate
	old.ToolCallID = "current"
	old.ToolStatus = "completed"
	mgr.handleAgentEventAtContextResetBoundaryInternal(execution, old, false, "", false)
	require.Equal(t, "current", execution.activeToolSnapshot().ToolCallID)
	current.Type = streams.EventTypeToolUpdate
	current.ToolStatus = "completed"
	mgr.handleAgentEventAtContextResetBoundaryInternal(execution, current, false, "", false)
	require.Nil(t, execution.activeToolSnapshot())
	current.Type = streams.EventTypeToolCall
	current.ToolCallID = "shell"
	current.ToolStatus = "pending"
	mgr.handleAgentEventAtContextResetBoundaryInternal(execution, current, false, "", false)
	require.Equal(t, "shell", execution.activeToolSnapshot().ToolCallID)
	old.Type = streams.EventTypeShellOutputUpdate
	old.ToolCallID = "shell"
	old.ToolStatus = "completed"
	mgr.handleAgentEventAtContextResetBoundaryInternal(execution, old, false, "", false)
	require.Equal(t, "shell", execution.activeToolSnapshot().ToolCallID)
	current.Type = streams.EventTypeShellOutputUpdate
	current.ToolStatus = "completed"
	mgr.handleAgentEventAtContextResetBoundaryInternal(execution, current, false, "", false)
	require.Nil(t, execution.activeToolSnapshot())
}
