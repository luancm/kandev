package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type errandRecoveryProjectionStore struct {
	AgentDeliveryRepository
	observations []models.ErrandRecoveryObservation
	changed      bool
	err          error
}

func (s *errandRecoveryProjectionStore) RecordErrandRecoveryObservation(_ context.Context, observation models.ErrandRecoveryObservation) (bool, error) {
	s.observations = append(s.observations, observation)
	return s.changed, s.err
}

func TestErrandRecoveryProjectionRefreshesCommittedObservations(t *testing.T) {
	ctx := t.Context()
	sm := newDeliveryReconciliationTestManager(newReconciliationTestClock())
	defer sm.Wait()
	manager := &Manager{streamManager: sm}
	store := &errandRecoveryProjectionStore{}
	sm.setAgentDeliveryRepository(store)
	var notifications [][2]string
	manager.SetErrandRecoveryRefresh(func(_ context.Context, taskID, errandID string) {
		notifications = append(notifications, [2]string{taskID, errandID})
	})
	identity := DeliveryReconciliationIdentity{
		Owner:       ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "errand", Generation: 2},
		ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 3, StreamID: "stream",
		NativeSessionID: "native", TurnID: "turn", SubmissionID: "submission", PromptGeneration: 4,
	}
	sm.projectErrandRecovery(ctx, identity, DeliveryReconciliationPhaseReconnecting)
	require.Empty(t, store.observations)
	sm.projectErrandRecovery(ctx, identity, DeliveryReconciliationPhaseUncertain)
	require.Empty(t, notifications, "an unchanged observation must not publish")
	want := models.ErrandRecoveryObservation{
		TaskID: "task", TurnID: "turn", SubmissionID: "submission", PromptGeneration: 4, Unresolved: true,
		Binding: models.ErrandHarnessBinding{
			ErrandID: "errand", Generation: 2, ExecutionID: "execution", IncarnationID: "incarnation",
			HarnessGeneration: 3, StreamID: "stream", NativeSessionID: "native",
		},
	}
	require.Equal(t, []models.ErrandRecoveryObservation{want}, store.observations)

	store.changed = true
	sm.projectErrandRecovery(ctx, identity, DeliveryReconciliationPhaseUncertain)
	require.Equal(t, [][2]string{{"task", "errand"}}, notifications)
	store.err = errors.New("observation commit failed")
	sm.projectErrandRecovery(ctx, identity, DeliveryReconciliationPhaseRecovered)
	require.Len(t, notifications, 1)
	store.err = nil
	sm.projectErrandRecovery(ctx, identity, DeliveryReconciliationPhaseRecovered)
	require.Len(t, notifications, 2)
	want.Unresolved = false
	require.Equal(t, want, store.observations[len(store.observations)-1])
}
