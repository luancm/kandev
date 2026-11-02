package errands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

const terminalParentOrigin = "parent"
const terminalInterruptedCause = "interrupted"

// ReconcileTerminalDecisions replays exact sealed terminal intents before
// admission. Unsealed/missing terminal evidence remains a durable fence.
func (s *Service) ReconcileTerminalDecisions(ctx context.Context, sessionID string) error {
	pending, err := s.store.PendingErrandTurnSettlements(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, effect := range pending {
		if !strings.Contains(effect.Outcome, ":") {
			return db.ErrErrandSettlementPending
		}
		if err := s.store.ReserveErrandTurnSettlement(ctx, effect.TaskID, effect); err != nil {
			return err
		}
		if err := s.settleTerminalDecision(ctx, effect, TurnSettlement{}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) settleExactTerminal(ctx context.Context, settlement TurnSettlement) error {
	pending, err := s.store.PendingErrandTurnSettlements(ctx, settlement.SessionID)
	if err != nil {
		return err
	}
	for _, effect := range pending {
		if effect.TurnID != settlement.TurnID {
			continue
		}
		if settlement.SubmissionID != "" && settlement.SubmissionID != effect.SubmissionID {
			return nil
		}
		origin, _, _ := strings.Cut(effect.Outcome, ":")
		cause := string(settlement.Outcome)
		if settlement.Interrupted {
			cause = terminalInterruptedCause
		}
		if !strings.Contains(effect.Outcome, ":") {
			effect.Outcome = origin + ":" + cause
		}
		if err := s.store.ReserveErrandTurnSettlement(ctx, effect.TaskID, effect); err != nil {
			return err
		}
		return s.settleTerminalDecision(ctx, effect, settlement)
	}
	return nil
}

func (s *Service) settleTerminalDecision(ctx context.Context, effect *models.AgentDeliveryEffect, observed TurnSettlement) error {
	err := s.commitTerminalDecision(ctx, effect, observed)
	if err != nil {
		return fmt.Errorf("commit exact errand terminal decision: %w", err)
	}
	return nil
}

func (s *Service) commitTerminalDecision(ctx context.Context, effect *models.AgentDeliveryEffect, observed TurnSettlement) error {
	view, err := s.readTurn(ctx, effect.SessionID, effect.TurnID)
	if err != nil {
		return err
	}
	if !view.Completed {
		return db.ErrErrandSettlementPending
	}
	origin, cause, sealed := strings.Cut(effect.Outcome, ":")
	if !sealed {
		return db.ErrErrandSettlementPending
	}
	settlement := TurnSettlement{SessionID: effect.SessionID, TurnID: effect.TurnID, SubmissionID: effect.SubmissionID,
		Outcome: models.ErrandOutcomeDone, FailureSummary: observed.FailureSummary, Interrupted: cause == terminalInterruptedCause}
	if cause == "failed" || cause == terminalInterruptedCause {
		settlement.Outcome = models.ErrandOutcomeFailed
	}
	text, truncated, kind := resultOf(settlement, view, origin == terminalParentOrigin)
	for attempt := 0; attempt < maxUpdateAttempts; attempt++ {
		row, err := s.store.GetErrand(ctx, effect.SessionID)
		if err != nil {
			return err
		}
		unlock := s.parentLocks.lock(row.ParentSessionID)
		alreadyCaptured := row.ResultTurnID == effect.TurnID && resultAlreadyCaptured(row.ResultState)
		next := *row
		changed := applySettlement(&next, settlement, origin == terminalParentOrigin, text, truncated, kind, s.clock.Now())
		decision := *effect
		at := s.clock.Now()
		decision.State, decision.CompletedAt = models.DeliveryEffectCompleted, &at
		applied, commitErr := s.store.CommitErrandTurnSettlement(ctx, &next, &decision)
		err = commitErr
		unlock()
		if err == nil {
			if changed {
				if applied {
					s.recordTerminalSettlement(&next, settlement, origin == terminalParentOrigin, alreadyCaptured)
				}
				s.afterResultWrite(ctx, &next)
			}
			return nil
		}
		if !errors.Is(err, models.ErrErrandVersionConflict) {
			return err
		}
	}
	return models.ErrErrandVersionConflict
}

func (s *Service) recordTerminalSettlement(row *models.Errand, settlement TurnSettlement, parentInitiated, alreadyCaptured bool) {
	if !parentInitiated || alreadyCaptured || row.Lifecycle == models.ErrandLifecycleCancelled {
		return
	}
	status := models.ErrandStatusDone
	if settlement.Outcome == models.ErrandOutcomeFailed {
		status = models.ErrandStatusFailed
	}
	countSettled(status)
	s.logger.Info("errand.settled", s.errandFields(row, "status", string(status))...)
}
