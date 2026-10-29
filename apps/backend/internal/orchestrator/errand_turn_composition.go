package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// errErrandNotificationEmpty reports that a notification entry reached dispatch
// after its results were taken by another turn. The drain consumes the entry
// without starting a turn.
var errErrandNotificationEmpty = errors.New("errand notification has no pending result")

// errandDeliveryKey names the user message metadata that records how a parent
// turn delivered its errand results.
const (
	errandDeliveryKey = "errand_delivery"
	// ErrandDeliveryTokenMetadataKey is the user message metadata key that
	// records the claim token of the errand results a turn delivered.
	// Reconciliation reads it through the delivery log.
	ErrandDeliveryTokenMetadataKey = errands.DeliveryTokenMetadataKey
	// ErrandDeliverySubmissionMetadataKey links a delivery snapshot to the
	// prompt submission whose durable outcome reconciliation reads back.
	ErrandDeliverySubmissionMetadataKey = "errand_delivery_submission_id"
)

// errandTurnComposer is the composition part of the errands service. It is
// type-asserted from ErrandController so the controller's method set stays fixed.
type errandTurnComposer interface {
	ComposeParentTurn(ctx context.Context, parentSessionID string, route models.ErrandDeliveryRoute) (errands.Composition, error)
	ReleaseTurnClaim(ctx context.Context, token string) error
}

func (s *Service) errandComposer() (errandTurnComposer, bool) {
	if s.errands == nil {
		return nil, false
	}
	composer, ok := s.errands.(errandTurnComposer)
	return composer, ok
}

// composeErrandTurn prepends the errand status block to a parent turn's prompt
// and returns the claim token that marks its results delivered. A notification
// entry dispatched with no pending result returns errErrandNotificationEmpty.
// A composition failure is logged and the prompt proceeds unchanged: errand
// state never gates a parent prompt, and undelivered results stay pending.
func (s *Service) composeErrandTurn(
	ctx context.Context, sessionID, prompt string, options *promptTaskOptions,
) (string, string, error) {
	composer, ok := s.errandComposer()
	if !ok || sessionID == "" {
		return prompt, "", nil
	}
	route := models.ErrandDeliveryTurnContext
	if options.errandNotification {
		route = models.ErrandDeliveryNotification
	}
	composition, err := composer.ComposeParentTurn(ctx, sessionID, route)
	if err != nil {
		s.logger.Warn("errand status block unavailable; prompt sent without it",
			zap.String("session_id", sessionID), zap.Error(err))
		return prompt, "", nil
	}
	if composition.Block == "" {
		if options.errandNotification && !hasErrandNotificationPrompt(prompt, options) {
			return "", "", errErrandNotificationEmpty
		}
		return prompt, "", nil
	}
	if options.queuedMessage == nil {
		if options.deliverySubmissionID == "" {
			options.deliverySubmissionID = "internal:" + uuid.NewString()
		}
		if err := s.recordDirectErrandDelivery(
			ctx, sessionID, options.deliverySubmissionID, options.directMessageContent, route, composition,
		); err != nil {
			s.releaseErrandClaimUnlessAccepted(context.WithoutCancel(ctx), composition.Token, false)
			s.logger.Warn("errand delivery snapshot could not be persisted; prompt sent without it",
				zap.String("session_id", sessionID), zap.Error(err))
			return prompt, "", nil
		}
	} else {
		recordErrandDelivery(options.queuedMessage, route, composition, options.deliverySubmissionID)
	}
	if route != models.ErrandDeliveryNotification {
		s.withdrawErrandNotification(ctx, sessionID)
	}
	return joinErrandBlock(composition.Block, prompt), composition.Token, nil
}

func hasErrandNotificationPrompt(prompt string, options *promptTaskOptions) bool {
	if options == nil || options.queuedMessage == nil {
		if options != nil && strings.TrimSpace(options.directMessageContent) != "" {
			return true
		}
		return strings.TrimSpace(prompt) != ""
	}
	entry := options.queuedMessage
	return strings.TrimSpace(entry.Content) != "" || len(entry.Attachments) > 0 ||
		len(entityrefs.NormalizePersisted(entry.Metadata[messagequeue.MetadataEntityReferences])) > 0 ||
		stepHandoffFromQueuedMetadata(entry.Metadata) != ""
}

// recordErrandDelivery stamps a queued entry's user message with its immutable
// result snapshot and the durable prompt submission identity. A notification
// has no text of its own, so its status block becomes the entry content.
func recordErrandDelivery(entry *messagequeue.QueuedMessage, route models.ErrandDeliveryRoute, composition errands.Composition, submissionID string) {
	if entry == nil {
		return
	}
	if entry.Metadata == nil {
		entry.Metadata = map[string]interface{}{}
	}
	items := appendErrandDeliveryItems(queuedErrandDeliveryItems(entry.Metadata), composition.Items)
	entry.Metadata[errandDeliveryKey] = errands.DeliveryEnvelope{
		Token: composition.Token, Route: route, Items: items,
	}
	entry.Metadata[ErrandDeliveryTokenMetadataKey] = composition.Token
	if submissionID != "" {
		entry.Metadata[ErrandDeliverySubmissionMetadataKey] = normalizeDeliverySubmissionID(submissionID)
	}
	if route == models.ErrandDeliveryNotification && strings.TrimSpace(entry.Content) == "" {
		entry.Content = composition.Block
	}
}

func queuedErrandDelivery(entry *messagequeue.QueuedMessage) (errands.DeliveryEnvelope, bool) {
	if entry == nil || entry.Metadata == nil {
		return errands.DeliveryEnvelope{}, false
	}
	envelope, ok := decodeErrandDeliveryEnvelope(entry.Metadata[errandDeliveryKey])
	return envelope, ok && envelope.Token != ""
}

func queuedErrandDeliveryItems(metadata map[string]interface{}) []errands.DeliveryItem {
	envelope, ok := decodeErrandDeliveryEnvelope(metadata[errandDeliveryKey])
	if !ok {
		return nil
	}
	return envelope.Items
}

func decodeErrandDeliveryEnvelope(value interface{}) (errands.DeliveryEnvelope, bool) {
	if envelope, ok := value.(errands.DeliveryEnvelope); ok {
		return envelope, true
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return errands.DeliveryEnvelope{}, false
	}
	var envelope errands.DeliveryEnvelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		return errands.DeliveryEnvelope{}, false
	}
	return envelope, true
}

func appendErrandDeliveryItems(previous, next []errands.DeliveryItem) []errands.DeliveryItem {
	items := make([]errands.DeliveryItem, 0, len(previous)+len(next))
	seen := make(map[string]struct{}, len(previous)+len(next))
	for _, item := range append(append([]errands.DeliveryItem(nil), previous...), next...) {
		if item.ErrandID == "" {
			continue
		}
		if _, exists := seen[item.ErrandID]; exists {
			continue
		}
		seen[item.ErrandID] = struct{}{}
		items = append(items, item)
	}
	return items
}

func hasQueuedErrandDelivery(entry *messagequeue.QueuedMessage) bool {
	_, ok := queuedErrandDelivery(entry)
	return ok
}

func (s *Service) recordDirectErrandDelivery(
	ctx context.Context, sessionID, submissionID, messageContent string,
	route models.ErrandDeliveryRoute, composition errands.Composition,
) error {
	if s.transientRetryMessages == nil {
		return errors.New("direct parent message is unavailable")
	}
	messages, err := s.transientRetryMessages.ListMessages(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("list parent messages: %w", err)
	}
	activeTurnID, _ := s.peekActiveTurnID(ctx, sessionID)
	var target *models.Message
	for _, message := range messages {
		if message == nil || message.AuthorType != models.MessageAuthorUser {
			continue
		}
		if submissionID != "" && message.ID == submissionID {
			target = message
			break
		}
		if message.Content != messageContent || (activeTurnID != "" && message.TurnID != activeTurnID) {
			continue
		}
		if target == nil || !message.CreatedAt.Before(target.CreatedAt) {
			target = message
		}
	}
	if target == nil {
		return errors.New("direct parent user message was not found")
	}
	return s.updateErrandDeliveryMessage(ctx, sessionID, target.ID, submissionID, route, composition)
}

func (s *Service) updateErrandDeliveryMessage(
	ctx context.Context, sessionID, messageID, submissionID string,
	route models.ErrandDeliveryRoute, composition errands.Composition,
) error {
	if messageID == "" || s.transientRetryMessages == nil {
		return errors.New("parent message service is unavailable")
	}
	messages, err := s.transientRetryMessages.ListMessages(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("list parent messages: %w", err)
	}
	for _, message := range messages {
		if message == nil || message.ID != messageID || message.AuthorType != models.MessageAuthorUser {
			continue
		}
		updated := *message
		updated.Metadata = make(map[string]interface{}, len(message.Metadata)+3)
		for key, value := range message.Metadata {
			updated.Metadata[key] = value
		}
		items := appendErrandDeliveryItems(queuedErrandDeliveryItems(message.Metadata), composition.Items)
		updated.Metadata[errandDeliveryKey] = errands.DeliveryEnvelope{
			Token: composition.Token, Route: route, Items: items,
		}
		updated.Metadata[ErrandDeliveryTokenMetadataKey] = composition.Token
		if submissionID != "" {
			updated.Metadata[ErrandDeliverySubmissionMetadataKey] = normalizeDeliverySubmissionID(submissionID)
		}
		if err := s.transientRetryMessages.UpdateMessage(ctx, &updated); err != nil {
			return fmt.Errorf("update parent message: %w", err)
		}
		return nil
	}
	return errors.New("direct parent user message was not found")
}

func joinErrandBlock(block, prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return block
	}
	return block + "\n\n" + prompt
}

// releaseErrandClaimUnlessAccepted returns the turn's results to pending when
// the prompt was never accepted by the provider.
func (s *Service) releaseErrandClaimUnlessAccepted(ctx context.Context, token string, accepted bool) {
	if token == "" || accepted {
		return
	}
	composer, ok := s.errandComposer()
	if !ok {
		return
	}
	if err := composer.ReleaseTurnClaim(ctx, token); err != nil {
		s.logger.Warn("failed to release errand result claim",
			zap.String("token", token), zap.Error(err))
	}
}

// withdrawErrandNotification removes a waiting notification entry once another
// turn has taken the results it announced.
func (s *Service) withdrawErrandNotification(ctx context.Context, parentSessionID string) {
	if s.messageQueue == nil {
		return
	}
	status := s.messageQueue.GetStatus(ctx, parentSessionID)
	if status == nil {
		return
	}
	var identity messagequeue.QueueSessionIdentity
	for i := range status.Entries {
		entry := &status.Entries[i]
		if !isErrandNotificationEntry(entry) {
			continue
		}
		if identity.SessionID == "" {
			resolved, err := s.parentQueueIdentity(ctx, parentSessionID)
			if err != nil {
				s.logger.Warn("failed to resolve parent queue identity for errand withdrawal",
					zap.String("session_id", parentSessionID), zap.Error(err))
				return
			}
			identity = resolved
		}
		if _, err := s.messageQueue.RemoveEntryForSession(ctx, identity, entry.ID); err != nil {
			s.logger.Warn("failed to withdraw errand notification",
				zap.String("session_id", parentSessionID), zap.String("entry_id", entry.ID), zap.Error(err))
		}
	}
}

// parentQueueIdentity resolves the immutable queue identity of a parent session.
func (s *Service) parentQueueIdentity(ctx context.Context, parentSessionID string) (messagequeue.QueueSessionIdentity, error) {
	session, err := s.repo.GetTaskSession(ctx, parentSessionID)
	if err != nil {
		return messagequeue.QueueSessionIdentity{}, fmt.Errorf("load parent session: %w", err)
	}
	return s.messageQueue.ResolveSessionIdentity(ctx, session.TaskID, parentSessionID)
}

// isErrandNotificationEntry reports whether a queue entry is a notification
// that carries no prompt of its own.
func isErrandNotificationEntry(entry *messagequeue.QueuedMessage) bool {
	if entry == nil {
		return false
	}
	marked, _ := entry.Metadata[errandNotificationKey].(bool)
	return marked
}
