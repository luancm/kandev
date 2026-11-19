package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/task/models"
)

type ownedErrandInterruptionStore interface {
	AgentDeliveryRepository
	ListOwnedErrandInterruptionAdmissions(context.Context) ([]models.ErrandInterruptedAdmission, error)
	CommitOwnedErrandInterruption(context.Context, models.ErrandInterruptedAdmission, int64) (bool, error)
}

// ReconcileOwnedErrandInterruptions checks the retained local journal only
// after the exact original writer's process identity is proven terminated.
func (m *Manager) ReconcileOwnedErrandInterruptions(ctx context.Context) error {
	if m.runtimeOwner == nil || m.streamManager == nil || !m.recoveryComplete.Load() {
		return nil
	}
	store, ok := m.streamManager.deliveryRepository().(ownedErrandInterruptionStore)
	if !ok {
		return ErrDurableAdoptionEvidenceUnavailable
	}
	admissions, err := store.ListOwnedErrandInterruptionAdmissions(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, admission := range admissions {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		terminated, err := processidentity.OriginalOwnerTerminated(admission.OriginalRuntime)
		if err == nil && terminated {
			attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = m.reconcileOwnedErrandInterruption(attempt, store, admission)
			cancel()
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("reconcile Errand %s interruption: %w", admission.Binding.ErrandID, err))
		}
	}
	return errors.Join(failures...)
}

func (m *Manager) reconcileOwnedErrandInterruption(ctx context.Context, store ownedErrandInterruptionStore, admission models.ErrandInterruptedAdmission) error {
	b := admission.Binding
	sessionID := models.ErrandExecutionIdentity(b.ErrandID)
	cursor, err := store.GetAgentDeliveryCursor(ctx, b.StreamID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var after uint64
	if cursor != nil {
		if cursor.SessionID != sessionID || cursor.IncarnationID != b.IncarnationID ||
			cursor.HarnessGeneration != b.HarnessGeneration || cursor.ProjectedSequence < 0 {
			return ErrDeliveryOwnerMismatch
		}
		after = uint64(cursor.ProjectedSequence)
	}
	lease, err := m.runtimeOwner.Acquire(ctx)
	if err != nil {
		return err
	}
	defer lease.Close()
	control := lease.NewControlClient(m.logger)
	if control == nil {
		return ErrDeliveryTransportUnavailable
	}
	owner := ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: admission.TaskID, ErrandID: b.ErrandID, Generation: b.Generation}
	if err := control.StopErrandInstance(ctx, b.ExecutionID, *errandMCPOwner(owner)); err != nil {
		return err
	}
	request := m.ownedErrandRetainedRequest(admission, after)
	execution := &AgentExecution{
		ID: b.ExecutionID, Owner: owner,
		ACPSessionID: b.NativeSessionID, DeliveryMode: DurableDeliveryV1, DeliveryIncarnationID: b.IncarnationID,
		DeliveryHarnessGeneration: uint64(b.HarnessGeneration), DeliveryStreamID: b.StreamID,
		DeliveryReplayCursor: after, promptGeneration: admission.PromptGeneration, promptTurnID: admission.TurnID,
	}
	execution.setDeliverySubmissionID(admission.SubmissionID)
	evidence, end, _, err := m.replayRetainedJournalPages(ctx, lease, control, request, execution)
	if err != nil {
		return err
	}
	if err := lease.CheckCurrent(); err != nil {
		return err
	}
	return settleRecoveredErrandInterruption(ctx, store, admission, evidence, end)
}

func (m *Manager) ownedErrandRetainedRequest(admission models.ErrandInterruptedAdmission, after uint64) journal.RetainedRecoveryRequest {
	b := admission.Binding
	return journal.RetainedRecoveryRequest{
		Root: m.dataDir, SessionID: models.ErrandExecutionIdentity(b.ErrandID), StorageOwnerID: journal.ErrandStorageOwnerID(b.ErrandID),
		ExecutionID: b.ExecutionID, IncarnationID: b.IncarnationID, HarnessGeneration: uint64(b.HarnessGeneration),
		StreamID: b.StreamID, SubmissionID: admission.SubmissionID, OriginalRuntime: admission.OriginalRuntime,
		ExpectedHash: admission.PayloadHash, After: after, Limit: 4,
	}
}

func (m *Manager) retireOwnedErrandInterruption(ctx context.Context, target models.ErrandTeardownTarget) error {
	if m.streamManager == nil {
		return nil
	}
	store, ok := m.streamManager.deliveryRepository().(interface {
		ReadOwnedErrandInterruption(context.Context, models.ErrandTeardownTarget) (*models.ErrandInterruptedAdmission, int64, error)
	})
	if !ok {
		return nil
	}
	admission, cursor, err := store.ReadOwnedErrandInterruption(ctx, target)
	if err != nil || admission == nil {
		return err
	}
	if m.runtimeOwner == nil {
		return ErrDeliveryTransportUnavailable
	}
	if err := m.streamManager.awaitDeliveryDurability(ctx); err != nil {
		return err
	}
	lease, err := m.runtimeOwner.Acquire(ctx)
	if err != nil {
		return err
	}
	defer lease.Close()
	control := lease.NewControlClient(m.logger)
	if control == nil {
		return ErrDeliveryTransportUnavailable
	}
	request := m.ownedErrandRetainedRequest(*admission, uint64(cursor))
	request.RetireInterrupted = true
	response, err := control.ReadRetainedDelivery(ctx, request)
	if err != nil {
		return err
	}
	if !response.ProcessTerminated || !response.Submission.Retired || response.Submission.Hash != admission.PayloadHash ||
		!matchesDeliveryReconciliationOwner(deliveryReconciliationIdentity(AgentDeliveryRecoveryIdentity{
			SessionID: request.SessionID, IncarnationID: request.IncarnationID, HarnessGeneration: request.HarnessGeneration,
			StreamID: request.StreamID, SubmissionID: request.SubmissionID,
		}), &response.Descriptor, &response.Submission) {
		return ErrDeliveryOwnerMismatch
	}
	return lease.CheckCurrent()
}

func settleRecoveredErrandInterruption(ctx context.Context, store ownedErrandInterruptionStore, admission models.ErrandInterruptedAdmission, evidence *journal.RetainedRecoveryResponse, end uint64) error {
	stream := evidence.Descriptor.Stream
	if !evidence.ProcessTerminated || end > math.MaxInt64 || (stream == nil && end != 0) ||
		(stream != nil && stream.HighWater != end) || evidence.Descriptor.SubmissionsTruncated {
		return ErrDurableAdoptionEvidenceUnavailable
	}
	peer := evidence.Submission
	if submissionHasRetainedTerminal(&peer) {
		settler, ok := store.(agentDeliveryTerminalSettler)
		if !ok || peer.TerminalSequence > end {
			return ErrDurableAdoptionEvidenceUnavailable
		}
		_, err := settler.SettleAgentDeliveryTerminal(ctx, peer.StreamID, int64(peer.TerminalSequence), models.DeliverySubmissionState(peer.State), peer.UpdatedAt)
		return err
	}
	if peer.TerminalSequence != 0 || peer.Retired || !isInterruptedErrandSubmission(peer.State) {
		return ErrUncertainPromptDelivery
	}
	_, err := store.CommitOwnedErrandInterruption(ctx, admission, int64(end))
	return err
}

func isInterruptedErrandSubmission(state journal.SubmissionState) bool {
	return state == journal.SubmissionInterruptedUnknown || state == journal.SubmissionAccepted || state == journal.SubmissionDispatching
}
