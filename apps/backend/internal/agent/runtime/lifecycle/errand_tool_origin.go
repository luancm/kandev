package lifecycle

import (
	"context"
	"errors"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"

	"github.com/kandev/kandev/internal/task/models"
)

func resolveErrandToolOrigin(ctx context.Context, delivery AgentDeliveryRepository, event *models.AgentDeliveryEvent) (*models.ErrandToolOrigin, error) {
	resolver, ok := delivery.(interface {
		ResolveErrandToolOrigin(context.Context, *models.AgentDeliveryEvent) (*models.ErrandToolOrigin, error)
	})
	if !ok {
		return nil, errors.New("errand tool origin unavailable")
	}
	return resolver.ResolveErrandToolOrigin(ctx, event)
}

func errandToolOriginCurrent(execution *AgentExecution, origin *models.ErrandToolOrigin) bool {
	if origin == nil {
		return false
	}
	return execution != nil && execution.promptGenerationSnapshot() == origin.PromptGeneration && execution.deliverySubmissionIDSnapshot() == origin.SubmissionID
}

func (e *AgentExecution) applyErrandToolActivity(event agentctl.AgentEvent) bool {
	if e == nil || event.PromptGeneration == 0 || event.DeliverySubmissionID == "" {
		return false
	}
	e.promptLifecycleMu.Lock()
	defer e.promptLifecycleMu.Unlock()
	if e.promptGeneration != event.PromptGeneration || e.promptCompletionGeneration == event.PromptGeneration || e.deliverySubmissionIDSnapshot() != event.DeliverySubmissionID {
		return false
	}
	if event.ParentToolCallID != "" {
		return true
	}
	switch event.Type {
	case streams.EventTypeToolCall:
		e.setActiveTool(activeTopLevelTool{ToolCallID: event.ToolCallID, Name: event.ToolName, Title: event.ToolTitle, Status: event.ToolStatus})
	case streams.EventTypeToolUpdate:
		if isTerminalToolUpdate(event) {
			e.clearActiveTool(event.ToolCallID)
		}
	}
	return true
}
