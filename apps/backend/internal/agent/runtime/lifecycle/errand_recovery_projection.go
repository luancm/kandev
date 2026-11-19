package lifecycle

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

type errandRecoveryObservationStore interface {
	RecordErrandRecoveryObservation(context.Context, models.ErrandRecoveryObservation) (bool, error)
}

// SetErrandRecoveryRefresh wires publication after an owned recovery observation commits.
func (m *Manager) SetErrandRecoveryRefresh(refresh func(context.Context, string, string)) {
	if m.streamManager == nil {
		return
	}
	m.streamManager.deliveryMu.Lock()
	m.streamManager.errandRecoveryRefresh = refresh
	m.streamManager.deliveryMu.Unlock()
}

// SetErrandContextRefresh connects committed context updates to the browser publisher.
func (m *Manager) SetErrandContextRefresh(refresh func(context.Context, string, string)) {
	if m.streamManager == nil {
		return
	}
	m.streamManager.deliveryMu.Lock()
	m.streamManager.errandContextRefresh = refresh
	m.streamManager.deliveryMu.Unlock()
}

func (sm *StreamManager) refreshErrandContext(owner ExecutionOwner) {
	sm.deliveryMu.RLock()
	refresh := sm.errandContextRefresh
	sm.deliveryMu.RUnlock()
	if refresh == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	refresh(ctx, owner.TaskID, owner.ErrandID)
}

func (sm *StreamManager) projectErrandRecovery(ctx context.Context, identity DeliveryReconciliationIdentity, phase DeliveryReconciliationPhase) {
	if sm == nil || (phase != DeliveryReconciliationPhaseUncertain && phase != DeliveryReconciliationPhaseRecovered) {
		return
	}
	store, ok := sm.deliveryRepository().(errandRecoveryObservationStore)
	if !ok {
		return
	}
	owner := identity.Owner
	observation := models.ErrandRecoveryObservation{
		TaskID: owner.TaskID, TurnID: identity.TurnID, SubmissionID: identity.SubmissionID,
		PromptGeneration: identity.PromptGeneration, Unresolved: phase == DeliveryReconciliationPhaseUncertain,
		Binding: models.ErrandHarnessBinding{
			ErrandID: owner.ErrandID, Generation: owner.Generation, ExecutionID: identity.ExecutionID,
			IncarnationID: identity.IncarnationID, HarnessGeneration: int64(identity.HarnessGeneration),
			StreamID: identity.StreamID, NativeSessionID: identity.NativeSessionID,
		},
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	changed, err := store.RecordErrandRecoveryObservation(writeCtx, observation)
	if err != nil {
		sm.logger.Warn("Errand recovery observation unavailable", zap.String("errand_id", owner.ErrandID), zap.Error(err))
		return
	}
	if !changed {
		return
	}
	sm.deliveryMu.RLock()
	refresh := sm.errandRecoveryRefresh
	sm.deliveryMu.RUnlock()
	if refresh != nil {
		refresh(writeCtx, owner.TaskID, owner.ErrandID)
	}
}
