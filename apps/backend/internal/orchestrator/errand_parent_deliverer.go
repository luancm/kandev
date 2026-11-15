package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// errandNotificationKey marks a queue entry that carries no prompt of its own.
// Its status block is composed at dispatch, so an entry that finds no pending
// result is consumed without starting a turn.
const errandNotificationKey = "errand_notification"

// ErrandParentDeliverer admits errand notifications onto a parent session's
// orchestrator message queue. It satisfies errands.ParentDeliverer.
type ErrandParentDeliverer struct {
	svc *Service
}

var _ errands.ParentDeliverer = (*ErrandParentDeliverer)(nil)

// NewErrandParentDeliverer wraps the orchestrator queue for the errands service.
func NewErrandParentDeliverer(svc *Service) *ErrandParentDeliverer {
	return &ErrandParentDeliverer{svc: svc}
}

// NotificationBlocked reports that a parent turn is running or a queue entry,
// a notification included, is already waiting. Either case leaves the next
// turn to take the pending results.
func (d *ErrandParentDeliverer) NotificationBlocked(ctx context.Context, parentSessionID string) bool {
	s := d.svc
	active, _ := s.activeTurns.Load(parentSessionID)
	cachedTurnID, _ := active.(string)
	if s.messageQueue == nil || cachedTurnID != "" || s.reservedPromptTurnID(parentSessionID) != "" ||
		s.isQueuedDispatchInFlight(parentSessionID) {
		return true
	}
	turnID, err := s.peekActiveTurnID(ctx, parentSessionID)
	if err != nil || turnID != "" {
		return true
	}
	status := s.messageQueue.GetStatus(ctx, parentSessionID)
	return status == nil || status.Count > 0
}

// DeliverPending admits one empty notification entry for the parent. A refused
// admission is logged and leaves the results pending for the next trigger.
func (d *ErrandParentDeliverer) DeliverPending(ctx context.Context, parentSessionID string) {
	err := d.admit(ctx, parentSessionID, "", map[string]interface{}{errandNotificationKey: true})
	if err != nil {
		d.svc.logger.Warn("errand notification refused; results stay pending",
			zap.String("parent_session_id", parentSessionID), zap.Error(err))
	}
}

// AdmitPrepared admits one entry whose content is already the status block.
func (d *ErrandParentDeliverer) AdmitPrepared(
	ctx context.Context, parentSessionID, content string, metadata map[string]interface{},
) error {
	return d.admit(ctx, parentSessionID, content, metadata)
}

func (d *ErrandParentDeliverer) admit(
	ctx context.Context, parentSessionID, content string, metadata map[string]interface{},
) error {
	s := d.svc
	if s.messageQueue == nil {
		return errors.New("message queue is not configured")
	}
	identity, err := s.parentQueueIdentity(ctx, parentSessionID)
	if err != nil {
		return fmt.Errorf("resolve parent for errand delivery: %w", err)
	}
	stamped := make(map[string]interface{}, len(metadata)+1)
	for key, value := range metadata {
		stamped[key] = value
	}
	var prepared *sendNowErrandSource
	if content == "" {
		stamped[errandSourceMetadataKey] = sendNowErrandSource{Provenance: errands.SendNowErrandProvenance, Kind: errands.SendNowSourceNotification}
	} else if envelope, ok := decodeErrandDeliveryEnvelope(metadata[errandDeliveryKey]); ok && envelope.Token != "" {
		stamp, stampErr := s.capturePreparedErrandSource(ctx, identity.TaskID, parentSessionID, content, metadata)
		if stampErr != nil {
			return stampErr
		}
		stamped[errandSourceMetadataKey] = stamp
		prepared = &stamp
	} else {
		return errands.ErrInvalidSendNowDelivery
	}
	if prepared != nil {
		key := preparedErrandQueueKey(identity, *prepared)
		prepared.Receipt.SourceID = key
		owner, ok := s.repo.(messagequeue.PreparedServerAdmission)
		if !ok {
			return errors.New("prepared errand ownership admission is unavailable")
		}
		_, err = s.messageQueue.QueuePreparedServerMessageForSession(ctx, identity, key, content, stamped, owner)
	} else {
		_, err = s.messageQueue.QueueMessageWithMetadataForSession(ctx, identity, content, "", messagequeue.QueuedByServer, false, nil, stamped)
	}
	if err != nil {
		return fmt.Errorf("queue errand notification: %w", err)
	}
	s.publishQueueStatusEvent(ctx, parentSessionID)
	// Queue admission can race the parent's ready event. Recheck using the
	// captured incarnation so an idle Auto-run parent starts promptly without
	// dispatching into a replacement session.
	if !workflowProfileSwitchGuardIsHeld(ctx, parentSessionID) && !cancelInFlightGuardHeldForSession(ctx, parentSessionID) {
		s.CheckQueueAdmissionReadiness(ctx, identity)
	}
	return nil
}

// capturePreparedErrandSource runs before deletion, while exact revision and
// admission-generation authority is still available. The queue then owns it.
func (s *Service) capturePreparedErrandSource(ctx context.Context, taskID, parentID, content string, metadata map[string]interface{}) (sendNowErrandSource, error) {
	envelope, ok := decodeErrandDeliveryEnvelope(metadata[errandDeliveryKey])
	if !ok || envelope.Token == "" || envelope.Route != models.ErrandDeliveryNotification || len(envelope.Items) == 0 || content == "" {
		return sendNowErrandSource{}, errands.ErrInvalidSendNowDelivery
	}
	if scalar, present := metadata[ErrandDeliveryTokenMetadataKey]; present && scalar != envelope.Token {
		return sendNowErrandSource{}, errands.ErrInvalidSendNowDelivery
	}
	reader, ok := s.repo.(interface {
		GetErrand(context.Context, string) (*models.Errand, error)
	})
	if !ok {
		return sendNowErrandSource{}, errors.New("prepared errand revision authority is unavailable")
	}
	receipt := &errands.SendNowDeliveryReceipt{Envelope: envelope}
	for _, item := range envelope.Items {
		row, err := reader.GetErrand(ctx, item.ErrandID)
		if err != nil {
			return sendNowErrandSource{}, fmt.Errorf("capture prepared errand revision: %w", err)
		}
		if !preparedErrandRowMatches(row, taskID, parentID) {
			return sendNowErrandSource{}, errands.ErrInvalidSendNowDelivery
		}
		receipt.Claims = append(receipt.Claims, models.ErrandResultClaim{ErrandID: row.ID, ResultRevision: row.ResultRevision,
			ParentAdmissionGeneration: row.ParentAdmissionGeneration, Token: envelope.Token, Route: envelope.Route})
	}
	identity, err := s.parentQueueIdentity(ctx, parentID)
	if err != nil {
		return sendNowErrandSource{}, err
	}
	return sendNowErrandSource{Identity: &identity, Provenance: errands.SendNowErrandProvenance, Kind: errands.SendNowSourcePrepared,
		PreparedBlock: content, Receipt: receipt}, nil
}

func preparedErrandRowMatches(row *models.Errand, taskID, parentID string) bool {
	return row.TaskID == taskID && row.ParentSessionID == parentID && row.ResultRevision != "" && (row.ResultState == models.ErrandResultPending || row.ResultState == models.ErrandResultDelivered)
}
