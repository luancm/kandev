package lifecycle

import (
	"context"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

type retainedEventTurnResolver interface {
	ResolveAgentDeliveryEventTurn(context.Context, *models.AgentDeliveryEvent) (string, error)
}

func resolveRetainedEventTurn(ctx context.Context, delivery AgentDeliveryRepository, retained *models.AgentDeliveryEvent, event agentctl.AgentEvent) (agentctl.AgentEvent, error) {
	resolver, ok := delivery.(retainedEventTurnResolver)
	if !ok {
		return event, nil
	}
	turn, err := resolver.ResolveAgentDeliveryEventTurn(ctx, retained)
	if err == nil && turn != "" && event.TurnID == "" {
		event.TurnID = turn
	}
	return event, err
}
