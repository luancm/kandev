package lifecycle

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func supportsOwnedRemoteRecovery(row *models.ExecutorRunning) bool {
	return row != nil && row.Runtime == agentruntime.RuntimeRemoteDocker && IsErrandRuntimeInventory(row)
}

func (m *Manager) retryOwnedRemoteRecovery(ctx context.Context, pending *remoteRecoveryRetry) {
	if m.retryPendingRemoteStop(ctx, pending) || m.executorRegistry == nil {
		return
	}
	row, _, err := m.currentOwnedRemoteRecoveryRecord(ctx, pending)
	if err != nil {
		m.logOwnedRemoteRecoveryBlocked(pending, err)
		return
	}
	if isTerminalExecutorRunningStatus(row.Status) {
		return
	}
	backend, err := m.executorRegistry.GetBackend(row.Runtime)
	if err != nil {
		m.logOwnedRemoteRecoveryBlocked(pending, err)
		return
	}
	ri, _, err := m.recoverExactRemoteInstance(ctx, backend, m.revealRemoteRecoveryCredentials(ctx, row))
	if err != nil || ri == nil {
		if err != nil {
			m.logOwnedRemoteRecoveryBlocked(pending, err)
		}
		return
	}
	if err := m.processRecoveredOwnedRemoteInstance(ctx, pending, ri); err != nil {
		m.discardRemoteRecoveryInstance(ri)
		m.logOwnedRemoteRecoveryBlocked(pending, err)
	}
}

func (m *Manager) currentOwnedRemoteRecoveryRecord(ctx context.Context, pending *remoteRecoveryRetry) (*models.ExecutorRunning, ExecutionOwner, error) {
	if _, ok := m.runningWriter.(executorRunningReader); !ok {
		return nil, ExecutionOwner{}, ErrDurableAdoptionEvidenceUnavailable
	}
	owner, err := recordedErrandExecutionOwner(pending.record)
	if err != nil {
		return nil, owner, err
	}
	row, present, err := m.remoteRecoveryAttemptRecord(ctx, pending)
	if err != nil {
		return nil, owner, err
	}
	if !present || !validRecordedErrandOwner(row, owner) || validateRunRecoveryOwner(row, owner) != nil {
		return nil, owner, errRemoteRecoveryFenced
	}
	return row, owner, nil
}

func (m *Manager) processRecoveredOwnedRemoteInstance(ctx context.Context, pending *remoteRecoveryRetry, ri *ExecutorInstance) error {
	row, owner, err := m.currentOwnedRemoteRecoveryRecord(ctx, pending)
	if err != nil {
		return err
	}
	if isTerminalExecutorRunningStatus(row.Status) || ri == nil || ri.Client == nil ||
		ri.InstanceID != row.AgentExecutionID || ri.RuntimeName != row.Runtime ||
		ri.recoveredErrandOwner != owner || ri.AgentProfileID != row.ExecutionProfileID ||
		ri.ContainerID != row.ContainerID || (row.WorktreePath != "" && ri.WorkspacePath != row.WorktreePath) {
		return errRemoteRecoveryFenced
	}
	ri.DeliveryStatus, err = ri.Client.GetDeliveryStatus(ctx, "")
	if err != nil {
		return err
	}
	if err := m.restoreOwnedErrandExecution(ctx, ri, pending); err != nil {
		return err
	}
	m.completeRemoteRecovery(pending)
	return nil
}

func (m *Manager) logOwnedRemoteRecoveryBlocked(pending *remoteRecoveryRetry, err error) {
	m.logger.Debug("owned remote errand recovery remains unresolved",
		zap.String("execution_id", pending.record.AgentExecutionID), zap.Error(err))
}
