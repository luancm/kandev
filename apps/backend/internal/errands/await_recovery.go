package errands

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

type awaitRecoveryStore interface {
	ListUnresolvedErrandAwaits(context.Context, string) ([]models.ErrandAwaitIdentity, error)
	RecoverErrandAwait(context.Context, models.ErrandAwaitIdentity) (bool, error)
}

// reconcileAwaitRecovery runs only at startup, after session recovery and
// authenticated replay. It never classifies ordinary live invocations.
func (s *Service) reconcileAwaitRecovery(ctx context.Context) error {
	store, ok := s.store.(awaitRecoveryStore)
	if !ok {
		return nil
	}
	attempts, err := store.ListUnresolvedErrandAwaits(ctx, "")
	if err != nil {
		return fmt.Errorf("list retained awaits: %w", err)
	}
	parents := map[string]struct{}{}
	for _, id := range attempts {
		if _, err := store.RecoverErrandAwait(ctx, id); err != nil {
			return fmt.Errorf("classify retained await: %w", err)
		}
		parents[id.ParentSessionID] = struct{}{}
	}
	for parent := range parents {
		s.RefreshAwaitDelivery(ctx, parent)
	}
	return nil
}

// RefreshAwaitDelivery publishes committed current claim evidence through the
// existing errand.updated surface. Reconnect/list repairs a missed publication.
// It is independent of the release flag for retained invocations.
func (s *Service) RefreshAwaitDelivery(ctx context.Context, parentID string) {
	rows, err := s.store.ListErrandsByParent(ctx, parentID)
	if err != nil {
		s.logger.Warn("errand await projection refresh failed", zap.String("parent_session_id", parentID), zap.Error(err))
		return
	}
	for _, row := range rows {
		if row.ResultState == models.ErrandResultDelivered && row.DeliveredRoute == models.ErrandDeliveryAwait {
			s.publish(ctx, row)
		}
	}
}
