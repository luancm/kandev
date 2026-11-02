package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

const errandSourceMetadataKey = "errand_source"

type sendNowErrandSource struct {
	Provenance    string                          `json:"provenance"`
	Kind          errands.SendNowSourceKind       `json:"kind"`
	PreparedBlock string                          `json:"prepared_block,omitempty"`
	Receipt       *errands.SendNowDeliveryReceipt `json:"receipt,omitempty"`
}

type sendNowErrandInput struct {
	claim           *messagequeue.SendNowClaim
	snapshot        errands.SendNowDelivery
	hasOrdinary     bool
	hasNotification bool
}

type sendNowErrandInputKey struct{}

type sendNowDeliveryPromptKey struct{}

type errandClaimReader interface {
	ListErrandResultClaims(context.Context, string, string, string) ([]models.ErrandResultClaim, error)
}

func (s *Service) prepareSendNowErrandInput(ctx context.Context, claim *messagequeue.SendNowClaim) (*sendNowErrandInput, error) {
	if claim.Identity.SessionIncarnationID == "" || !s.messageQueue.PendingSendNowClaimPersistenceAvailable() {
		return nil, messagequeue.ErrSendNowClaimChanged
	}
	if _, _, err := errands.DecodeSendNowDelivery(claim.Dispatch.Metadata); err != nil {
		return nil, err
	}
	input := &sendNowErrandInput{claim: claim, snapshot: errands.SendNowDelivery{
		Version: 1, TaskID: claim.Dispatch.TaskID, SessionID: claim.Dispatch.SessionID,
		SessionIncarnationID: claim.Identity.SessionIncarnationID,
		OperationGeneration:  claim.OperationGeneration, QueueGeneration: claim.SessionGeneration,
	}}
	for i := range claim.Sources {
		source := &claim.Sources[i]
		if source.TaskID != claim.Dispatch.TaskID || source.SessionID != claim.Dispatch.SessionID {
			return nil, messagequeue.ErrSendNowClaimChanged
		}
		generation, captured := claim.SourceGenerations[source.TaskID]
		if !captured {
			return nil, messagequeue.ErrSendNowClaimChanged
		}
		part, receipt, err := s.sendNowSource(ctx, source, generation)
		if err != nil {
			return nil, err
		}
		input.snapshot.SourceIDs = append(input.snapshot.SourceIDs, source.ID)
		input.snapshot.Sources = append(input.snapshot.Sources, part)
		if receipt != nil {
			input.snapshot.Receipts = append(input.snapshot.Receipts, *receipt)
		}
		if part.Kind != errands.SendNowSourceOrdinary {
			input.hasNotification = true
		}
		if part.Kind == errands.SendNowSourceOrdinary && sendNowSourceHasInput(source) {
			if !input.hasOrdinary {
				claim.Dispatch.Model, claim.Dispatch.PlanMode = source.Model, source.PlanMode
			}
			input.hasOrdinary = true
		}
	}
	// Synthetic dispatch metadata is presentation only. Its oldest source cannot
	// supply delivery ownership for the complete selection.
	for _, key := range []string{errandNotificationKey, errandDeliveryKey, ErrandDeliveryTokenMetadataKey, ErrandDeliverySubmissionMetadataKey} {
		delete(claim.Dispatch.Metadata, key)
	}
	return input, nil
}

func sendNowSourceHasInput(source *messagequeue.QueuedMessage) bool {
	return source.Content != "" || len(source.Attachments) > 0 || source.PlanMode ||
		len(entityrefs.NormalizePersisted(source.Metadata[messagequeue.MetadataEntityReferences])) > 0 ||
		stepHandoffFromQueuedMetadata(source.Metadata) != ""
}

func (s *Service) sendNowSource(ctx context.Context, source *messagequeue.QueuedMessage, generation int64) (errands.SendNowDeliverySource, *errands.SendNowDeliveryReceipt, error) {
	part := errands.SendNowDeliverySource{ID: source.ID, QueueGeneration: generation,
		Kind: errands.SendNowSourceOrdinary, QueuedBy: source.QueuedBy, Content: source.Content}
	if source.QueuedBy != messagequeue.QueuedByServer {
		return part, nil, nil
	}
	stamp, present, err := decodeSendNowErrandSource(source.Metadata)
	if err != nil {
		return part, nil, err
	}
	if !present {
		stamp, err = s.legacySendNowErrandSource(ctx, source)
		if err != nil {
			return part, nil, err
		}
		if stamp.Kind == "" {
			return part, nil, nil
		}
	}
	if stamp.Kind == errands.SendNowSourcePrepared && source.Content != stamp.PreparedBlock {
		return part, nil, errands.ErrInvalidSendNowDelivery
	}
	if stamp.Kind == errands.SendNowSourceNotification && source.Content != "" {
		return part, nil, errands.ErrInvalidSendNowDelivery
	}
	part.Kind, part.Provenance, part.Content = stamp.Kind, stamp.Provenance, ""
	part.PreparedBlock = stamp.PreparedBlock
	if stamp.Receipt == nil {
		return part, nil, nil
	}
	receipt := *stamp.Receipt
	receipt.SourceID = source.ID
	return part, &receipt, nil
}

func (s *Service) legacySendNowErrandSource(ctx context.Context, source *messagequeue.QueuedMessage) (sendNowErrandSource, error) {
	if _, present := source.Metadata[errandDeliveryKey]; present {
		return s.capturePreparedErrandSource(ctx, source.TaskID, source.SessionID, source.Content, source.Metadata)
	}
	for _, key := range []string{ErrandDeliveryTokenMetadataKey, ErrandDeliverySubmissionMetadataKey} {
		if _, present := source.Metadata[key]; present {
			return sendNowErrandSource{}, errands.ErrInvalidSendNowDelivery
		}
	}
	if isErrandNotificationEntry(source) && !sendNowSourceHasInput(source) {
		return sendNowErrandSource{Provenance: errands.SendNowErrandProvenance, Kind: errands.SendNowSourceNotification}, nil
	}
	return sendNowErrandSource{}, nil
}

// The updater captures the exact claim; runtime admission may use a fresh context.
func (s *Service) sendNowDeliveryClaimUpdater(claim *messagequeue.SendNowClaim) deliveryClaimUpdater {
	return func(ctx context.Context, protocol, submissionID, payloadHash string) error {
		snapshot, present, err := errands.DecodeSendNowDelivery(claim.Dispatch.Metadata)
		if err != nil {
			return err
		}
		payload, canonical := ctx.Value(sendNowDeliveryPromptKey{}).(agentDeliveryPromptPayload)
		if present {
			if !canonical {
				return errors.New("send now runtime prompt is unavailable")
			}
			snapshot.EffectivePrompt = payload.Text
			claim.Dispatch.Metadata[errands.SendNowDeliveryMetadataKey] = snapshot
			if _, _, err := errands.DecodeSendNowDelivery(claim.Dispatch.Metadata); err != nil {
				return err
			}
		}
		if err := s.messageQueue.SetPendingSendNowClaimDelivery(ctx, claim, protocol, submissionID, payloadHash); err != nil {
			return err
		}
		if present {
			if err := s.recordQueuedUserMessage(ctx, &claim.Dispatch, payload.Attachments, sendNowSourceIDs(claim)...); err != nil {
				return fmt.Errorf("persist Send Now runtime prompt transcript: %w", err)
			}
		}
		return nil
	}
}

func decodeSendNowErrandSource(metadata map[string]interface{}) (sendNowErrandSource, bool, error) {
	value, present := metadata[errandSourceMetadataKey]
	if !present {
		return sendNowErrandSource{}, false, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return sendNowErrandSource{}, true, errands.ErrInvalidSendNowDelivery
	}
	var stamp sendNowErrandSource
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&stamp); err != nil || stamp.Provenance != errands.SendNowErrandProvenance {
		return sendNowErrandSource{}, true, errands.ErrInvalidSendNowDelivery
	}
	switch stamp.Kind {
	case errands.SendNowSourceNotification:
		if stamp.PreparedBlock != "" || stamp.Receipt != nil {
			return stamp, true, errands.ErrInvalidSendNowDelivery
		}
	case errands.SendNowSourcePrepared:
		if stamp.PreparedBlock == "" || stamp.Receipt == nil || !preparedSourceGenerationsPresent(encoded) {
			return stamp, true, errands.ErrInvalidSendNowDelivery
		}
	default:
		return stamp, true, errands.ErrInvalidSendNowDelivery
	}
	return stamp, true, nil
}

func (s *Service) composeSendNowErrandTurn(ctx context.Context, input *sendNowErrandInput, prompt string) (string, string, error) {
	claim := input.claim
	route := models.ErrandDeliveryTurnContext
	if input.hasNotification {
		route = models.ErrandDeliveryNotification
	}
	var composition errands.Composition
	if composer, ok := s.errandComposer(); ok {
		var err error
		composition, err = composer.ComposeParentTurn(ctx, claim.Dispatch.SessionID, route)
		if err != nil {
			return "", "", err
		}
	}
	token := composition.Token
	if composition.Block != "" {
		if err := s.appendSendNowLiveReceipt(ctx, input, route, composition); err != nil {
			s.releaseErrandClaimUnlessAccepted(context.WithoutCancel(ctx), token, false)
			return "", "", err
		}
		prompt = joinErrandBlock(composition.Block, prompt)
	}
	if !input.hasOrdinary && len(input.snapshot.Receipts) == 0 {
		return "", token, errErrandNotificationEmpty
	}
	if !input.hasOrdinary && claim.Dispatch.Content == "" {
		claim.Dispatch.Content = composition.Block
	}
	input.snapshot.EffectivePrompt = prompt
	claim.Dispatch.Metadata[errands.SendNowDeliveryMetadataKey] = input.snapshot
	stampSendNowDisplay(claim, input.snapshot.Receipts, route, composition)
	if _, _, err := errands.DecodeSendNowDelivery(claim.Dispatch.Metadata); err != nil {
		s.releaseErrandClaimUnlessAccepted(context.WithoutCancel(ctx), token, false)
		return "", "", err
	}
	protocol, submissionID, payloadHash := claim.DeliverySubmission()
	if err := s.messageQueue.SetPendingSendNowClaimDelivery(ctx, claim, protocol, submissionID, payloadHash); err != nil {
		s.releaseErrandClaimUnlessAccepted(context.WithoutCancel(ctx), token, false)
		return "", "", fmt.Errorf("persist Send Now dispatch snapshot: %w", err)
	}
	return prompt, token, nil
}

func (s *Service) appendSendNowLiveReceipt(ctx context.Context, input *sendNowErrandInput, route models.ErrandDeliveryRoute, composition errands.Composition) error {
	reader, ok := s.repo.(errandClaimReader)
	if !ok {
		return errors.New("immutable errand claims are unavailable")
	}
	claims, err := reader.ListErrandResultClaims(ctx, input.snapshot.TaskID, input.snapshot.SessionID, composition.Token)
	if err != nil {
		return fmt.Errorf("read committed Send Now result claims: %w", err)
	}
	if len(claims) > 0 {
		input.snapshot.Receipts = append(input.snapshot.Receipts, errands.SendNowDeliveryReceipt{
			Envelope: errands.DeliveryEnvelope{Token: composition.Token, Route: route, Items: composition.Items}, Claims: claims,
		})
	}
	return nil
}

func stampSendNowDisplay(claim *messagequeue.SendNowClaim, receipts []errands.SendNowDeliveryReceipt, route models.ErrandDeliveryRoute, composition errands.Composition) {
	envelope := errands.DeliveryEnvelope{Route: route}
	for _, receipt := range receipts {
		envelope.Items = appendErrandDeliveryItems(envelope.Items, receipt.Envelope.Items)
		if envelope.Token == "" {
			envelope.Token = receipt.Envelope.Token
		}
	}
	envelope.Items = appendErrandDeliveryItems(envelope.Items, composition.Items)
	if len(envelope.Items) > 0 {
		claim.Dispatch.Metadata[errandDeliveryKey] = envelope
	}
	if envelope.Token != "" {
		claim.Dispatch.Metadata[ErrandDeliveryTokenMetadataKey] = envelope.Token
	}
	_, submissionID, _ := claim.DeliverySubmission()
	claim.Dispatch.Metadata[ErrandDeliverySubmissionMetadataKey] = normalizeDeliverySubmissionID(submissionID)
}

func (s *Service) sendNowHasErrandContext(claim *messagequeue.SendNowClaim) bool {
	if claim == nil {
		return false
	}
	if _, configured := s.errandComposer(); configured {
		return true
	}
	for i := range claim.Sources {
		source := &claim.Sources[i]
		if source.QueuedBy == messagequeue.QueuedByServer &&
			(isErrandNotificationEntry(source) || hasQueuedErrandDelivery(source) || source.Metadata[errandSourceMetadataKey] != nil) {
			return true
		}
	}
	return false
}

func preparedSourceGenerationsPresent(encoded []byte) bool {
	var fields struct {
		Receipt *struct {
			Claims []struct {
				Generation *int64 `json:"ParentAdmissionGeneration"`
			} `json:"claims"`
		} `json:"receipt"`
	}
	if err := json.Unmarshal(encoded, &fields); err != nil || fields.Receipt == nil {
		return false
	}
	for _, claim := range fields.Receipt.Claims {
		if claim.Generation == nil {
			return false
		}
	}
	return true
}
