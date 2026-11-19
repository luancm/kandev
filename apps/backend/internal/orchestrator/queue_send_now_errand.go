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
	Provenance string                    `json:"provenance"`
	Kind       errands.SendNowSourceKind `json:"kind"`
}

type sendNowErrandInput struct {
	claim           *messagequeue.SendNowClaim
	snapshot        errands.SendNowDelivery
	composedBlock   string
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
		part, err := s.selectedSendNowSource(ctx, source, generation)
		if err != nil {
			return nil, err
		}
		input.snapshot.SourceIDs = append(input.snapshot.SourceIDs, source.ID)
		input.snapshot.Sources = append(input.snapshot.Sources, part)
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

func (s *Service) selectedSendNowSource(ctx context.Context, source *messagequeue.QueuedMessage, generation int64) (errands.SendNowDeliverySource, error) {
	if !s.errandQueueSourceSelection(ctx, source) {
		return errands.SendNowDeliverySource{}, errands.ErrInvalidSendNowDelivery
	}
	part := errands.SendNowDeliverySource{ID: source.ID, QueueGeneration: generation,
		Kind: errands.SendNowSourceOrdinary, QueuedBy: source.QueuedBy, Content: source.Content}
	if queuedErrandServerSource(source) {
		part.Kind = errands.SendNowSourceNotification
		part.Provenance = errands.SendNowErrandProvenance
	}
	return part, nil
}

func sendNowSourceHasInput(source *messagequeue.QueuedMessage) bool {
	return source.Content != "" || len(source.Attachments) > 0 || source.PlanMode ||
		len(entityrefs.NormalizePersisted(source.Metadata[messagequeue.MetadataEntityReferences])) > 0 ||
		stepHandoffFromQueuedMetadata(source.Metadata) != ""
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
			return fmt.Errorf("%w: persist Send Now delivery claim: %w", errLifecyclePromptMessagePersistence, err)
		}
		if present {
			if err := s.recordQueuedUserMessage(ctx, &claim.Dispatch, payload.Attachments, sendNowSourceIDs(claim)...); err != nil {
				return fmt.Errorf("%w: persist Send Now runtime prompt transcript: %w", errLifecyclePromptMessagePersistence, err)
			}
		}
		return nil
	}
}

func hasErrandNotificationStamp(metadata map[string]interface{}) bool {
	value, present := metadata[errandSourceMetadataKey]
	if !present {
		return false
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return false
	}
	var stamp sendNowErrandSource
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	return decoder.Decode(&stamp) == nil && stamp.Provenance == errands.SendNowErrandProvenance &&
		stamp.Kind == errands.SendNowSourceNotification
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
		input.composedBlock = composition.Block
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
