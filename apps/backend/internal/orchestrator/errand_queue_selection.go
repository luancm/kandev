package orchestrator

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/google/uuid"
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
func (s *Service) errandQueueSourceSelection(ctx context.Context, source *messagequeue.QueuedMessage) messagequeue.DispatchSourceDecision {
	rejected := messagequeue.DispatchSourceDecision{}
	if !queuedErrandServerSource(source) {
		return messagequeue.DispatchSourceDecision{Allowed: true}
	}
	if s.errandsEnabled != nil && !s.errandsEnabled() {
		return rejected
	}
	part, receipt, err := s.sendNowSource(ctx, source, 0)
	if err != nil {
		return rejected
	}
	if part.Kind != errands.SendNowSourcePrepared {
		return messagequeue.DispatchSourceDecision{Allowed: part.Kind == errands.SendNowSourceNotification}
	}
	return s.preparedErrandQueueSelection(ctx, source, part, receipt)
}

func (s *Service) preparedErrandQueueSelection(ctx context.Context, source *messagequeue.QueuedMessage, part errands.SendNowDeliverySource, receipt *errands.SendNowDeliveryReceipt) messagequeue.DispatchSourceDecision {
	rejected := messagequeue.DispatchSourceDecision{}
	identity, err := s.parentQueueIdentity(ctx, source.SessionID)
	if err != nil || identity.TaskID != source.TaskID {
		return rejected
	}
	stamp, present, err := decodeSendNowErrandSource(source.Metadata)
	if err != nil {
		return rejected
	}
	legacy := !present || stamp.Identity == nil
	if legacy {
		// Only a still-retained row can prove missing legacy revision/target data.
		stamp, err = s.capturePreparedErrandSource(ctx, source.TaskID, source.SessionID, source.Content, source.Metadata)
	}
	if err != nil || stamp.Identity == nil || *stamp.Identity != identity {
		return rejected
	}
	if receipt == nil || !preparedQueueEnvelopeMatches(source, receipt) || !reflect.DeepEqual(stamp.Receipt.Claims, receipt.Claims) {
		return rejected
	}
	_, _, err = errands.DecodeSendNowDelivery(map[string]interface{}{errands.SendNowDeliveryMetadataKey: errands.SendNowDelivery{
		Version: 1, TaskID: identity.TaskID, SessionID: identity.SessionID, SessionIncarnationID: identity.SessionIncarnationID,
		SourceIDs: []string{source.ID}, Sources: []errands.SendNowDeliverySource{part},
		Receipts: []errands.SendNowDeliveryReceipt{*receipt}, EffectivePrompt: part.PreparedBlock,
	}})
	if err != nil {
		return rejected
	}
	decision := messagequeue.DispatchSourceDecision{Allowed: true}
	if legacy {
		generation, err := s.messageQueue.LifecycleGeneration(ctx, source.TaskID)
		if err != nil {
			return rejected
		}
		stamp.QueueGeneration = &generation
		stamp.Receipt.SourceID = source.ID
		decision.Metadata = mergeMetadata(source.Metadata, map[string]interface{}{errandSourceMetadataKey: stamp})
	}
	return decision
}

func preparedQueueEnvelopeMatches(source *messagequeue.QueuedMessage, receipt *errands.SendNowDeliveryReceipt) bool {
	envelope, ok := decodeErrandDeliveryEnvelope(source.Metadata[errandDeliveryKey])
	if !ok || !reflect.DeepEqual(envelope, receipt.Envelope) {
		return false
	}
	if token, present := source.Metadata[ErrandDeliveryTokenMetadataKey]; present && token != envelope.Token {
		return false
	}
	return true
}

func preparedErrandQueueKey(identity messagequeue.QueueSessionIdentity, stamp sendNowErrandSource) string {
	parts := []string{identity.TaskID, identity.SessionID, identity.SessionIncarnationID}
	for _, claim := range stamp.Receipt.Claims {
		parts = append(parts, claim.ErrandID, claim.ResultRevision)
	}
	encoded, _ := json.Marshal(parts)
	return "errand-notice:" + uuid.NewSHA1(uuid.NameSpaceOID, encoded).String()
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
	_, legacyDelivery := claim.Dispatch.Metadata[errandDeliveryKey]
	return legacyDelivery || (len(claim.Sources) == 0 && queuedErrandServerSource(&claim.Dispatch))
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
	stamp, present, err := decodeSendNowErrandSource(entry.Metadata)
	return err == nil && (!present || stamp.Kind == errands.SendNowSourceNotification)
}

// A definite refusal can normalize proved legacy metadata while transferring
// the whole original claim back to the queue. Dispatch never edits Sources.
func restoredPreparedSourceMetadata(claim *messagequeue.SendNowClaim) (map[string]map[string]interface{}, error) {
	if claim == nil {
		return nil, nil
	}
	snapshot, present, err := errands.DecodeSendNowDelivery(claim.Dispatch.Metadata)
	if err != nil || !present {
		return nil, err
	}
	if err := validateRestoredDeliveryIdentity(claim, snapshot); err != nil {
		return nil, err
	}
	updates := make(map[string]map[string]interface{})
	for i, part := range snapshot.Sources {
		if part.Kind != errands.SendNowSourcePrepared {
			continue
		}
		receipt := preparedReceiptForSource(snapshot, part.ID)
		stamp, normalize, err := restoredPreparedStamp(&claim.Sources[i], claim.Identity, part, receipt)
		if err != nil {
			return nil, err
		}
		if normalize {
			updates[part.ID] = map[string]interface{}{errandSourceMetadataKey: stamp}
		}
	}
	return updates, nil
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

func preparedReceiptForSource(snapshot errands.SendNowDelivery, id string) *errands.SendNowDeliveryReceipt {
	for i := range snapshot.Receipts {
		if snapshot.Receipts[i].SourceID == id {
			return &snapshot.Receipts[i]
		}
	}
	return nil
}

func restoredPreparedStamp(source *messagequeue.QueuedMessage, identity messagequeue.QueueSessionIdentity, part errands.SendNowDeliverySource, receipt *errands.SendNowDeliveryReceipt) (sendNowErrandSource, bool, error) {
	if source.QueuedBy != messagequeue.QueuedByServer || part.PreparedBlock != source.Content || receipt == nil || !preparedQueueEnvelopeMatches(source, receipt) {
		return sendNowErrandSource{}, false, errands.ErrInvalidSendNowDelivery
	}
	stamp, present, err := decodeSendNowErrandSource(source.Metadata)
	if err != nil {
		return stamp, false, err
	}
	if present {
		if !preparedStampMatchesReceipt(stamp, source, receipt) {
			return stamp, false, errands.ErrInvalidSendNowDelivery
		}
		if stamp.Identity != nil {
			if *stamp.Identity != identity {
				return stamp, false, errands.ErrInvalidSendNowDelivery
			}
			return stamp, false, nil
		}
	}
	copyReceipt := *receipt
	return sendNowErrandSource{QueueGeneration: &part.QueueGeneration, Identity: &identity, Provenance: errands.SendNowErrandProvenance,
		Kind: errands.SendNowSourcePrepared, PreparedBlock: part.PreparedBlock, Receipt: &copyReceipt}, true, nil
}

func preparedStampMatchesReceipt(stamp sendNowErrandSource, source *messagequeue.QueuedMessage, receipt *errands.SendNowDeliveryReceipt) bool {
	return stamp.Kind == errands.SendNowSourcePrepared && stamp.PreparedBlock == source.Content && stamp.Receipt != nil &&
		reflect.DeepEqual(stamp.Receipt.Claims, receipt.Claims) && reflect.DeepEqual(stamp.Receipt.Envelope, receipt.Envelope) &&
		(stamp.Receipt.SourceID == "" || stamp.Receipt.SourceID == source.ID)
}
