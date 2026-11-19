package lifecycle

import (
	"context"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

func (sm *StreamManager) reofferRetainedErrandPermissions(ctx context.Context, execution *AgentExecution, repository AgentDeliveryRepository) {
	if execution.Owner.Kind != ExecutionOwnerErrand || sm.callbacks.OnAgentEvent == nil {
		return
	}
	origin := agentctl.AgentEvent{
		TurnID: execution.promptTurnIDSnapshot(), PromptGeneration: execution.promptGenerationSnapshot(),
		DeliverySubmissionID: execution.deliverySubmissionIDSnapshot(),
	}
	if !execution.errandPermissionCurrent(origin) {
		return
	}
	reader, ok := repository.(interface {
		ListErrandAutomaticPermissionProposals(context.Context, models.ErrandPermissionOwner, string, string) ([]agentctl.AgentEvent, error)
	})
	if !ok {
		return
	}
	owner := models.ErrandPermissionOwner{
		TaskID: execution.Owner.TaskID, ErrandID: execution.Owner.ErrandID,
		Generation: execution.Owner.Generation, ExecutionID: execution.ID,
	}
	proposals, err := reader.ListErrandAutomaticPermissionProposals(ctx, owner, origin.TurnID, origin.DeliverySubmissionID)
	if err != nil {
		sm.logger.Debug("read retained Errand permission proposals", zap.Error(err))
		return
	}
	if sm.isExecutionCurrent != nil && !sm.isExecutionCurrent(execution) {
		return
	}
	for _, proposal := range proposals {
		if execution.errandPermissionCurrent(proposal) {
			sm.callbacks.OnAgentEvent(execution, proposal)
		}
	}
}
