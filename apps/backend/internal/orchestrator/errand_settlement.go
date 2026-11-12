package orchestrator

import (
	"context"
	"time"

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
) error {
	settler, ok := s.errandSettler()
	if !ok || sessionID == "" || turnID == "" {
		return nil
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
	return err
}

func (s *Service) settleCompletedErrandTurn(ctx context.Context, data watcher.AgentEventData, session *models.TaskSession, turnID string) error {
	settler, ok := s.errandSettler()
	if !ok || session == nil || !models.IsErrandSession(session) || session.ID == "" || turnID == "" {
		return nil
	}
	target := models.ErrandTeardownTarget{
		TaskID: session.TaskID, SessionID: session.ID, SessionIncarnation: session.QueueIncarnationID,
		ExecutionID: data.AgentExecutionID, TurnID: turnID, PromptGeneration: data.PromptGeneration,
	}
	err := settler.OnErrandTurnSettled(ctx, errands.TurnSettlement{
		SessionID: session.ID, TurnID: turnID, SubmissionID: data.DeliverySubmissionID,
		Outcome: models.ErrandOutcomeDone, CompletionTarget: &target,
	})
	if err != nil {
		s.logger.Warn("failed to record successful errand completion",
			zap.String("session_id", session.ID), zap.String("turn_id", turnID), zap.Error(err))
	}
	return err
}

func (s *Service) scheduleErrandCompletionRetry(ctx context.Context, sessionID string) {
	controller, ok := s.errands.(interface {
		RetryErrandCompletion(context.Context, string) error
	})
	if !ok || sessionID == "" {
		return
	}
	go func() {
		attemptCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := controller.RetryErrandCompletion(attemptCtx, sessionID); err != nil {
			s.logger.Warn("errand completion cleanup remains pending",
				zap.String("session_id", sessionID), zap.Error(err))
		}
	}()
}

func (s *Service) settleCancelledErrandTurn(ctx context.Context, sessionID, turnID, submissionID string) {
	canceller, ok := s.errands.(errandTurnCanceller)
	if !ok || sessionID == "" || turnID == "" {
		return
	}
	if err := s.reconcileAgentDeliverySettlements(withSettledDeliverySessionStateConvergenceSkipped(ctx), sessionID); err != nil {
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
// parent still owes an errand outcome. Unreadable inventory defers task workflow
// settlement while leaving session completion independent.
func (s *Service) settleParentTurn(ctx context.Context, sessionID string, cause errands.SettleCause) (bool, error) {
	settler, ok := s.errandSettler()
	if !ok || sessionID == "" {
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
// turn. An errand turn settles its result; a parent turn settles its errands
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
		if models.IsErrandSession(session) {
			return false, false
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
