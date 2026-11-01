package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) reconcileErrandTerminalDecisions(ctx context.Context, sessionID string) error {
	controller, ok := s.errands.(interface {
		ReconcileTerminalDecisions(context.Context, string) error
	})
	if !ok {
		return nil
	}
	return controller.ReconcileTerminalDecisions(ctx, sessionID)
}

func (s *Service) reserveErrandTerminalIntent(ctx context.Context, session *models.TaskSession, origin errands.ParentPromptOrigin, turnID, submissionID, executionID string) error {
	store, ok := s.repo.(interface {
		ReserveErrandTurnSettlement(context.Context, string, *models.AgentDeliveryEffect) error
	})
	if !ok || !models.IsErrandSession(session) {
		return nil
	}
	if submissionID == "" {
		submissionID = "errand-turn:" + turnID
	}
	incarnationID := session.QueueIncarnationID
	generation, err := s.currentDeliveryGeneration(ctx, session, incarnationID)
	if err != nil {
		return err
	}
	source := "user"
	if origin.SenderSessionID == session.ParentSessionID {
		source = "parent"
	}
	submissionID = normalizeDeliverySubmissionID(submissionID)
	return store.ReserveErrandTurnSettlement(ctx, session.TaskID, &models.AgentDeliveryEffect{
		EffectKey:  fmt.Sprintf("errand.turn:%s:%s:%s:%s", session.TaskID, incarnationID, turnID, submissionID),
		EffectType: db.ErrandTurnSettlementEffect, State: models.DeliveryEffectPending, SessionID: session.ID,
		IncarnationID: incarnationID, HarnessGeneration: generation, StreamID: executionID,
		TurnID: turnID, SubmissionID: submissionID, Outcome: source,
	})
}
