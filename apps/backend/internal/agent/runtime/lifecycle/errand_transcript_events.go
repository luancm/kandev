package lifecycle

import (
	"context"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// A transcript change carries scope only. Browsers read canonical rows through
// their task authorization and retain no copy of the raw agent event.
func (p *EventPublisher) publishErrandTranscriptChange(execution *AgentExecution, event agentctl.AgentEvent) {
	if p == nil || p.eventBus == nil || execution == nil || execution.Owner.Kind != ExecutionOwnerErrand || event.DeliverySequence == 0 {
		return
	}
	switch event.Type {
	case streams.EventTypePermissionRequest, streams.EventTypeToolCall, streams.EventTypeToolUpdate,
		streams.EventTypeShellOutputUpdate, streams.EventTypeComplete:
	default:
		if !event.CanonicalProjection {
			return
		}
	}
	payload := map[string]any{"task_id": execution.Owner.TaskID, "errand_id": execution.Owner.ErrandID}
	if err := p.eventBus.Publish(context.Background(), events.ErrandTranscriptChanged,
		bus.NewEvent(events.ErrandTranscriptChanged, "agent-manager", payload)); err != nil {
		p.logger.Debug("failed to publish errand transcript change", zap.Error(err))
	}
}
