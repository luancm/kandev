package orchestrator

import (
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

func preserveSendNowErrandMetadata(claim *messagequeue.SendNowClaim) bool {
	if claim == nil {
		return false
	}
	if claim.Dispatch.Metadata == nil {
		claim.Dispatch.Metadata = make(map[string]interface{})
	}
	notification := sendNowClaimHasErrandNotification(claim)
	if notification {
		claim.Dispatch.Metadata[errandNotificationKey] = true
	}
	envelope, hasEnvelope := decodeErrandDeliveryEnvelope(claim.Dispatch.Metadata[errandDeliveryKey])
	for i := range claim.Sources {
		source := &claim.Sources[i]
		envelope, hasEnvelope = mergeSendNowErrandEnvelope(envelope, hasEnvelope, source.Metadata)
		if _, exists := claim.Dispatch.Metadata[ErrandDeliverySubmissionMetadataKey]; !exists {
			if submissionID, ok := source.Metadata[ErrandDeliverySubmissionMetadataKey]; ok {
				claim.Dispatch.Metadata[ErrandDeliverySubmissionMetadataKey] = submissionID
			}
		}
	}
	if hasEnvelope {
		claim.Dispatch.Metadata[errandDeliveryKey] = envelope
		if envelope.Token != "" {
			claim.Dispatch.Metadata[ErrandDeliveryTokenMetadataKey] = envelope.Token
		}
	}
	return notification
}

func mergeSendNowErrandEnvelope(
	envelope errands.DeliveryEnvelope,
	hasEnvelope bool,
	sourceMetadata map[string]interface{},
) (errands.DeliveryEnvelope, bool) {
	sourceEnvelope, ok := decodeErrandDeliveryEnvelope(sourceMetadata[errandDeliveryKey])
	if !ok {
		return envelope, hasEnvelope
	}
	if !hasEnvelope {
		return sourceEnvelope, true
	}
	envelope.Items = appendErrandDeliveryItems(envelope.Items, sourceEnvelope.Items)
	if envelope.Token == "" {
		envelope.Token = sourceEnvelope.Token
	}
	if envelope.Route == "" {
		envelope.Route = sourceEnvelope.Route
	}
	return envelope, true
}

func sendNowClaimHasErrandNotification(claim *messagequeue.SendNowClaim) bool {
	if claim == nil {
		return false
	}
	if isErrandNotificationEntry(&claim.Dispatch) {
		return true
	}
	for i := range claim.Sources {
		if isErrandNotificationEntry(&claim.Sources[i]) {
			return true
		}
	}
	return false
}

func (s *Service) sendNowHasErrandContext(claim *messagequeue.SendNowClaim) bool {
	if claim == nil {
		return false
	}
	if sendNowClaimHasErrandNotification(claim) || hasQueuedErrandDelivery(&claim.Dispatch) {
		return true
	}
	for i := range claim.Sources {
		if hasQueuedErrandDelivery(&claim.Sources[i]) {
			return true
		}
	}
	_, configured := s.errandComposer()
	return configured
}
