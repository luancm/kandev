package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type errandExecutionRecoveryStore interface {
	AgentDeliveryRepository
	agentDeliverySubmissionMetadataReader
	GetErrandExecutionDescriptor(context.Context, string) (*models.ErrandExecutionDescriptor, error)
	ErrandHarnessBindingStale(context.Context, string, *models.ErrandHarnessBinding) (bool, error)
}

func (m *Manager) tryRestoreOwnedErrandExecution(ctx context.Context, ri *ExecutorInstance) bool {
	if err := m.restoreOwnedErrandExecution(ctx, ri, nil); err != nil {
		m.logger.Warn("owned errand recovery remains unresolved",
			zap.String("execution_id", ri.InstanceID), zap.String("errand_id", ri.recoveredErrandOwner.ErrandID), zap.Error(err))
		m.discardRemoteRecoveryInstance(ri)
		return false
	}
	m.recoveryGuard.Release(ri.SessionID)
	return true
}

func (m *Manager) restoreOwnedErrandExecution(ctx context.Context, ri *ExecutorInstance, pending *remoteRecoveryRetry) error {
	if ri.Client == nil || ri.TaskID != "" || ri.InstanceID == "" || ri.ProviderSessionID == "" ||
		ri.SessionID != models.ErrandExecutionIdentity(ri.recoveredErrandOwner.ErrandID) {
		return fmt.Errorf("%w: incomplete retained Errand identity", ErrDurableAdoptionIdentityMismatch)
	}
	execution := recoveredErrandExecution(ri)
	if err := m.reDeriveRecoveredAgentIdentity(ctx, execution); err != nil {
		return err
	}
	ownerCurrent, err := m.restoreErrandDelivery(ctx, execution, ri)
	if err != nil {
		return err
	}
	if err := ownerCurrent(); err != nil {
		return err
	}
	if err := m.registerRecoveredErrand(execution, pending); err != nil {
		return err
	}
	if err := m.streamManager.ReplayRecoveredDelivery(ctx, execution); err != nil {
		m.executionStore.Remove(execution.ID)
		return err
	}
	if err := ownerCurrent(); err != nil {
		m.executionStore.Remove(execution.ID)
		return err
	}
	if current, ok := m.executionStore.Get(execution.ID); ok && current == execution {
		if err := m.connectRecoveredErrand(ctx, execution, pending); err != nil {
			m.executionStore.Remove(execution.ID)
			return err
		}
	}
	return nil
}

func (m *Manager) registerRecoveredErrand(execution *AgentExecution, pending *remoteRecoveryRetry) error {
	if pending != nil {
		return m.registerRecoveredRemoteExecution(pending, execution)
	}
	return m.executionStore.Add(execution)
}

func (m *Manager) connectRecoveredErrand(ctx context.Context, execution *AgentExecution, pending *remoteRecoveryRetry) error {
	if pending != nil {
		return m.streamManager.ConnectAllAndWait(ctx, execution)
	}
	m.streamManager.ConnectAll(execution, nil)
	return nil
}

func recoveredErrandExecution(ri *ExecutorInstance) *AgentExecution {
	owner := ri.recoveredErrandOwner
	originalWorkspace := getMetadataString(ri.Metadata, MetadataKeyOriginalWorkspacePath)
	if originalWorkspace == "" {
		originalWorkspace = ri.WorkspacePath
	}
	execution := &AgentExecution{
		ID: ri.InstanceID, Owner: owner, WorkspaceID: owner.WorkspaceID,
		TaskEnvironmentID: getMetadataString(ri.Metadata, MetadataKeyTaskEnvironmentID),
		AgentProfileID:    ri.AgentProfileID, ExecutorType: getMetadataString(ri.Metadata, MetadataKeyExecutorType),
		ContainerID: ri.ContainerID, ContainerIP: ri.ContainerIP,
		WorkspacePath: ri.WorkspacePath, OriginalWorkspacePath: originalWorkspace, WorkspaceSourceRoots: ri.WorkspaceSourceRoots,
		RuntimeName: ri.RuntimeName, Status: v1.AgentStatusRunning, StartedAt: time.Now(), metadata: ri.Metadata,
		agentctl: ri.Client, standaloneInstanceID: ri.StandaloneInstanceID, standalonePort: ri.StandalonePort,
		runtimeEpoch: runtimeEpochFromClient(ri.Client), standaloneHostPID: runtimeProcessIDFromClient(ri.Client),
		attachedExistingCompute: ri.ReusedWorkspace,
		promptDoneCh:            make(chan PromptCompletionSignal, 1), ACPSessionID: ri.ProviderSessionID,
		CopilotNativeSessionEvents: ri.CopilotNativeSessionEvents,
	}
	execution.setSessionInitialized(true)
	execution.setRuntimeEnvironment(ri.Env)
	execution.restoreIdleActivity(ri.Metadata)
	return execution
}

func (m *Manager) restoreErrandDelivery(ctx context.Context, execution *AgentExecution, ri *ExecutorInstance) (func() error, error) {
	status := ri.DeliveryStatus
	if ri.DeliveryRecoveryError != nil {
		return nil, ri.DeliveryRecoveryError
	}
	if status == nil || status.HarnessGeneration > math.MaxInt64 || status.SubmissionsTruncated {
		return nil, fmt.Errorf("%w: incomplete retained Errand delivery evidence", ErrDurableAdoptionBlocked)
	}
	store, ok := m.streamManager.deliveryRepository().(errandExecutionRecoveryStore)
	if !ok {
		return nil, fmt.Errorf("%w: Errand recovery store is unavailable", ErrDurableAdoptionBlocked)
	}
	owner := execution.Owner
	binding := &models.ErrandHarnessBinding{
		ErrandID: owner.ErrandID, Generation: owner.Generation, ExecutionID: execution.ID,
		IncarnationID: status.IncarnationID, HarnessGeneration: int64(status.HarnessGeneration),
		StreamID: status.StreamID, NativeSessionID: execution.ACPSessionID,
	}
	ownerCurrent := func() error {
		stale, err := store.ErrandHarnessBindingStale(ctx, owner.TaskID, binding)
		if err != nil || stale || !ri.Client.RuntimeCurrent() {
			return errors.Join(ErrDurableAdoptionIdentityMismatch, err)
		}
		return nil
	}
	descriptor, err := readCurrentErrandRecoveryDescriptor(ctx, execution, store)
	if err != nil {
		return nil, err
	}
	execution.DeliveryIncarnationID = binding.IncarnationID
	execution.DeliveryHarnessGeneration = status.HarnessGeneration
	execution.DeliveryStreamID = binding.StreamID
	cursor, err := authenticatedDeliveryReplayCursor(ctx, execution, store, status)
	if err != nil {
		return nil, err
	}
	execution.DeliveryReplayCursor = cursor
	execution.DeliveryDescriptor = cloneDeliveryStatus(status)
	execution.DeliveryMode = DurableDeliveryV1
	return ownerCurrent, m.restoreErrandSubmission(ctx, execution, ri.Client, store, descriptor, ownerCurrent)
}

func readCurrentErrandRecoveryDescriptor(ctx context.Context, execution *AgentExecution, store errandExecutionRecoveryStore) (*models.ErrandExecutionDescriptor, error) {
	owner := execution.Owner
	descriptor, err := store.GetErrandExecutionDescriptor(ctx, owner.ErrandID)
	if err != nil {
		return nil, err
	}
	if descriptor == nil || descriptor.ErrandID != owner.ErrandID || descriptor.TaskID != owner.TaskID ||
		descriptor.Generation != owner.Generation || descriptor.ExecutionID != execution.ID {
		return nil, fmt.Errorf("%w: Errand execution generation changed", ErrDurableAdoptionIdentityMismatch)
	}
	return descriptor, nil
}

func (m *Manager) restoreErrandSubmission(
	ctx context.Context, execution *AgentExecution, client *agentctl.Client, store errandExecutionRecoveryStore,
	descriptor *models.ErrandExecutionDescriptor, ownerCurrent func() error,
) error {
	status := execution.DeliveryDescriptor
	id := descriptor.ActiveSubmissionID
	if id == "" {
		if status.SubmissionCount != 0 || status.Stream != nil && execution.DeliveryReplayCursor < status.Stream.HighWater {
			return fmt.Errorf("%w: retained peer has work without an admitted Errand submission", ErrDurableAdoptionBlocked)
		}
		execution.Status = v1.AgentStatusReady
		return nil
	}
	stored, err := store.GetAgentDeliverySubmissionMetadata(ctx, id)
	if err != nil {
		return err
	}
	peer, err := client.GetDeliverySubmission(ctx, id)
	if err != nil {
		return err
	}
	if !matchesRecoveredErrandSubmission(execution, descriptor, stored, peer) {
		return fmt.Errorf("%w: retained Errand submission does not match the admitted turn", ErrDurableAdoptionIdentityMismatch)
	}
	execution.setDeliverySubmissionID(id)
	execution.promptTurnID = descriptor.ActiveTurnID
	execution.promptGeneration = descriptor.PromptGeneration
	execution.dispatchedPromptGeneration = descriptor.PromptGeneration
	execution.promptTurnIDs = map[uint64]string{descriptor.PromptGeneration: descriptor.ActiveTurnID}
	execution.armAcceptedPromptActivity()
	if peer.State != journal.SubmissionAccepted || stored.State != models.DeliverySubmissionDispatching {
		return nil
	}
	if err := ownerCurrent(); err != nil {
		return err
	}
	if m.sessionManager == nil {
		return ErrDurableAdoptionEvidenceUnavailable
	}
	return m.sessionManager.recordErrandPromptAccepted(ctx, execution, stored)
}

func matchesRecoveredErrandSubmission(
	execution *AgentExecution, descriptor *models.ErrandExecutionDescriptor, stored *models.AgentDeliverySubmission,
	peer *journal.Submission,
) bool {
	if !matchesStoredErrandRecoverySubmission(execution, descriptor, stored) {
		return false
	}
	status := execution.DeliveryDescriptor
	if !matchesPeerErrandRecoverySubmission(status, stored, peer) {
		return false
	}
	for _, candidate := range status.Submissions {
		if candidate.ID != peer.ID {
			return false
		}
	}
	switch peer.State {
	case journal.SubmissionAccepted:
		return true
	case journal.SubmissionDispatching:
		// The helper holds dispatching throughout the provider call.
		return stored.State == models.DeliverySubmissionAccepted
	default:
		return status.Stream != nil && peer.TerminalSequence <= status.Stream.HighWater &&
			retainedPeerTerminal(status, peerSubmissionSummary(peer))
	}
}

func matchesPeerErrandRecoverySubmission(status *agentctl.DeliveryStatus, stored *models.AgentDeliverySubmission, peer *journal.Submission) bool {
	return peer != nil && !peer.Retired && peer.ID == stored.ID && peer.SessionID == status.SessionID &&
		peer.IncarnationID == status.IncarnationID && peer.HarnessGeneration == status.HarnessGeneration &&
		peer.StreamID == status.StreamID && peer.Hash == stored.PayloadHash
}

func matchesStoredErrandRecoverySubmission(execution *AgentExecution, descriptor *models.ErrandExecutionDescriptor, stored *models.AgentDeliverySubmission) bool {
	status := execution.DeliveryDescriptor
	return stored != nil && descriptor.ActiveTurnID != "" && descriptor.PromptGeneration != 0 &&
		stored.ID == descriptor.ActiveSubmissionID && stored.DispatchAttemptID == execution.ID &&
		stored.SessionID == status.SessionID && stored.IncarnationID == status.IncarnationID && stored.PayloadHash != "" &&
		stored.HarnessGeneration == int64(status.HarnessGeneration) && stored.OwnerGeneration == stored.HarnessGeneration &&
		deliverySubmissionNeedsRecovery(stored.State)
}
