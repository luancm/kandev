package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// errandTurnSettler is the settlement part of the errands service. It is
// type-asserted from ErrandController, which keeps its own method set fixed.
type errandTurnSettler interface {
	OnErrandTurnSettled(ctx context.Context, settlement errands.TurnSettlement) error
	OnParentTurnSettled(ctx context.Context, parentSessionID string, cause errands.SettleCause) (bool, error)
}

type errandTurnCanceller interface {
	OnErrandTurnCancelled(ctx context.Context, sessionID, turnID, submissionID string) error
}

// errandOutstandingReason is the delivery workflow no-op reason recorded when
// a parent turn ends while its errands still owe an outcome.
const errandOutstandingReason = "errands_outstanding"

func (s *Service) errandSettler() (errandTurnSettler, bool) {
	if s.errands == nil {
		return nil, false
	}
	settler, ok := s.errands.(errandTurnSettler)
	return settler, ok
}

// settleErrandTurn records a settled errand turn. Only the errand's own
// settled turn is captured; a session that is not an errand is ignored.
func (s *Service) settleErrandTurn(
	ctx context.Context, sessionID, turnID string, outcome models.ErrandOutcome, failureSummary string, submissionIDs ...string,
) {
	settler, ok := s.errandSettler()
	if !ok || sessionID == "" || turnID == "" {
		return
	}
	submissionID := ""
	if len(submissionIDs) > 0 {
		submissionID = submissionIDs[0]
	}
	err := settler.OnErrandTurnSettled(ctx, errands.TurnSettlement{
		SessionID: sessionID, TurnID: turnID, SubmissionID: submissionID,
		Outcome: outcome, FailureSummary: failureSummary,
	})
	if err != nil {
		s.logger.Warn("failed to record errand turn settlement",
			zap.String("session_id", sessionID), zap.String("turn_id", turnID), zap.Error(err))
	}
}

func (s *Service) settleCancelledErrandTurn(ctx context.Context, sessionID, turnID, submissionID string) {
	canceller, ok := s.errands.(errandTurnCanceller)
	if !ok || sessionID == "" || turnID == "" {
		return
	}
	if err := s.reconcileAgentDeliverySettlements(ctx, sessionID); err != nil {
		s.logger.Warn("skipping interrupted errand settlement while durable terminal evidence is unresolved",
			zap.String("session_id", sessionID), zap.String("turn_id", turnID), zap.Error(err))
		return
	}
	if err := canceller.OnErrandTurnCancelled(ctx, sessionID, turnID, submissionID); err != nil {
		s.logger.Warn("failed to settle cancelled errand turn",
			zap.String("session_id", sessionID), zap.String("turn_id", turnID), zap.Error(err))
	}
}

// settleParentTurn runs the parent-side settlement and reports whether the
// parent still owes an errand outcome. A failure is logged and treated as not
// outstanding, so the turn completes as it would without errands.
func (s *Service) settleParentTurn(ctx context.Context, sessionID string, cause errands.SettleCause) bool {
	settler, ok := s.errandSettler()
	if !ok || sessionID == "" {
		return false
	}
	outstanding, err := settler.OnParentTurnSettled(ctx, sessionID, cause)
	if err != nil {
		s.logger.Warn("failed to settle errands of parent turn",
			zap.String("session_id", sessionID), zap.Error(err))
		return false
	}
	return outstanding
}

// evaluateCompletedAgentWorkflow runs the workflow half of a completed agent
// turn. An errand turn settles its result; a parent turn settles its errands
// before the queue drain. It reports whether the workflow transitioned, and
// whether the parent is outstanding with no turn started and no transition,
// which the caller settles without writing task review state.
func (s *Service) evaluateCompletedAgentWorkflow(
	completionCtx, transitionCtx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
) (transitioned, outstanding bool) {
	if models.IsWorkflowDetachedSession(session) {
		if err := s.recordNotRequiredDeliveryWorkflowOutcome(completionCtx, data.TaskID, workflowEffectFromContext(completionCtx)); err != nil {
			s.logger.Warn("failed to record non-workflow completion outcome",
				zap.String("task_id", data.TaskID), zap.String("session_id", data.SessionID), zap.Error(err))
		}
		if models.IsErrandSession(session) {
			s.settleErrandTurn(completionCtx, data.SessionID, data.TurnID, models.ErrandOutcomeDone, "", data.DeliverySubmissionID)
		}
		return false, false
	}
	outstanding = s.settleParentTurn(completionCtx, data.SessionID, errands.SettleCauseTurnDone)
	if s.drainQueuedBeforeWorkflowTransition(completionCtx, data.TaskID, data.SessionID, session) {
		return false, false
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

// errandParentAdmitter is the parent-prompt part of the errands service.
// Type-asserted from ErrandController.
type errandParentAdmitter interface {
	OnParentPromptAdmitted(ctx context.Context, errandSessionID, senderSessionID string) error
}

// OnErrandParentPromptAdmitted records that a session admitted a prompt into an
// errand. Callers pass the sender session they verified; the errands service
// acts only when that session is the errand's parent.
func (s *Service) OnErrandParentPromptAdmitted(ctx context.Context, errandSessionID, senderSessionID string) error {
	if s.errands == nil {
		return nil
	}
	admitter, ok := s.errands.(errandParentAdmitter)
	if !ok {
		return nil
	}
	return admitter.OnParentPromptAdmitted(ctx, errandSessionID, senderSessionID)
}
