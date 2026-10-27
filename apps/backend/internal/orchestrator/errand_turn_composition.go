package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

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
	ErrandDeliveryTokenMetadataKey = "errand_delivery_token"
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
		if options.errandNotification && prompt == "" {
			return "", "", errErrandNotificationEmpty
		}
		return prompt, "", nil
	}
	if route != models.ErrandDeliveryNotification {
		s.withdrawErrandNotification(ctx, sessionID)
	}
	recordErrandDelivery(options.queuedMessage, route, composition)
	return joinErrandBlock(composition.Block, prompt), composition.Token, nil
}

// recordErrandDelivery stamps a queued entry's user message with the route and
// claim token that delivered its results. A notification has no text of its
// own, so its status block becomes the entry's content and transcript text.
func recordErrandDelivery(entry *messagequeue.QueuedMessage, route models.ErrandDeliveryRoute, composition errands.Composition) {
	if entry == nil {
		return
	}
	if entry.Metadata == nil {
		entry.Metadata = map[string]interface{}{}
	}
	entry.Metadata[errandDeliveryKey] = string(route)
	entry.Metadata[ErrandDeliveryTokenMetadataKey] = composition.Token
	if route == models.ErrandDeliveryNotification {
		entry.Content = composition.Block
	}
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
