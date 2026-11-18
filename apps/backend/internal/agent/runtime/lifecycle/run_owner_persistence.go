package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

var ErrErrandRecoveryRequiresReconnect = errors.New("errand recovery requires confirmed execution-scoped stop")

const runExecutionOwnerMetadataKey = "execution_owner"

// Runtime inventory and executor correlation use the durable owner's session.
// AgentExecution.SessionID remains exclusively a task-session identity.
func executionInventorySessionID(execution *AgentExecution) string {
	if execution.Owner.Kind == ExecutionOwnerErrand {
		return models.ErrandExecutionIdentity(execution.Owner.ErrandID)
	}
	if execution.Owner.Kind == ExecutionOwnerRun {
		return execution.Owner.RunSessionID
	}
	return execution.SessionID
}

func launchInventorySessionID(req *LaunchRequest) string {
	if req.Owner.Kind == ExecutionOwnerErrand {
		return models.ErrandExecutionIdentity(req.Owner.ErrandID)
	}
	if req.Owner.Kind == ExecutionOwnerRun {
		return req.Owner.RunSessionID
	}
	return req.SessionID
}

func launchProviderTaskID(req *LaunchRequest) string {
	if req.Owner.Kind != ExecutionOwnerErrand {
		return req.TaskID
	}
	switch models.ExecutorType(req.ExecutorType) {
	case models.ExecutorTypeSSH:
		return req.Owner.TaskID
	case models.ExecutorTypeKubernetes:
		return req.EnvironmentOwnerTaskID
	default:
		return req.TaskID
	}
}

// StopRunOwnerForRecovery proves the predecessor has stopped using its durable
// runtime inventory. A missing in-memory execution is never liveness evidence.
func (m *Manager) StopRunOwnerForRecovery(ctx context.Context, owner ExecutionOwner) error {
	if owner.Kind != ExecutionOwnerRun || owner.RunSessionID == "" {
		return errors.New("invalid run recovery owner")
	}
	return m.stopOwnerForRecovery(ctx, owner, owner.RunSessionID, "office_run_recovery")
}

func (m *Manager) stopOwnerForRecovery(ctx context.Context, owner ExecutionOwner, inventoryID, stopReason string) error {
	if err := m.recoveryGuard.CheckLaunchAllowed(inventoryID); err != nil {
		return err
	}
	reader, ok := m.runningWriter.(executorRunningReader)
	if !ok {
		return errors.New("run recovery requires durable runtime inventory")
	}
	row, err := reader.GetExecutorRunningBySessionID(ctx, inventoryID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) || (err == nil && row == nil) {
		return m.runRecoveryErr
	}
	if err != nil {
		return err
	}
	if err := validateRunRecoveryOwner(row, owner); err != nil {
		return err
	}
	if row.Status == models.ExecutorRunningStatusStopped {
		return nil
	}
	backend, err := m.executorRegistry.GetBackend(row.Runtime)
	if err != nil {
		return err
	}
	instance := &ExecutorInstance{
		InstanceID: row.AgentExecutionID, SessionID: row.SessionID,
		StandaloneInstanceID: row.AgentExecutionID, StandalonePort: row.AgentctlPort,
		ContainerID: row.ContainerID, RuntimeName: row.Runtime, Metadata: row.Metadata,
		WorkspacePath: row.WorktreePath, StopReason: stopReason,
	}
	if err := m.stopWorkspaceRuntime(ctx, backend, instance, true); err != nil {
		return fmt.Errorf("stop predecessor run execution: %w", err)
	}
	return m.runningWriter.RepairExecutorRunningDead(ctx, row.SessionID)
}

func validateRunRecoveryOwner(row *models.ExecutorRunning, owner ExecutionOwner) error {
	var recorded ExecutionOwner
	raw, err := json.Marshal(row.Metadata[runExecutionOwnerMetadataKey])
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &recorded); err != nil {
		return err
	}
	if recorded != owner || row.TaskID != "" || row.AgentExecutionID == "" {
		return errors.New("run recovery inventory owner mismatch")
	}
	return nil
}
