package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
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
	if s.messageQueue == nil || s.getActiveTurnID(parentSessionID) != "" ||
		s.isQueuedDispatchInFlight(parentSessionID) {
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
	_, err = s.messageQueue.QueueMessageWithMetadataForSession(
		ctx, identity, content, "", messagequeue.QueuedByServer, false, nil, metadata,
	)
	if err != nil {
		return fmt.Errorf("queue errand notification: %w", err)
	}
	s.publishQueueStatusEvent(ctx, parentSessionID)
	// Queue admission can race the parent's ready event. Recheck using the
	// captured incarnation so an idle Auto-run parent starts promptly without
	// dispatching into a replacement session.
	s.CheckQueueAdmissionReadiness(ctx, identity)
	return nil
}
