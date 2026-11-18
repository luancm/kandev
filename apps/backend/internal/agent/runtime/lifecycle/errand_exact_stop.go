package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"maps"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
)

type errandInstanceStopper interface {
	stopErrandInstance(context.Context, *models.ExecutorRunning, agentctltypes.ErrandMCPOwner) error
}

// StopErrandOwnerForRecovery closes only the execution captured by the
// Errand teardown intent. It settles inventory after provider confirmation.
func (m *Manager) StopErrandOwnerForRecovery(ctx context.Context, owner ExecutionOwner, executionID string) error {
	if owner.Kind != ExecutionOwnerErrand || owner.ErrandID == "" || owner.Generation == 0 || executionID == "" {
		return errors.New("invalid errand teardown identity")
	}
	inventoryID := models.ErrandExecutionIdentity(owner.ErrandID)
	if err := m.recoveryGuard.CheckLaunchAllowed(inventoryID); err != nil {
		return err
	}
	reader, ok := m.runningWriter.(executorRunningReader)
	if !ok {
		return errors.New("errand teardown requires durable runtime inventory")
	}
	row, err := reader.GetExecutorRunningBySessionID(ctx, inventoryID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) || (err == nil && row == nil) {
		return errors.Join(ErrErrandRecoveryRequiresReconnect, m.runRecoveryErr)
	}
	if err != nil {
		return err
	}
	if row.AgentExecutionID != executionID || !validRecordedErrandOwner(row, owner) {
		return models.ErrExecutionRotated
	}
	if err := validateRunRecoveryOwner(row, owner); err != nil {
		return err
	}
	return m.stopRecordedErrandInstance(ctx, row, owner)
}

func (m *Manager) stopRecordedErrandInstance(ctx context.Context, row *models.ExecutorRunning, owner ExecutionOwner) error {
	cas, ok := m.runningWriter.(executorRunningCASWriter)
	if !ok {
		return errors.New("errand recovery requires conditional inventory repair")
	}
	if m.executorRegistry == nil {
		return ErrErrandRecoveryRequiresReconnect
	}
	backend, err := m.executorRegistry.GetBackend(row.Runtime)
	if err != nil {
		return err
	}
	stopper, ok := backend.(errandInstanceStopper)
	if !ok {
		return ErrErrandRecoveryRequiresReconnect
	}
	record := m.revealRemoteRecoveryCredentials(ctx, row)
	if err := stopper.stopErrandInstance(ctx, record, *errandMCPOwner(owner)); err != nil {
		return fmt.Errorf("stop recorded errand instance: %w", err)
	}
	return cas.RepairExecutorRunningDeadIfCurrent(ctx, row.SessionID, row.AgentExecutionID, row.UpdatedAt)
}

func (r *StandaloneExecutor) stopErrandInstance(ctx context.Context, row *models.ExecutorRunning, owner agentctltypes.ErrandMCPOwner) error {
	control, controlCtx, release, lease, err := r.acquireControl(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := control.StopErrandInstance(controlCtx, row.AgentExecutionID, owner); err != nil {
		return err
	}
	if lease != nil {
		return lease.CheckCurrent()
	}
	return nil
}

func stopAllocatedErrandInstance(ctx context.Context, backend ExecutorBackend, instance *ExecutorInstance, owner agentctltypes.ErrandMCPOwner, launchMetadata map[string]interface{}) error {
	stopper, ok := backend.(errandInstanceStopper)
	if !ok || instance == nil || instance.InstanceID == "" ||
		instance.SessionID != models.ErrandExecutionIdentity(owner.ErrandID) ||
		instance.RuntimeName != backend.Name() {
		return ErrErrandRecoveryRequiresReconnect
	}
	metadata := make(map[string]interface{}, len(launchMetadata)+len(instance.Metadata))
	maps.Copy(metadata, launchMetadata)
	maps.Copy(metadata, instance.Metadata)
	token := instance.AuthToken
	if token == "" && instance.Client != nil {
		token = instance.Client.AuthToken()
	}
	row := &models.ExecutorRunning{
		AgentExecutionID: instance.InstanceID, SessionID: instance.SessionID, Runtime: instance.RuntimeName,
		ContainerID: instance.ContainerID, PID: metadataInt(metadata, MetadataKeySSHRemoteAgentctlPID),
		TransientAuthToken: token, Metadata: metadata,
	}
	return stopper.stopErrandInstance(ctx, row, owner)
}
