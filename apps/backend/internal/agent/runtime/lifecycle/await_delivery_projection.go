package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

type errandAwaitRecoveryRepository interface {
	ListUnresolvedErrandAwaits(context.Context, string) ([]models.ErrandAwaitIdentity, error)
	RecoverErrandAwait(context.Context, models.ErrandAwaitIdentity) (bool, error)
	ClassifyErrandAwaitTerminal(context.Context, *models.AgentDeliveryEvent) error
}

// SetErrandAwaitRefresh wires best-effort presentation after SQL evidence
// commits. It does not grant receipt or release authority to the callback.
func (m *Manager) SetErrandAwaitRefresh(refresh func(context.Context, string)) {
	if m.streamManager == nil {
		return
	}
	m.streamManager.deliveryMu.Lock()
	m.streamManager.errandAwaitRefresh = refresh
	m.streamManager.deliveryMu.Unlock()
}

func (sm *StreamManager) refreshErrandAwait(ctx context.Context, parentID string) {
	sm.deliveryMu.RLock()
	refresh := sm.errandAwaitRefresh
	sm.deliveryMu.RUnlock()
	if refresh != nil {
		refresh(ctx, parentID)
	}
}

func (sm *StreamManager) recoverErrandAwaitOwner(ctx context.Context, execution *AgentExecution) error {
	if execution.TaskID == "" || execution.SessionID == "" || execution.DeliveryIncarnationID == "" || execution.DeliveryHarnessGeneration == 0 {
		return nil
	}
	store, ok := sm.deliveryRepository().(errandAwaitRecoveryRepository)
	if !ok {
		return nil
	}
	attempts, err := store.ListUnresolvedErrandAwaits(ctx, execution.SessionID)
	if err != nil {
		return err
	}
	for _, id := range attempts {
		if id.TaskID != execution.TaskID || id.ParentIncarnationID != execution.DeliveryIncarnationID || id.HarnessGeneration != int64(execution.DeliveryHarnessGeneration) {
			continue
		}
		if _, err := store.RecoverErrandAwait(ctx, id); err != nil {
			return err
		}
	}
	sm.refreshErrandAwait(ctx, execution.SessionID)
	return nil
}

func (sm *StreamManager) finishErrandAwaitRecovery(ctx context.Context, execution *AgentExecution, ownerCurrent func() error) error {
	if ownerCurrent != nil {
		if err := ownerCurrent(); err != nil {
			return err
		}
	}
	return sm.recoverErrandAwaitOwner(ctx, execution)
}

func (sm *StreamManager) classifyErrandAwaitTerminal(ctx context.Context, execution *AgentExecution, delivery AgentDeliveryRepository, prepared preparedAgentEvent) error {
	store, ok := delivery.(errandAwaitRecoveryRepository)
	if !ok || prepared.durableEvent == nil || !prepared.durableEvent.Terminal {
		return nil
	}
	event := *prepared.durableEvent
	// Inbox rows retain the authenticated stream identity; private host fields
	// are supplied again by the recovered execution during prefix projection.
	if event.TaskID == "" && execution != nil {
		owner := durableAgentEvent(execution, prepared.event)
		event.TaskID, event.ExecutionID = owner.TaskID, owner.ExecutionID
		event.OwnerGeneration, event.ExpectedStreamID = owner.OwnerGeneration, owner.ExpectedStreamID
	}
	if err := store.ClassifyErrandAwaitTerminal(ctx, &event); err != nil {
		return err
	}
	sm.refreshErrandAwait(ctx, event.SessionID)
	return nil
}
