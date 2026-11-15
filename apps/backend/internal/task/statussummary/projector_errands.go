package statussummary

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/events"
)

// ErrandsLoader reads the bounded errand projection for one task from its
// errand rows. It must aggregate in the store, never return the rows.
type ErrandsLoader func(context.Context, string) (ErrandsSummary, error)

// isErrandRefreshEvent names the sources that can change the errand
// projection: an errand row or its projected status, a parent session's
// waiting state, and an errand session's removal. The set is closed.
func isErrandRefreshEvent(eventType string) bool {
	switch eventType {
	case events.ErrandUpdated, events.TaskSessionStateChanged, events.SessionRemoved:
		return true
	default:
		return false
	}
}

// refreshErrandsLocked re-reads the errand projection for a source that can
// change it and reports whether the projected value moved.
func (p *Projector) refreshErrandsLocked(
	ctx context.Context,
	taskID string,
	state *projectionState,
	eventType string,
) (bool, error) {
	if p.loadErrands == nil || !isErrandRefreshEvent(eventType) {
		return false, nil
	}
	next, err := p.loadErrands(ctx, taskID)
	if err != nil {
		return false, fmt.Errorf("load errands for task status summary %q: %w", taskID, err)
	}
	normalized := normalizeErrands(next)
	// Compare against the stored row too: a restored in-memory value already
	// matches the authority, but the row may still hold the pre-restart value.
	var stored *ErrandsSummary
	if state.current != nil {
		stored = state.current.Errands
	}
	changed := !equalErrands(state.errands, normalized) || !equalErrands(stored, normalized)
	state.errands = normalized
	return changed, nil
}

// restoreErrands rehydrates the errand projection after a restart or a lost
// compare-and-set. Without a loader the stored value remains the baseline.
func (p *Projector) restoreErrands(ctx context.Context, taskID string, state *projectionState) error {
	if p.loadErrands == nil {
		return nil
	}
	next, err := p.loadErrands(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load errands for task status summary %q: %w", taskID, err)
	}
	state.errands = normalizeErrands(next)
	return nil
}

// normalizeErrands returns nil when nothing is live and nobody waits, so the
// field is omitted from the task row.
func normalizeErrands(value ErrandsSummary) *ErrandsSummary {
	if value.Live <= 0 && !value.Waiting {
		return nil
	}
	if value.Live < 0 {
		value.Live = 0
	}
	return &value
}

func cloneErrands(value *ErrandsSummary) *ErrandsSummary {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func equalErrands(left, right *ErrandsSummary) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}
