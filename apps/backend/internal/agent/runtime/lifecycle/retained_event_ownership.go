package lifecycle

import (
	"context"
	"encoding/json"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

type retainedEventTurnResolver interface {
	ResolveAgentDeliveryEventTurn(context.Context, *models.AgentDeliveryEvent) (string, bool, error)
}

func resolveRetainedEventTurn(ctx context.Context, delivery AgentDeliveryRepository, retained *models.AgentDeliveryEvent, event agentctl.AgentEvent) (agentctl.AgentEvent, bool, error) {
	resolver, ok := delivery.(retainedEventTurnResolver)
	if !ok {
		return event, false, nil
	}
	turn, errand, err := resolver.ResolveAgentDeliveryEventTurn(ctx, retained)
	if err == nil && turn != "" && event.TurnID == "" {
		event.TurnID = turn
	}
	return event, errand, err
}

// Intake resolves callback attribution while leaving the received payload immutable.
func prepareRetainedEventTurn(ctx context.Context, delivery AgentDeliveryRepository, retained *models.AgentDeliveryEvent, event agentctl.AgentEvent) (agentctl.AgentEvent, bool, error) {
	if retained != nil && event.DeliverySubmissionID == "" && event.TurnID == "" {
		// The intake envelope may include a lifecycle-derived turn for ordinary
		// native activity. Errand attribution derives its own turn from retained
		// stream evidence, so resolve against the producer event without that
		// enrichment while leaving the receipt bytes untouched.
		source := *retained
		payload, err := json.Marshal(event)
		if err != nil {
			return event, false, err
		}
		source.Payload = payload
		retained = &source
	}
	return resolveRetainedEventTurn(ctx, delivery, retained, event)
}
