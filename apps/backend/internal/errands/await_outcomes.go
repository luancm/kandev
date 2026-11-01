package errands

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) OfferAwait(ctx context.Context, id models.ErrandAwaitIdentity, delivery models.ErrandAwaitDelivery) error {
	if delivery.Version != 1 || delivery.Token != id.Token {
		return models.ErrInvalidErrand
	}
	return s.store.OfferErrandAwait(ctx, id, delivery.Items)
}
func (s *Service) AbortOwnedAwait(ctx context.Context, id models.ErrandAwaitIdentity) error {
	changed, err := s.store.AbortErrandAwait(ctx, id)
	if err != nil {
		return err
	}
	for _, row := range changed {
		s.publish(ctx, row)
	}
	s.wakeAwaits(id.ParentSessionID)
	s.deliverPending(ctx, id.ParentSessionID)
	return nil
}
