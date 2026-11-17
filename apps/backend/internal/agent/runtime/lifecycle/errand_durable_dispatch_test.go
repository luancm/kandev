package lifecycle

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
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
	var count int
	require.NoError(t, db.Get(&count, `SELECT COUNT(*) FROM task_sessions`))
	require.Zero(t, count)
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
	execution, err := mgr.Launch(ctx, req)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "child-vertical", 1, execution.ID)
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, mgr.StartAgentProcess(ctx, execution.ID))
	select {
	case err = <-dispatch:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("prompt not dispatched")
	}
	message := agentctl.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "answer", SessionID: execution.ACPSessionID, DeliveryStreamID: "stream-vertical", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1, DeliverySequence: 1, DeliverySubmissionID: "prompt:first-vertical", PromptGeneration: 1, TurnID: "turn-vertical"}
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, message, 0))
	terminal := message
	terminal.Type = streams.EventTypeComplete
	terminal.Text = ""
	terminal.DeliverySequence = 2
	require.NoError(t, mgr.streamManager.processAgentEvent(ctx, execution, client, repo, terminal, 0))
	messages, _, err := repo.ListErrandMessages(ctx, "child-vertical", "", 10)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "answer", messages[0].Content)
	submission, err := repo.GetAgentDeliverySubmission(ctx, "prompt:first-vertical")
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionCompleted, submission.State)
	var count int
	require.NoError(t, db.GetContext(ctx, &count, `SELECT COUNT(*) FROM task_sessions`))
	require.Zero(t, count)
}
