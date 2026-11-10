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
	if err == nil && errand {
		event.TurnID = turn
	}
	return event, errand, err
}

// Intake validates producer ownership before retaining lifecycle-derived fields.
// Existing inbox replay uses resolveRetainedEventTurn without rewriting bytes.
func prepareRetainedEventTurn(ctx context.Context, delivery AgentDeliveryRepository, retained *models.AgentDeliveryEvent, event agentctl.AgentEvent) (agentctl.AgentEvent, bool, error) {
	source := *retained
	payload, err := json.Marshal(event)
	if err != nil {
		return event, false, err
	}
	source.Payload = payload
	resolved, errand, err := resolveRetainedEventTurn(ctx, delivery, &source, event)
	if err != nil || !errand {
		return resolved, errand, err
	}
	var stored agentctl.AgentEvent
	if err = json.Unmarshal(retained.Payload, &stored); err != nil {
		return event, true, err
	}
	stored.TurnID = event.TurnID
	retained.Payload, err = json.Marshal(stored)
	return resolved, true, err
}
