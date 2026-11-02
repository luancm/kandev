package messagequeue

import (
	"context"
	"encoding/json"
)

// DispatchSourceDecision keeps bypassed sources pending. Metadata optionally
// carries a complete, proved normalization; input and source IDs stay intact.
type DispatchSourceDecision struct {
	Allowed  bool
	Metadata map[string]interface{}
}

type dispatchSourcePolicy struct {
	selectSource func(context.Context, *QueuedMessage) DispatchSourceDecision
}

type dispatchSelectionKey struct{}

type selectedDispatchSource struct {
	snapshot string
	decision DispatchSourceDecision
}

// SetDispatchSourcePolicy binds the trusted producer's source policy. It runs
// outside SQL so legacy provenance can be read before the reservation begins.
func (s *Service) SetDispatchSourcePolicy(selectSource func(context.Context, *QueuedMessage) DispatchSourceDecision) {
	if selectSource == nil {
		s.dispatchSourcePolicy.Store(nil)
		return
	}
	s.dispatchSourcePolicy.Store(&dispatchSourcePolicy{selectSource: selectSource})
}

// SelectDispatchSources preserves original FIFO snapshots for exact Send Now
// claims. Any proved metadata normalization is persisted with that claim.
func (s *Service) SelectDispatchSources(ctx context.Context, entries []QueuedMessage) []QueuedMessage {
	policy := s.dispatchSourcePolicy.Load()
	if policy == nil {
		return entries
	}
	selected := make([]QueuedMessage, 0, len(entries))
	for i := range entries {
		if policy.selectSource(ctx, cloneQueuedMessage(&entries[i])).Allowed {
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
		selection[entry.ID] = selectedDispatchSource{snapshot: string(encoded), decision: policy.selectSource(ctx, cloneQueuedMessage(entry))}
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
	return expected.decision.Allowed, true
}

func applyDispatchSourceMetadata(ctx context.Context, entry *QueuedMessage) {
	selection, _ := ctx.Value(dispatchSelectionKey{}).(map[string]selectedDispatchSource)
	if metadata := selection[entry.ID].decision.Metadata; metadata != nil {
		entry.Metadata = copyMessageMetadata(metadata, 0)
	}
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
