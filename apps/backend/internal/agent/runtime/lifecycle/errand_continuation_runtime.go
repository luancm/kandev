package lifecycle

import (
	"context"
	"path/filepath"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/task/models"
)

func (m *Manager) validateColdLaunchRuntime(ctx context.Context, req *LaunchRequest) error {
	retained := launchRequestCarriesRetainedEvidence(req)
	if req.Owner.Kind == ExecutionOwnerErrand && (retained || req.ErrandPredecessor != nil || req.RequiredNativeConversationID != "") {
		return m.validateErrandContinuationRuntime(ctx, req)
	}
	return m.ensureDurableExecutionSelectionCompatible(ctx, req.SessionID, req.TaskEnvironmentID, req.ExecutorType, retained)
}

func (m *Manager) validateErrandContinuationRuntime(ctx context.Context, req *LaunchRequest) error {
	selected, _ := normalizeExecutorSelection(req.ExecutorType)
	refuse := func(detail string) error { return identityUnverifiedError(req.ExecutorType, selected, detail) }
	if !matchesErrandContinuationTarget(req) {
		return refuse("errand continuation predecessor is unverified")
	}
	owner, target := req.Owner, req.ErrandPredecessor
	inventoryID := models.ErrandExecutionIdentity(owner.ErrandID)
	if err := m.recoveryGuard.CheckLaunchAllowed(inventoryID); err != nil {
		return err
	}
	row, available, err := m.readRetainedInventory(ctx, inventoryID)
	if err != nil || !available || row == nil {
		return refuse("errand predecessor inventory is unavailable")
	}
	owner.Generation = target.OwnerGeneration
	if row.AgentExecutionID != target.ExecutionID || !matchesStoppedErrandInventory(row, owner) {
		return refuse("errand predecessor inventory does not match confirmed close")
	}
	if !runtimeSatisfiesSelection(selected, row.Runtime) || !matchesErrandContinuationWorkspace(req, row) {
		return refuse("errand predecessor workspace does not match")
	}
	if err := m.retireOwnedErrandInterruption(ctx, *target); err != nil {
		return err
	}
	req.OriginalWorkspacePath = getMetadataString(row.Metadata, MetadataKeyOriginalWorkspacePath)
	if req.OriginalWorkspacePath == "" {
		req.OriginalWorkspacePath = row.WorktreePath
	}
	return nil
}

func matchesErrandContinuationTarget(req *LaunchRequest) bool {
	owner, target := req.Owner, req.ErrandPredecessor
	return target != nil && target.TaskID == owner.TaskID && target.ErrandID == owner.ErrandID &&
		target.OwnerGeneration > 0 && target.OwnerGeneration == owner.Generation-1 && target.ExecutionID != "" &&
		req.RequiredNativeConversationID != "" && req.RequiredNativeConversationID == req.ACPSessionID
}

func matchesStoppedErrandInventory(row *models.ExecutorRunning, owner ExecutionOwner) bool {
	return row.Status == models.ExecutorRunningStatusStopped &&
		validRecordedErrandOwner(row, owner) && validateRunRecoveryOwner(row, owner) == nil
}

func matchesErrandContinuationWorkspace(req *LaunchRequest, row *models.ExecutorRunning) bool {
	if row.WorktreePath == "" || filepath.Clean(row.WorktreePath) != filepath.Clean(req.WorkspacePath) ||
		getMetadataString(row.Metadata, MetadataKeyTaskEnvironmentID) != req.TaskEnvironmentID {
		return false
	}
	switch row.Runtime {
	case executor.NameDocker, executor.NameRemoteDocker:
		return row.ContainerID != "" && row.ContainerID == getMetadataString(req.Metadata, MetadataKeyContainerID)
	case executor.NameSprites:
		sprite := getMetadataString(row.Metadata, MetadataKeySpriteName)
		return sprite != "" && sprite == getMetadataString(req.Metadata, MetadataKeySpriteName)
	default:
		return true
	}
}
