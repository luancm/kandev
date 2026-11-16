package lifecycle

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

type interruptedDelegationStore interface {
	ReconcileErrandInterruptedTerminalReceipts(context.Context, string) error
	HasPendingInterruptedDelegation(context.Context, string) (bool, error)
	PreserveErrandInterruptedDelegation(context.Context, *models.ExecutorRunning) error
	ConsumeErrandInterruptedDelegations(context.Context, *models.NativeResumeDeliveryEvidence) error
}

func (m *Manager) preserveInterruptedDelegation(ctx context.Context, sessionID string) error {
	if m.streamManager == nil {
		return nil
	}
	store, ok := m.streamManager.deliveryRepository().(interruptedDelegationStore)
	if !ok {
		return nil
	}
	reader, ok := m.runningWriter.(executorRunningReader)
	if !ok {
		return nil
	}
	running, err := reader.GetExecutorRunningBySessionID(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return store.PreserveErrandInterruptedDelegation(ctx, running)
}

func (m *Manager) consumeInterruptedDelegations(ctx context.Context, proof *models.NativeResumeDeliveryEvidence) error {
	if m.streamManager == nil || proof == nil {
		return nil
	}
	store, ok := m.streamManager.deliveryRepository().(interruptedDelegationStore)
	if !ok {
		return nil
	}
	return store.ConsumeErrandInterruptedDelegations(ctx, proof)
}

// ReconcileInterruptedErrandDelivery retries retained assessments without
// manufacturing new owner proof from an already-retired native conversation.
func (m *Manager) ReconcileInterruptedErrandDelivery(ctx context.Context, sessionID string) error {
	if m.streamManager == nil {
		return nil
	}
	store, ok := m.streamManager.deliveryRepository().(interruptedDelegationStore)
	if !ok {
		return nil
	}
	if err := store.ReconcileErrandInterruptedTerminalReceipts(ctx, sessionID); err != nil {
		return err
	}
	pending, err := store.HasPendingInterruptedDelegation(ctx, sessionID)
	if err != nil || !pending {
		return err
	}
	proof, err := m.GetNativeResumeDeliveryEvidence(ctx, sessionID)
	if err != nil {
		return err
	}
	return store.ConsumeErrandInterruptedDelegations(ctx, proof)
}

func (m *Manager) savedInterruptedDelegationOwner(ctx context.Context, execution *AgentExecution, row *models.AgentDeliverySubmission, streamID string) (*AgentDeliveryRecoveryIdentity, error) {
	if m.streamManager == nil {
		return nil, nil
	}
	store, ok := m.streamManager.deliveryRepository().(interface {
		GetInterruptedDelegationAssessment(context.Context, string, string) (*models.InterruptedDelegationAssessment, error)
	})
	if !ok {
		return nil, nil
	}
	p, err := store.GetInterruptedDelegationAssessment(ctx, execution.SessionID, row.ID)
	if err != nil || p == nil {
		return nil, err
	}
	if p.TaskID != execution.TaskID || p.SessionID != execution.SessionID || p.IncarnationID != row.IncarnationID || p.HarnessGeneration != uint64(row.HarnessGeneration) || p.StreamID != streamID || p.PayloadHash != row.PayloadHash || p.NativeSessionID != execution.ACPSessionID {
		return nil, ErrDeliveryOwnerMismatch
	}
	identity := interruptedDelegationIdentity(p)
	return &identity, nil
}

func interruptedDelegationIdentity(p *models.InterruptedDelegationAssessment) AgentDeliveryRecoveryIdentity {
	return AgentDeliveryRecoveryIdentity{OriginalRuntime: p.OriginalRuntime, TaskID: p.TaskID, SessionID: p.SessionID, ExecutionID: p.ExecutionID, IncarnationID: p.IncarnationID, HarnessGeneration: p.HarnessGeneration, PromptGeneration: p.PromptGeneration, StreamID: p.StreamID, SubmissionID: p.SubmissionID, TurnID: p.TurnID, NativeSessionID: p.NativeSessionID}
}

func (m *Manager) savedInterruptedSuccessorMatches(ctx context.Context, execution *AgentExecution, historical AgentDeliveryRecoveryIdentity) (bool, error) {
	store, ok := m.streamManager.deliveryRepository().(interface {
		GetInterruptedDelegationAssessment(context.Context, string, string) (*models.InterruptedDelegationAssessment, error)
	})
	if !ok {
		return false, nil
	}
	p, err := store.GetInterruptedDelegationAssessment(ctx, execution.SessionID, historical.SubmissionID)
	if err != nil || p == nil {
		return false, err
	}
	return interruptedDelegationIdentity(p) == historical && execution.TaskID == p.TaskID &&
		execution.SessionID == p.SessionID && execution.DeliveryIncarnationID == p.IncarnationID &&
		execution.ACPSessionID == p.NativeSessionID && execution.DeliveryMode == DurableDeliveryV1 &&
		execution.DeliveryHarnessGeneration > p.HarnessGeneration && execution.isSessionInitialized(), nil
}
