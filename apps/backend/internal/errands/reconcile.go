package errands

import (
	"context"
	"fmt"
	"sort"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/sessionstate"
	"github.com/kandev/kandev/internal/task/models"
)

// interruptedNotice is the result of a parent-initiated turn that restart or
// stall recovery abandoned before it settled.
const interruptedNotice = "The errand was interrupted before it finished."

const (
	reconcileActionCompletedTurn    = "completed_parent_turn"
	reconcileActionInterruptedTurn  = "interrupted_parent_turn"
	reconcileActionFailedLaunch     = "failed_launched_row"
	reconcileActionDiscardedResult  = "terminal_parent_result_discarded"
	reconcileActionReleasedDelivery = "orphaned_delivery_claim_released"
)

// Reconcile repairs errand state after a restart, once session recovery has
// settled session states. It runs whether or not the feature flag is on, so
// existing errands stay governed. Each step is independent: a failed row is
// logged and skipped, and the next boot retries it.
//
// Order: launched rows first, then claims whose delivery token reached no
// parent message, then pending results of terminal parents, then delivery for
// every parent still holding a pending result.
func (s *Service) Reconcile(ctx context.Context) error {
	rows, err := s.store.ListErrands(ctx)
	if err != nil {
		return fmt.Errorf("list errands for reconciliation: %w", err)
	}
	for _, row := range rows {
		if err := s.reconcileLaunched(ctx, row); err != nil {
			fields := s.errandFields(row)
			fields = append(fields, zap.Error(err))
			s.logger.Warn("errand reconciliation skipped a launched row", fields...)
		}
	}
	if err := s.releaseUndeliveredClaims(ctx); err != nil {
		return err
	}
	if err := s.discardTerminalParentResults(ctx); err != nil {
		return err
	}
	return s.deliverForPendingParents(ctx)
}

func (s *Service) reconcileLaunched(ctx context.Context, row *models.Errand) error {
	if row.Lifecycle != models.ErrandLifecycleLaunched {
		return nil
	}
	session, err := s.store.GetTaskSession(ctx, row.ID)
	if err != nil && !isMissingSession(err) {
		return fmt.Errorf("load errand session: %w", err)
	}
	if (session == nil || session.State == models.TaskSessionStateFailed) && !resultAlreadyCaptured(row.ResultState) {
		return s.failLaunchedRow(ctx, row)
	}
	if session == nil || sessionstate.IsWorking(session.State) {
		return nil
	}
	return s.settleAbandonedOrCompleted(ctx, row)
}

// settleAbandonedOrCompleted captures a completed, uncaptured parent-initiated
// turn, and marks an abandoned one interrupted.
func (s *Service) settleAbandonedOrCompleted(ctx context.Context, row *models.Errand) error {
	if s.turns == nil {
		return nil
	}
	turnID, view, err := s.turns.LatestTurn(ctx, row.ID)
	if err != nil {
		return fmt.Errorf("read latest errand turn: %w", err)
	}
	if turnID == "" || (row.ResultTurnID == turnID && resultAlreadyCaptured(row.ResultState)) {
		return nil
	}
	if !parentInitiatedTurn(row, turnID, view) {
		return nil
	}
	if view.Completed {
		return s.onErrandTurnSettled(ctx, TurnSettlement{
			SessionID: row.ID, TurnID: turnID, Outcome: models.ErrandOutcomeDone,
		}, reconcileActionCompletedTurn)
	}
	if !row.AwaitingResult {
		return nil
	}
	return s.writeReconciled(ctx, row, reconcileActionInterruptedTurn, func(e *models.Errand) bool {
		e.LastOutcome = models.ErrandOutcomeFailed
		e.AwaitingResult = false
		writePendingResult(e, interruptedNotice, false, turnID, models.ErrandResultInterrupted, s.clock.Now())
		return true
	})
}

func (s *Service) failLaunchedRow(ctx context.Context, row *models.Errand) error {
	return s.writeReconciled(ctx, row, reconcileActionFailedLaunch, func(e *models.Errand) bool {
		e.LastOutcome = models.ErrandOutcomeFailed
		e.AwaitingResult = false
		writePendingResult(e, failureNotice, false, e.ResultTurnID, models.ErrandResultFailure, s.clock.Now())
		return true
	})
}

// writeReconciled applies one version-guarded change under the parent's mutex,
// then publishes and wakes waiters, in that order.
func (s *Service) writeReconciled(
	ctx context.Context, row *models.Errand, action string, change func(*models.Errand) bool,
) error {
	unlock := s.parentLocks.lock(row.ParentSessionID)
	updated, changed, err := s.update(ctx, row.ID, change)
	unlock()
	if err != nil {
		return fmt.Errorf("write reconciled errand: %w", err)
	}
	if changed {
		s.logReconciled(updated, action)
		s.afterResultWrite(ctx, updated)
	}
	return nil
}

func (s *Service) logReconciled(errand *models.Errand, action string) {
	s.logger.Info("errand.reconciled", s.errandFields(errand,
		"action", action, "result_state", string(errand.ResultState))...)
}

// releaseUndeliveredClaims returns claims whose token is absent from the
// persisted parent transcript. Direct prompts attach the snapshot to their
// existing user message before provider admission, and queued prompts persist
// it with their transcript message before dispatch.
func (s *Service) releaseUndeliveredClaims(ctx context.Context) error {
	if s.receipts == nil {
		return nil
	}
	rows, err := s.store.ListErrands(ctx)
	if err != nil {
		return fmt.Errorf("list errands for claim reconciliation: %w", err)
	}
	orphaned := map[string]struct{}{}
	candidates := map[string][]*models.Errand{}
	for _, row := range rows {
		if row.ResultState != models.ErrandResultDelivered || row.DeliveryToken == "" ||
			(row.DeliveredRoute != models.ErrandDeliveryNotification && row.DeliveredRoute != models.ErrandDeliveryTurnContext) {
			continue
		}
		found, err := s.receipts.HasDeliveryToken(ctx, row.ParentSessionID, row.DeliveryToken)
		if err != nil {
			return fmt.Errorf("check delivery token: %w", err)
		}
		if !found {
			orphaned[row.DeliveryToken] = struct{}{}
			candidates[row.DeliveryToken] = append(candidates[row.DeliveryToken], row)
		}
	}
	for _, token := range sortedTokens(orphaned) {
		if err := s.releaseClaim(ctx, token); err != nil {
			return err
		}
		for _, candidate := range candidates[token] {
			s.logReleasedDelivery(ctx, candidate)
		}
	}
	return nil
}

func (s *Service) logReleasedDelivery(ctx context.Context, candidate *models.Errand) {
	updated, err := s.store.GetErrand(ctx, candidate.ID)
	if err != nil {
		fields := s.errandFields(candidate)
		fields = append(fields, zap.Error(err))
		s.logger.Warn("errand reconciliation could not verify released delivery", fields...)
		return
	}
	if updated.Version <= candidate.Version || updated.ResultState != models.ErrandResultPending || updated.DeliveryToken != "" {
		return
	}
	s.logReconciled(updated, reconcileActionReleasedDelivery)
}

// discardTerminalParentResults marks pending results of a parent that can no
// longer receive them as discarded.
func (s *Service) discardTerminalParentResults(ctx context.Context) error {
	rows, err := s.store.ListErrands(ctx)
	if err != nil {
		return fmt.Errorf("list errands for terminal parents: %w", err)
	}
	for _, row := range rows {
		if row.ResultState != models.ErrandResultPending {
			continue
		}
		parent, err := s.store.GetTaskSession(ctx, row.ParentSessionID)
		if err != nil && !isMissingSession(err) {
			return fmt.Errorf("load parent session: %w", err)
		}
		if parent != nil && !isTerminalParent(parent.State) {
			continue
		}
		if err := s.writeReconciled(ctx, row, reconcileActionDiscardedResult, discardResult); err != nil {
			return err
		}
	}
	return nil
}

func discardResult(e *models.Errand) bool {
	if e.ResultState != models.ErrandResultPending {
		return false
	}
	e.ResultState = models.ErrandResultDiscarded
	return true
}

func isTerminalParent(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateCompleted ||
		state == models.TaskSessionStateFailed ||
		state == models.TaskSessionStateCancelled
}

// deliverForPendingParents runs deliverPending for each parent that still holds
// a pending result. deliverPending itself declines while a turn runs.
func (s *Service) deliverForPendingParents(ctx context.Context) error {
	rows, err := s.store.ListErrands(ctx)
	if err != nil {
		return fmt.Errorf("list errands for delivery: %w", err)
	}
	parents := map[string]struct{}{}
	for _, row := range rows {
		if row.ResultState == models.ErrandResultPending {
			parents[row.ParentSessionID] = struct{}{}
		}
	}
	for _, parentID := range sortedKeys(parents) {
		s.deliverPending(ctx, parentID)
	}
	return nil
}

func sortedTokens(set map[string]struct{}) []string {
	tokens := make([]string, 0, len(set))
	for token := range set {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return tokens
}
