package errands

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

// OnSessionStateChanged publishes the current projection and wakes joins after authoritative session persistence.
func (s *Service) OnSessionStateChanged(ctx context.Context, sessionID string) error {
	if !s.featureEnabled() {
		return nil
	}
	row, err := s.store.GetErrand(ctx, sessionID)
	if errors.Is(err, models.ErrErrandNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	s.publish(ctx, row)
	s.wakeAwaits(row.ParentSessionID)
	return nil
}
