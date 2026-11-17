package lifecycle

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestErrandPermissionLiveSnapshotFencesOwnerAndClient(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-live", Title: "task"}))
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{ID: "child-live", TaskID: "task-live", ParentSessionID: "parent", Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone}))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "child-live", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "child-live", 1, "execution-live")
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &models.ErrandHarnessBinding{ErrandID: "child-live", Generation: 1, ExecutionID: "execution-live", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream"}, 0))
	require.NoError(t, repo.CreateErrandTurn(ctx, "child-live", &models.Turn{ID: "turn", TaskID: "task-live"}))
	sub := &models.AgentDeliverySubmission{ID: "submission", Payload: []byte("prompt"), PayloadHash: journal.SubmissionHash([]byte("prompt"))}
	_, err = repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{ErrandID: "child-live", Generation: 1, ExecutionID: "execution-live", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", PromptGeneration: 1, TurnID: "turn", Submission: sub})
	require.NoError(t, err)
	var resolveCalls atomic.Int32
	var listCalls atomic.Int32
	var cancelCalls atomic.Int32
	var holdResolve atomic.Bool
	resolveEntered, releaseResolve := make(chan struct{}, 1), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseResolve) })
	mock := newMockAgentServer(t)
	mock.handler = func(msg ws.Message) *ws.Message {
		switch msg.Action {
		case "agent.permissions.list":
			listCalls.Add(1)
			response, _ := ws.NewResponse(msg.ID, msg.Action, streams.PermissionListResponse{Permissions: []streams.PendingAgentPermission{{RequestID: "request", PendingID: "pending", Options: []streams.PermissionChoice{{OptionID: "allow", Kind: streams.PermissionOptionKindAllowOnce}}}, {RequestID: "cancel-request", PendingID: "cancel-pending"}, {RequestID: "old-request", PendingID: "old-pending", Options: []streams.PermissionChoice{{OptionID: "allow"}}}, {RequestID: "new-request", PendingID: "new-pending", Options: []streams.PermissionChoice{{OptionID: "allow"}}}}})
			return response
		case "agent.permissions.resolve":
			resolveCalls.Add(1)
			if holdResolve.Load() {
				resolveEntered <- struct{}{}
				<-releaseResolve
			}
			response, _ := ws.NewResponse(msg.ID, msg.Action, streams.PermissionResolveResponse{RequestID: "request", PendingID: "pending", OptionID: "allow", Status: "resolved"})
			return response
		case "agent.permissions.cancel":
			cancelCalls.Add(1)
			response, _ := ws.NewResponse(msg.ID, msg.Action, streams.PermissionCancelResponse{RequestID: "cancel-request", PendingID: "cancel-pending", Status: "cancelled"})
			return response
		default:
			return nil
		}
	}
	t.Cleanup(mock.Close)
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	require.NoError(t, client.StreamUpdates(ctx, func(_ agentctl.AgentEvent) {}, nil, nil))
	waitForWSConnected(t, mock)
	mgr := newTestManager(t)
	mgr.streamManager.setAgentDeliveryRepository(repo)
	owner := ExecutionOwner{Kind: ExecutionOwnerErrand, WorkspaceID: "workspace", TaskID: "task-live", ErrandID: "child-live", Generation: 1}
	execution := &AgentExecution{ID: "execution-live", Owner: owner, WorkspaceID: owner.WorkspaceID, DeliveryMode: DurableDeliveryV1, DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1, DeliveryStreamID: "stream", promptGeneration: 1, promptTurnID: "turn", agentctl: client}
	execution.setDeliverySubmissionID("submission")
	require.NoError(t, mgr.executionStore.Add(execution))
	target := ErrandPermissionTarget{Owner: owner, ExecutionID: "execution-live", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream"}
	for name, mutate := range map[string]func(*ErrandPermissionTarget){
		"execution":   func(value *ErrandPermissionTarget) { value.ExecutionID = "other" },
		"task":        func(value *ErrandPermissionTarget) { value.Owner.TaskID = "other" },
		"workspace":   func(value *ErrandPermissionTarget) { value.Owner.WorkspaceID = "other" },
		"owner":       func(value *ErrandPermissionTarget) { value.Owner.ErrandID = "other" },
		"generation":  func(value *ErrandPermissionTarget) { value.Owner.Generation++ },
		"stream":      func(value *ErrandPermissionTarget) { value.StreamID = "other" },
		"incarnation": func(value *ErrandPermissionTarget) { value.IncarnationID = "other" },
		"harness":     func(value *ErrandPermissionTarget) { value.HarnessGeneration++ },
	} {
		wrong := target
		mutate(&wrong)
		_, err = mgr.SnapshotErrandPermissions(ctx, wrong)
		require.Error(t, err, name)
	}
	require.Zero(t, listCalls.Load())
	permissionOwner := models.ErrandPermissionOwner{ErrandID: owner.ErrandID, TaskID: owner.TaskID, Generation: owner.Generation, ExecutionID: target.ExecutionID}
	lookup := func(request, pending string) models.ErrandPermissionSource {
		source, lookupErr := repo.LookupErrandPermissionSource(ctx, permissionOwner, request, pending)
		require.NoError(t, lookupErr)
		return *source
	}
	missing := models.ErrandPermissionSource{Owner: permissionOwner, RequestID: "request", PendingID: "pending"}
	snapshot, err := mgr.SnapshotErrandPermissions(ctx, target)
	require.NoError(t, err)
	require.Len(t, snapshot.Permissions, 4)
	_, err = mgr.ResolveErrandPermission(ctx, snapshot, missing, "allow")
	require.Error(t, err, "live membership alone must not prove prompt attribution")
	require.Zero(t, resolveCalls.Load())
	require.True(t, execution.agentctlLifecycleMu.TryLock(), "failed source check must release client lease")
	execution.agentctlLifecycleMu.Unlock()
	agent := streams.AgentEvent{Type: streams.EventTypePermissionRequest, RequestID: "request", PendingID: "pending", TurnID: "turn", PromptGeneration: 1}
	payload, err := json.Marshal(agent)
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{SessionID: "errand:child-live", TaskID: "task-live", ExecutionID: "execution-live", IncarnationID: "incarnation", HarnessGeneration: 1, OwnerGeneration: 1, ExpectedStreamID: "stream", StreamID: "stream", Sequence: 1, SubmissionID: "submission", EventType: agent.Type, Payload: payload}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	source := lookup("request", "pending")
	forgedSource := source
	forgedSource.TurnID = "other"
	_, err = mgr.ResolveErrandPermission(ctx, snapshot, forgedSource, "allow")
	require.Error(t, err)
	snapshot.Permissions[0].Options[0].OptionID = "forged"
	_, err = mgr.ResolveErrandPermission(ctx, snapshot, source, "forged")
	require.Error(t, err)
	require.Zero(t, resolveCalls.Load())
	holdResolve.Store(true)
	resolved := make(chan error, 1)
	go func() {
		_, resolveErr := mgr.ResolveErrandPermission(ctx, snapshot, source, "allow")
		resolved <- resolveErr
	}()
	select {
	case <-resolveEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("permission RPC did not reach agentctl")
	}
	require.False(t, execution.agentctlLifecycleMu.TryLock(), "client must stay leased until the RPC completes")
	releaseOnce.Do(func() { close(releaseResolve) })
	require.NoError(t, <-resolved)
	holdResolve.Store(false)
	require.True(t, execution.agentctlLifecycleMu.TryLock(), "successful RPC must release client lease")
	execution.agentctlLifecycleMu.Unlock()
	require.EqualValues(t, 1, resolveCalls.Load())
	agent.RequestID, agent.PendingID = "cancel-request", "cancel-pending"
	cancelPayload, err := json.Marshal(agent)
	require.NoError(t, err)
	cancelEvent := *event
	cancelEvent.Sequence, cancelEvent.Payload = 2, cancelPayload
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, &cancelEvent, 2)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, &cancelEvent, nil)
	require.NoError(t, err)
	_, err = mgr.CancelErrandPermission(ctx, snapshot, lookup("cancel-request", "cancel-pending"))
	require.NoError(t, err)
	require.EqualValues(t, 1, cancelCalls.Load())
	agent.RequestID, agent.PendingID = "old-request", "old-pending"
	oldPayload, err := json.Marshal(agent)
	require.NoError(t, err)
	oldEvent := *event
	oldEvent.Sequence, oldEvent.Payload = 3, oldPayload
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, &oldEvent, 3)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, &oldEvent, nil)
	require.NoError(t, err)
	terminalAgent := streams.AgentEvent{Type: streams.EventTypeComplete, TurnID: "turn", PromptGeneration: 1}
	terminalPayload, err := json.Marshal(terminalAgent)
	require.NoError(t, err)
	terminal := *event
	terminal.Sequence, terminal.EventType, terminal.Terminal, terminal.Payload = 4, terminalAgent.Type, true, terminalPayload
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, &terminal, 4)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, &terminal, nil)
	require.NoError(t, err)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, "stream", 4, models.DeliverySubmissionCompleted, time.Time{})
	require.NoError(t, err)
	require.True(t, settled)
	next := &models.AgentDeliverySubmission{ID: "next", Payload: []byte("next"), PayloadHash: journal.SubmissionHash([]byte("next"))}
	_, err = repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{ErrandID: "child-live", Generation: 1, ExecutionID: "execution-live", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", PromptGeneration: 2, TurnID: "turn", Submission: next})
	require.NoError(t, err)
	execution.promptLifecycleMu.Lock()
	execution.promptGeneration = 2
	execution.setDeliverySubmissionID("next")
	execution.promptLifecycleMu.Unlock()
	snapshot, err = mgr.SnapshotErrandPermissions(ctx, target)
	require.NoError(t, err)
	_, err = mgr.ResolveErrandPermission(ctx, snapshot, lookup("old-request", "old-pending"), "allow")
	require.NoError(t, err, "an older request still live on this execution remains answerable")
	require.EqualValues(t, 2, resolveCalls.Load())
	agent.RequestID, agent.PendingID, agent.PromptGeneration = "new-request", "new-pending", 2
	nextPayload, err := json.Marshal(agent)
	require.NoError(t, err)
	nextEvent := *event
	nextEvent.Sequence, nextEvent.SubmissionID, nextEvent.Payload = 5, "next", nextPayload
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, &nextEvent, 5)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, &nextEvent, nil)
	require.NoError(t, err)
	_, err = mgr.ResolveErrandPermission(ctx, snapshot, lookup("new-request", "new-pending"), "allow")
	require.NoError(t, err)
	require.EqualValues(t, 3, resolveCalls.Load())
	snapshot, err = mgr.SnapshotErrandPermissions(ctx, target)
	require.NoError(t, err)
	replacement := createTestClient(t, mock.server.URL)
	t.Cleanup(replacement.Close)
	execution.agentctlLifecycleMu.Lock()
	started, done := make(chan struct{}), make(chan error, 1)
	go func() {
		close(started)
		_, resolveErr := mgr.ResolveErrandPermission(ctx, snapshot, lookup("new-request", "new-pending"), "allow")
		done <- resolveErr
	}()
	<-started
	execution.replaceAgentctlClient(replacement)
	execution.agentctlLifecycleMu.Unlock()
	err = <-done
	require.Error(t, err)
	var notSent *agentctl.RequestNotSentError
	require.ErrorAs(t, err, &notSent)
	require.EqualValues(t, 3, resolveCalls.Load())
	require.True(t, execution.agentctlLifecycleMu.TryLock(), "stale client rejection must release lease")
	execution.agentctlLifecycleMu.Unlock()
}
