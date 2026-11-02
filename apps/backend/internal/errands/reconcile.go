package errands

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

const reconcileActionReleasedDelivery = "orphaned_delivery_claim_released"

// Reconcile repairs errand state after a restart, once session recovery has
// settled session states. It runs whether or not the feature flag is on, so
// existing errands stay governed. Each step is independent: a failed row is
// logged and skipped, and the next boot retries it.
//
// Order: recover durable owner cascades before launched results, orphaned
// claims or notifications. Unreadable owners exclude only their task.
func (s *Service) Reconcile(ctx context.Context) (err error) {
	teardownErr := s.retryPendingTeardowns(ctx)
	owners, lifecycleErr := s.reconcileLifecycle(ctx)
	defer func() { err = errors.Join(teardownErr, lifecycleErr, err) }()
	if owners == nil {
		return nil
	}
	// Queue recovery shares the normal FIFO and launch guard. Adoption happens
	// before result recovery, so an admitted reservation is never replayed.
	s.drainQueued(ctx, owners)
	rows, err := s.store.ListErrands(ctx)
	if err != nil {
		return fmt.Errorf("list errands for reconciliation: %w", err)
	}
	for _, row := range rows {
		if !owners.permits(row) {
			continue
		}
		if err := s.reconcileLaunched(ctx, row); err != nil {
			fields := s.errandFields(row)
			fields = append(fields, zap.Error(err))
			s.logger.Warn("errand reconciliation skipped a launched row", fields...)
		}
	}
	if err := s.reconcileAwaitRecovery(ctx); err != nil {
		return err
	}
	if err := s.releaseUndeliveredClaims(ctx, owners); err != nil {
		return err
	}
	return s.deliverForPendingParents(ctx)
}

func (s *Service) reconcileLaunched(ctx context.Context, row *models.Errand) error {
	if row.Lifecycle != models.ErrandLifecycleLaunched {
		return nil
	}
	return s.ReconcileTerminalDecisions(ctx, row.ID)
}

func (s *Service) logReconciled(errand *models.Errand, action string) {
	s.logger.Info("errand.reconciled", s.errandFields(errand,
		"action", action, "result_state", string(errand.ResultState))...)
}

// releaseUndeliveredClaims returns claims whose token is absent from the
// persisted parent transcript. Direct prompts attach the snapshot to their
// existing user message before provider admission, and queued prompts persist
// it with their transcript message before dispatch.
func (s *Service) releaseUndeliveredClaims(ctx context.Context, owners *lifecycleInventory) error {
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
		if !owners.permits(row) || !isDeliveredParentClaim(row) {
			continue
		}
		claim := models.ErrandResultClaim{ErrandID: row.ID, ResultRevision: row.ResultRevision,
			ParentAdmissionGeneration: row.ParentAdmissionGeneration, Token: row.DeliveryToken, Route: row.DeliveredRoute}
		owned, err := s.store.HasErrandPreparedOwnership(ctx, row.TaskID, row.ParentSessionID, claim)
		if err != nil {
			return fmt.Errorf("check prepared source ownership: %w", err)
		}
		if owned {
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

func isDeliveredParentClaim(row *models.Errand) bool {
	return row.ResultState == models.ErrandResultDelivered && row.DeliveryToken != "" &&
		(row.DeliveredRoute == models.ErrandDeliveryNotification || row.DeliveredRoute == models.ErrandDeliveryTurnContext)
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
	owners := s.inspectLifecycle(ctx, rows)
	parents := map[string]struct{}{}
	for _, row := range rows {
		if owners.permits(row) && row.ResultState == models.ErrandResultPending {
			parents[row.ParentSessionID] = struct{}{}
		}
	}
	for _, parentID := range sortedKeys(parents) {
		s.deliverPending(ctx, parentID)
	}
	return owners.err
}

func sortedTokens(set map[string]struct{}) []string {
	tokens := make([]string, 0, len(set))
	for token := range set {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return tokens
}
