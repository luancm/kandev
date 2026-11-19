package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

func queuedErrandServerSource(source *messagequeue.QueuedMessage) bool {
	if source == nil || source.QueuedBy != messagequeue.QueuedByServer {
		return false
	}
	for _, key := range []string{errandSourceMetadataKey, errandDeliveryKey, ErrandDeliveryTokenMetadataKey, ErrandDeliverySubmissionMetadataKey} {
		if _, present := source.Metadata[key]; present {
			return true
		}
	}
	return isErrandNotificationEntry(source)
}

// The canonical restart-required callback gates source selection, independently
// of live composition. Disabled sources remain untouched and consume capacity.
func (s *Service) errandQueueSourceSelection(_ context.Context, source *messagequeue.QueuedMessage) bool {
	if !queuedErrandServerSource(source) {
		return true
	}
	enabled := s.errandsEnabled == nil || s.errandsEnabled()
	return enabled && isErrandNotificationPlaceholder(source)
}

func (s *Service) sendNowClaimOwnsErrandSources(claim *messagequeue.SendNowClaim) bool {
	for i := range claim.Sources {
		if queuedErrandServerSource(&claim.Sources[i]) {
			return true
		}
	}
	snapshot, present, err := errands.DecodeSendNowDelivery(claim.Dispatch.Metadata)
	if present {
		return err != nil || len(snapshot.Receipts) > 0
	}
	_, hasEnvelope := claim.Dispatch.Metadata[errandDeliveryKey]
	return hasEnvelope || (len(claim.Sources) == 0 && queuedErrandServerSource(&claim.Dispatch))
}

func isErrandNotificationPlaceholder(entry *messagequeue.QueuedMessage) bool {
	if !isErrandNotificationEntry(entry) || sendNowSourceHasInput(entry) {
		return false
	}
	for _, key := range []string{errandDeliveryKey, ErrandDeliveryTokenMetadataKey, ErrandDeliverySubmissionMetadataKey} {
		if _, present := entry.Metadata[key]; present {
			return false
		}
	}
	return hasErrandNotificationStamp(entry.Metadata)
}

func validateRestoredErrandDelivery(claim *messagequeue.SendNowClaim) error {
	if claim == nil {
		return nil
	}
	snapshot, present, err := errands.DecodeSendNowDelivery(claim.Dispatch.Metadata)
	if err != nil || !present {
		return err
	}
	return validateRestoredDeliveryIdentity(claim, snapshot)
}

func validateRestoredDeliveryIdentity(claim *messagequeue.SendNowClaim, snapshot errands.SendNowDelivery) error {
	if snapshot.TaskID != claim.Identity.TaskID || snapshot.SessionID != claim.Identity.SessionID ||
		snapshot.SessionIncarnationID != claim.Identity.SessionIncarnationID || snapshot.OperationGeneration != claim.OperationGeneration ||
		snapshot.QueueGeneration != claim.SessionGeneration || len(snapshot.Sources) != len(claim.Sources) {
		return errands.ErrInvalidSendNowDelivery
	}
	for i, source := range claim.Sources {
		if !restoredSourceMatchesSnapshot(claim, source, snapshot.Sources[i]) {
			return errands.ErrInvalidSendNowDelivery
		}
	}
	return nil
}

func restoredSourceMatchesSnapshot(claim *messagequeue.SendNowClaim, source messagequeue.QueuedMessage, part errands.SendNowDeliverySource) bool {
	generation, present := claim.SourceGenerations[source.TaskID]
	return present && part.ID == source.ID && part.QueuedBy == source.QueuedBy && part.QueueGeneration == generation &&
		source.TaskID == claim.Identity.TaskID && source.SessionID == claim.Identity.SessionID &&
		(part.Kind != errands.SendNowSourceOrdinary || part.Content == source.Content)
}
