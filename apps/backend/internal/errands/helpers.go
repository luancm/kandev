package errands

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/orchestrator/sessionstate"
	"github.com/kandev/kandev/internal/task/models"
)

func isMissingSession(err error) bool {
	return errors.Is(err, models.ErrTaskSessionNotFound)
}

func isWorking(session *models.TaskSession) bool {
	return sessionstate.IsWorking(session.State)
}

// maxUpdateAttempts bounds compare-and-set retries of one errand mutation.
const maxUpdateAttempts = 5

// update applies change to a fresh copy of the errand and writes it with the
// version guard, retrying when another writer won. change returns false to
// leave the row untouched. The returned errand is the stored row after the
// call and changed reports whether this call wrote it.
func (s *Service) update(
	ctx context.Context, id string, change func(*models.Errand) bool,
) (*models.Errand, bool, error) {
	for attempt := 0; attempt < maxUpdateAttempts; attempt++ {
		current, err := s.store.GetErrand(ctx, id)
		if err != nil {
			return nil, false, err
		}
		next := *current
		if !change(&next) {
			return current, false, nil
		}
		err = s.store.UpdateErrand(ctx, &next)
		if err == nil {
			return &next, true, nil
		}
		if !errors.Is(err, models.ErrErrandVersionConflict) {
			return nil, false, err
		}
	}
	return nil, false, fmt.Errorf("%w: %s", models.ErrErrandVersionConflict, id)
}

// oneLine collapses whitespace and cuts text to at most limit bytes at a
// character boundary.
func oneLine(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}
