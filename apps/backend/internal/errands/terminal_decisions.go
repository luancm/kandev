package errands

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

const terminalParentOrigin = "parent"
const terminalInterruptedCause = "interrupted"

// ReconcileTerminalDecisions replays exact sealed terminal intents before
// admission. Unsealed/missing terminal evidence remains a durable fence.
func (s *Service) ReconcileTerminalDecisions(ctx context.Context, sessionID string) error {
	if err := s.reconcileTerminalReceipts(ctx, sessionID); err != nil && !errors.Is(err, db.ErrErrandSettlementPending) {
		return err
	}
	if s.reconcileInterruptedDelegation != nil {
		if err := s.reconcileInterruptedDelegation(ctx, sessionID); err != nil {
			return err
		}
	}
	return s.reconcileTerminalReceipts(ctx, sessionID)
}

func (s *Service) reconcileTerminalReceipts(ctx context.Context, sessionID string) error {
	pending, err := s.store.PendingErrandTurnSettlements(ctx, sessionID)
	if err != nil && !errors.Is(err, db.ErrErrandSettlementPending) {
		return err
	}
	unresolved := err
	for _, effect := range pending {
		if !strings.Contains(effect.Outcome, ":") {
			unresolved = db.ErrErrandSettlementPending
			continue
		}
		if err := s.sealTerminalIntent(ctx, effect); err != nil {
			return err
		}
		if err := s.settleTerminalDecision(ctx, effect, TurnSettlement{}); err != nil {
			return err
		}
	}
	return unresolved
}

func (s *Service) settleExactTerminal(ctx context.Context, settlement TurnSettlement) error {
	pending, err := s.store.PendingErrandTurnSettlements(ctx, settlement.SessionID)
	if err != nil && !errors.Is(err, db.ErrErrandSettlementPending) {
		return err
	}
	unresolved := err
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
		if err := s.sealTerminalIntent(ctx, effect); err != nil {
			return err
		}
		return s.settleTerminalDecision(ctx, effect, settlement)
	}
	return unresolved
}

func (s *Service) sealTerminalIntent(ctx context.Context, effect *models.AgentDeliveryEffect) error {
	err := s.store.SealErrandTurnSettlement(ctx, effect.TaskID, effect)
	if errors.Is(err, sql.ErrNoRows) {
		return s.store.ReserveErrandTurnSettlement(ctx, effect.TaskID, effect)
	}
	return err
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
		Outcome: models.ErrandOutcomeDone, FailureSummary: observed.FailureSummary, Interrupted: cause == terminalInterruptedCause,
		CompletionTarget: observed.CompletionTarget}
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
		if err := s.prepareCompletionIntent(&next, settlement, effect); err != nil {
			unlock()
			return err
		}
		decision := *effect
		at := s.clock.Now()
		decision.State, decision.CompletedAt = models.DeliveryEffectCompleted, &at
		applied, commitErr := s.store.CommitErrandTurnSettlement(ctx, &next, &decision)
		err = commitErr
		unlock()
		if errors.Is(err, db.ErrErrandSettlementPending) {
			return s.store.CommitHistoricalErrandTurnReceipt(ctx, row.TaskID, &decision)
		}
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

func (s *Service) setCompletionIntent(row *models.Errand, settlement TurnSettlement, effect *models.AgentDeliveryEffect) error {
	if !eligibleCompletionSettlement(row, settlement, effect) {
		return nil
	}
	target := *settlement.CompletionTarget
	if err := validateCompletionTarget(row, target, effect); err != nil {
		return err
	}
	target.SubmissionID = effect.SubmissionID
	reuse, err := reuseCompletionIntent(row.TeardownIntent, target)
	if err != nil || reuse {
		return err
	}
	intent := &models.ErrandTeardownIntent{Version: models.ErrandTeardownIntentVersion, ID: uuid.NewString(), State: models.ErrandTeardownIntentPending, Purpose: models.ErrandTeardownPurposeCompletion, RequestedAt: s.clock.Now(), Target: target}
	if err := intent.ValidateFor(row); err != nil {
		return err
	}
	row.TeardownIntent = intent
	return nil
}

func (s *Service) prepareCompletionIntent(row *models.Errand, settlement TurnSettlement, effect *models.AgentDeliveryEffect) error {
	if settlement.CompletionTarget == nil && settlement.Outcome == models.ErrandOutcomeDone && !settlement.Interrupted && effect.StreamID != "" {
		settlement.CompletionTarget = &models.ErrandTeardownTarget{TaskID: row.TaskID, SessionID: row.ID, SessionIncarnation: effect.IncarnationID, ExecutionID: effect.StreamID, TurnID: effect.TurnID, SubmissionID: effect.SubmissionID}
	}
	return s.setCompletionIntent(row, settlement, effect)
}

func eligibleCompletionSettlement(row *models.Errand, settlement TurnSettlement, effect *models.AgentDeliveryEffect) bool {
	return settlement.CompletionTarget != nil && row.Lifecycle != models.ErrandLifecycleCancelled && !settlement.Interrupted && settlement.Outcome == models.ErrandOutcomeDone && strings.HasSuffix(effect.Outcome, ":done")
}

func validateCompletionTarget(row *models.Errand, target models.ErrandTeardownTarget, effect *models.AgentDeliveryEffect) error {
	if target.TaskID != row.TaskID || target.SessionID != row.ID || target.SessionIncarnation != effect.IncarnationID || target.ExecutionID == "" || target.TurnID != effect.TurnID || (target.SubmissionID != "" && target.SubmissionID != effect.SubmissionID) {
		return models.ErrInvalidErrand
	}
	if row.LatestSettledTurnID != effect.TurnID {
		return models.ErrErrandVersionConflict
	}
	return nil
}

func reuseCompletionIntent(current *models.ErrandTeardownIntent, target models.ErrandTeardownTarget) (bool, error) {
	if current == nil {
		return false, nil
	}
	if current.Purpose == models.ErrandTeardownPurposeCompletion && current.Target == target {
		return true, nil
	}
	switch current.State {
	case models.ErrandTeardownIntentPending:
		if !newerCompletionOnSameExecution(current, target) {
			return false, models.ErrErrandVersionConflict
		}
	case models.ErrandTeardownIntentClosed:
		return false, models.ErrErrandVersionConflict
	}
	return current.Purpose == models.ErrandTeardownPurposeCompletion && current.Target.TurnID == target.TurnID, nil
}

func newerCompletionOnSameExecution(current *models.ErrandTeardownIntent, target models.ErrandTeardownTarget) bool {
	return current.Purpose == models.ErrandTeardownPurposeCompletion && current.Target.SessionIncarnation == target.SessionIncarnation && current.Target.ExecutionID == target.ExecutionID && current.Target.TurnID != target.TurnID
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
