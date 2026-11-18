package lifecycle

import (
	"encoding/json"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestErrandUpdatesReconnectReoffersRetainedAutomaticPermission(t *testing.T) {
	ctx := t.Context()
	repo, _ := runOwnerTestRepository(t)
	owner := ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "child", Generation: 1, WorkspaceID: "workspace"}
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: owner.TaskID, Title: "task"}))
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{
		ID: owner.ErrandID, TaskID: owner.TaskID, ParentSessionID: "parent", Lifecycle: models.ErrandLifecycleLaunched,
		ResultState: models.ErrandResultNone,
	}))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, owner.ErrandID, 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, owner.ErrandID, owner.Generation, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, repo.CreateErrandTurn(ctx, owner.ErrandID, &models.Turn{ID: "turn", TaskID: owner.TaskID}))

	peer := newMockAgentServer(t)
	t.Cleanup(peer.Close)
	requests := make(chan agentctl.AgentEvent, 2)
	stream := NewStreamManager(newSessionTestLogger(), StreamCallbacks{OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) {
		requests <- event
	}}, nil, nil)
	t.Cleanup(stream.Wait)
	stream.setAgentDeliveryRepository(repo)
	sm := NewSessionManager(newSessionTestLogger(), nil)
	sm.SetDependencies(nil, stream, nil, nil)
	client := createTestClient(t, peer.server.URL)
	t.Cleanup(client.Close)
	execution := &AgentExecution{
		ID: "execution", Owner: owner, ACPSessionID: "native", DeliveryMode: DurableDeliveryV1, agentctl: client,
		DeliveryIncarnationID: "incarnation", DeliveryStreamID: "stream", DeliveryHarnessGeneration: 1,
		promptTurnID: "turn", promptGeneration: 1,
	}
	require.NoError(t, sm.bindErrandDelivery(ctx, execution))
	stored, err := sm.persistErrandDeliveryBeforeDispatch(ctx, execution, "hello", nil, "original", 1)
	require.NoError(t, err)
	execution.setDeliverySubmissionID(stored.ID)
	permission := agentctl.AgentEvent{
		Type: streams.EventTypePermissionRequest, RequestID: "request", PendingID: "pending",
		TurnID: "turn", PromptGeneration: 1, AutoApprovalPending: true, AutoApprovedOptionID: "exact-allow",
		AutoApprovedOptionKind: string(streams.PermissionOptionKindAllowOnce), AutoApprovalSource: streams.PermissionDecisionSourceAutoApprove,
	}
	payload, err := json.Marshal(permission)
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{
		SessionID: stored.SessionID, TaskID: owner.TaskID, ExecutionID: execution.ID, IncarnationID: stored.IncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, ExpectedStreamID: execution.DeliveryStreamID, StreamID: execution.DeliveryStreamID,
		Sequence: 1, SubmissionID: stored.ID, EventType: permission.Type, Payload: payload,
	}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	peer.deliveryStatus = &agentctl.DeliveryStatus{
		StorageCapability: journal.StorageCapability{Durable: true, Version: journal.CurrentProtocolVersion},
		SessionID:         stored.SessionID, IncarnationID: stored.IncarnationID, HarnessGeneration: 1, StreamID: execution.DeliveryStreamID,
		Stream: &journal.Stream{
			SessionID: stored.SessionID, IncarnationID: stored.IncarnationID, HarnessGeneration: 1, StreamID: execution.DeliveryStreamID,
			HighWater: 1, Acknowledged: 1, FirstRetained: 2,
		},
	}
	stream.connectUpdatesStream(execution, nil)
	require.True(t, client.HasAgentStream(), "the exact updates stream must have attached")
	select {
	case proposal := <-requests:
		require.Equal(t, permission.RequestID, proposal.RequestID)
		require.Equal(t, permission.PendingID, proposal.PendingID)
		require.Equal(t, permission.AutoApprovedOptionID, proposal.AutoApprovedOptionID)
		require.Equal(t, stored.ID, proposal.DeliverySubmissionID)
	default:
		t.Fatal("reconnecting the updates stream should reoffer the already-projected automatic proposal")
	}
}
