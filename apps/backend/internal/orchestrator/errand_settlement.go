package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// errandOutstandingReason is the delivery workflow no-op reason recorded when
// a parent turn ends while its errands still owe an outcome.
const errandOutstandingReason = "errands_outstanding"

// settleParentTurn runs the parent-side settlement and reports whether the
// parent still owes an errand outcome. Unreadable inventory defers task workflow
// settlement while leaving session completion independent.
func (s *Service) settleParentTurn(ctx context.Context, sessionID string, cause errands.SettleCause) (bool, error) {
	settler := s.errands
	if settler == nil || sessionID == "" {
		return false, nil
	}
	outstanding, err := settler.OnParentTurnSettled(ctx, sessionID, cause)
	if err != nil {
		s.logger.Warn("failed to settle errands of parent turn",
			zap.String("session_id", sessionID), zap.Error(err))
	}
	return outstanding, err
}

// evaluateCompletedAgentWorkflow runs the workflow half of a completed agent
// turn. A parent turn settles its errands
// before the queue drain. It reports whether the workflow transitioned, and
// whether task workflow is deferred with no turn started and no transition,
// which the caller settles without writing task review state.
func (s *Service) evaluateCompletedAgentWorkflow(
	completionCtx, transitionCtx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
) (transitioned, deferTaskWorkflow bool) {
	if models.IsWorkflowDetachedSession(session) {
		if err := s.recordNotRequiredDeliveryWorkflowOutcome(completionCtx, data.TaskID, workflowEffectFromContext(completionCtx)); err != nil {
			s.logger.Warn("failed to record non-workflow completion outcome",
				zap.String("task_id", data.TaskID), zap.String("session_id", data.SessionID), zap.Error(err))
		}
		return false, false
	}
	outstanding, settleErr := s.settleParentTurn(completionCtx, data.SessionID, errands.SettleCauseTurnDone)
	if s.drainQueuedBeforeWorkflowTransition(completionCtx, data.TaskID, data.SessionID, session, outstanding || settleErr != nil) {
		return false, false
	}
	if settleErr != nil {
		return false, true
	}
	if outstanding {
		s.recordErrandsOutstandingNoOp(completionCtx, data.TaskID, data.SessionID)
		return false, true
	}
	return s.processOnTurnCompleteViaEngineWithCause(transitionCtx, data.TaskID, session, agentTurnCompletionCause(data)), false
}

// settleOutstandingAgentReadyTurn settles a parent whose turn ended while its
// errands are outstanding. The parent becomes promptable again without a
// workflow evaluation or task review state.
func (s *Service) settleOutstandingAgentReadyTurn(
	completionCtx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
) {
	s.recordErrandsOutstandingNoOp(completionCtx, data.TaskID, data.SessionID)
	s.setSessionWaitingForInputWithoutTaskState(completionCtx, data.TaskID, data.SessionID, session)
}

// recordErrandsOutstandingNoOp records that a parent turn completed without a
// workflow transition because its errands are outstanding.
func (s *Service) recordErrandsOutstandingNoOp(ctx context.Context, taskID, sessionID string) {
	if err := s.recordDeliveryWorkflowOutcomeWithReason(
		ctx, taskID, workflowEffectFromContext(ctx), errandOutstandingReason,
	); err != nil {
		s.logger.Warn("failed to record errands-outstanding workflow no-op",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
	}
}
