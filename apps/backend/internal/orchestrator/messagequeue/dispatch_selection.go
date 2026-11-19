package messagequeue

import (
	"context"
	"encoding/json"
)

type dispatchSourcePolicy struct {
	selectSource func(context.Context, *QueuedMessage) bool
}

type dispatchSelectionKey struct{}

type selectedDispatchSource struct {
	snapshot string
	allowed  bool
}

// SetDispatchSourcePolicy binds the trusted producer's source policy. It runs
// outside SQL before the reservation begins.
func (s *Service) SetDispatchSourcePolicy(selectSource func(context.Context, *QueuedMessage) bool) {
	if selectSource == nil {
		s.dispatchSourcePolicy.Store(nil)
		return
	}
	s.dispatchSourcePolicy.Store(&dispatchSourcePolicy{selectSource: selectSource})
}

// SelectDispatchSources preserves original FIFO snapshots for exact Send Now
// claims.
func (s *Service) SelectDispatchSources(ctx context.Context, entries []QueuedMessage) []QueuedMessage {
	policy := s.dispatchSourcePolicy.Load()
	if policy == nil {
		return entries
	}
	selected := make([]QueuedMessage, 0, len(entries))
	for i := range entries {
		if policy.selectSource(ctx, cloneQueuedMessage(&entries[i])) {
			selected = append(selected, entries[i])
		}
	}
	return selected
}

func (s *Service) dispatchSelectionContext(ctx context.Context, sessionID string) (context.Context, error) {
	policy := s.dispatchSourcePolicy.Load()
	if policy == nil {
		return ctx, nil
	}
	entries, err := s.repo.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	selection := make(map[string]selectedDispatchSource, len(entries))
	for i := range entries {
		entry := &entries[i]
		encoded, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		selection[entry.ID] = selectedDispatchSource{snapshot: string(encoded), allowed: policy.selectSource(ctx, cloneQueuedMessage(entry))}
	}
	return context.WithValue(ctx, dispatchSelectionKey{}, selection), nil
}

// A changed or newly inserted source fences this attempt. Advancing past a
// stale ordinary snapshot would reorder user input rather than preserve FIFO.
func dispatchSourceSelected(ctx context.Context, entry *QueuedMessage) (selected, current bool) {
	selection, present := ctx.Value(dispatchSelectionKey{}).(map[string]selectedDispatchSource)
	if !present {
		return true, true
	}
	expected, found := selection[entry.ID]
	encoded, err := json.Marshal(entry)
	if !found || err != nil || expected.snapshot != string(encoded) {
		return false, false
	}
	return expected.allowed, true
}

func dispatchHeadIndex(ctx context.Context, entries []*QueuedMessage) int {
	index := -1
	for i, entry := range entries {
		selected, current := dispatchSourceSelected(ctx, entry)
		if current && !selected {
			continue
		}
		if index < 0 || entry.Position < entries[index].Position {
			index = i
		}
	}
	if index < 0 {
		return -1
	}
	if _, current := dispatchSourceSelected(ctx, entries[index]); !current {
		return -1
	}
	return index
}
