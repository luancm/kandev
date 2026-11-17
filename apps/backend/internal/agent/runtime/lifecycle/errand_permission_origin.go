package lifecycle

import (
	"context"
	"errors"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

func resolveErrandPermissionOrigin(ctx context.Context, delivery AgentDeliveryRepository, event *models.AgentDeliveryEvent) (*models.ErrandPermissionOrigin, error) {
	resolver, ok := delivery.(interface {
		ResolveErrandPermissionOrigin(context.Context, *models.AgentDeliveryEvent) (*models.ErrandPermissionOrigin, error)
	})
	if !ok {
		return nil, errors.New("errand permission origin unavailable")
	}
	return resolver.ResolveErrandPermissionOrigin(ctx, event)
}

func (e *AgentExecution) errandPermissionCurrent(event agentctl.AgentEvent) bool {
	if e == nil || event.PromptGeneration == 0 || event.DeliverySubmissionID == "" || event.TurnID == "" {
		return false
	}
	e.promptLifecycleMu.Lock()
	defer e.promptLifecycleMu.Unlock()
	return e.promptGeneration == event.PromptGeneration && e.promptCompletionGeneration != event.PromptGeneration && e.promptTurnID == event.TurnID && e.deliverySubmissionIDSnapshot() == event.DeliverySubmissionID
}
