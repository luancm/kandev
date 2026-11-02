package messagequeue

import (
	"context"
	"encoding/json"
)

type restoredSourceMetadataKey struct{}

type restoredSourceMetadata struct {
	claimID    string
	identity   QueueSessionIdentity
	operation  int64
	generation int64
	snapshots  map[string]string
	updates    map[string]map[string]interface{}
}

// WithRestoredSourceMetadata binds proved metadata additions to the exact
// original claim. Repositories apply them only after validating full restore
// authority; original content, mode, attachments and source IDs are immutable.
func WithRestoredSourceMetadata(ctx context.Context, claim *SendNowClaim, updates map[string]map[string]interface{}) (context.Context, error) {
	if len(updates) == 0 {
		return ctx, nil
	}
	if claim == nil {
		return nil, ErrSendNowClaimChanged
	}
	captured := restoredSourceMetadata{claimID: claim.ClaimID, identity: claim.Identity,
		operation: claim.OperationGeneration, generation: claim.SessionGeneration, snapshots: make(map[string]string), updates: make(map[string]map[string]interface{})}
	for i := range claim.Sources {
		source := &claim.Sources[i]
		encoded, err := json.Marshal(source)
		if err != nil {
			return nil, err
		}
		captured.snapshots[source.ID] = string(encoded)
		captured.updates[source.ID] = copyMessageMetadata(updates[source.ID], 0)
	}
	for id := range updates {
		if _, exists := captured.snapshots[id]; !exists {
			return nil, ErrSendNowClaimChanged
		}
	}
	return context.WithValue(ctx, restoredSourceMetadataKey{}, captured), nil
}

func normalizedRestoredSendNowSources(ctx context.Context, claim *SendNowClaim) ([]QueuedMessage, error) {
	captured, present := ctx.Value(restoredSourceMetadataKey{}).(restoredSourceMetadata)
	if !present {
		return claim.Sources, nil
	}
	if captured.claimID != claim.ClaimID || captured.identity != claim.Identity || captured.operation != claim.OperationGeneration || captured.generation != claim.SessionGeneration {
		return nil, ErrSendNowClaimChanged
	}
	sources := make([]QueuedMessage, 0, len(claim.Sources))
	for _, source := range claim.Sources {
		encoded, err := json.Marshal(source)
		if err != nil || captured.snapshots[source.ID] != string(encoded) {
			return nil, ErrSendNowClaimChanged
		}
		source.Metadata = restoredSendNowMetadata(ctx, source.ID, source.Metadata)
		sources = append(sources, source)
	}
	return sources, nil
}

func restoredSendNowMetadata(ctx context.Context, id string, metadata map[string]interface{}) map[string]interface{} {
	captured, _ := ctx.Value(restoredSourceMetadataKey{}).(restoredSourceMetadata)
	updates := captured.updates[id]
	if len(updates) == 0 {
		return metadata
	}
	normalized := copyMessageMetadata(metadata, len(updates))
	for key, value := range updates {
		normalized[key] = value
	}
	return normalized
}
