package lifecycle

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestErrandPermissionHandlerFencesHistoricalOrigin(t *testing.T) {
	mgr := newTestManager(t)
	eventBus := bus.NewMemoryEventBus(newTestLogger())
	mgr.eventPublisher = NewEventPublisher(eventBus, newTestLogger())
	received := make(chan *bus.Event, 2)
	_, err := eventBus.Subscribe(events.BuildPermissionRequestSubject("errand:child"), func(_ context.Context, event *bus.Event) error {
		received <- event
		return nil
	})
	require.NoError(t, err)
	execution := &AgentExecution{ID: "execution", Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "child", Generation: 1}, promptGeneration: 1, promptTurnID: "turn"}
	execution.setDeliverySubmissionID("first")
	old := agentctl.AgentEvent{Type: streams.EventTypePermissionRequest, RequestID: "old", PendingID: "pending-old", TurnID: "turn", DeliverySubmissionID: "first", PromptGeneration: 1}
	require.True(t, execution.errandPermissionCurrent(old))
	execution.promptLifecycleMu.Lock()
	execution.promptGeneration = 2
	execution.setDeliverySubmissionID("second")
	execution.promptLifecycleMu.Unlock()
	mgr.handleAgentEventAtContextResetBoundaryInternal(execution, old, false, "", false)
	select {
	case <-received:
		t.Fatal("historical permission must not notify the current owner")
	default:
	}
	current := old
	current.RequestID, current.PendingID = "current", "pending-current"
	current.DeliverySubmissionID, current.PromptGeneration = "second", 2
	mgr.handleAgentEventAtContextResetBoundaryInternal(execution, current, false, "", false)
	select {
	case event := <-received:
		data, marshalErr := json.Marshal(event.Data)
		require.NoError(t, marshalErr)
		var payload map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &payload))
		require.Equal(t, "second", payload["delivery_submission_id"])
		require.Equal(t, float64(2), payload["prompt_generation"])
		require.Equal(t, "turn", payload["turn_id"])
	case <-time.After(time.Second):
		t.Fatal("current permission notification missing")
	}
}

func TestErrandPermissionActivityChecksSubmissionAtMutation(t *testing.T) {
	mgr := newTestManager(t)
	mgr.executionStore = nil
	execution := &AgentExecution{ID: "execution", Owner: ExecutionOwner{Kind: ExecutionOwnerErrand, ErrandID: "child", Generation: 1}, Status: v1.AgentStatusReady, promptGeneration: 2, promptTurnID: "turn"}
	execution.setDeliverySubmissionID("first")
	event := agentctl.AgentEvent{Type: streams.EventTypePermissionRequest, TurnID: "turn", DeliverySubmissionID: "first", PromptGeneration: 2}
	require.True(t, execution.errandPermissionCurrent(event))
	execution.promptLifecycleMu.Lock()
	execution.setDeliverySubmissionID("second")
	execution.promptLifecycleMu.Unlock()
	beforeTime, beforeEpoch := execution.lastActivityAt, execution.promptActivityEpoch
	mgr.recordActivity(execution, event)
	require.Equal(t, beforeTime, execution.lastActivityAt)
	require.Equal(t, beforeEpoch, execution.promptActivityEpoch)
	event.DeliverySubmissionID = "second"
	mgr.recordActivity(execution, event)
	require.False(t, execution.lastActivityAt.IsZero())
	require.Equal(t, beforeEpoch+1, execution.promptActivityEpoch)
}
