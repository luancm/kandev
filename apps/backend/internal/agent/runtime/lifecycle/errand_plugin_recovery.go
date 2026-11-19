package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *PluginRemoteExecutor) recoverRetainedPluginErrand(ctx context.Context, row *models.ExecutorRunning, owner ExecutionOwner) (*ExecutorInstance, error) {
	if isTerminalExecutorRunningStatus(row.Status) {
		return nil, nil
	}
	state, err := r.loadErrandPluginExecutorState(ctx, row, owner.TaskID)
	if err != nil {
		return nil, err
	}
	if state.inventory.Phase != pluginExecutorPhaseReady {
		return nil, ErrErrandRecoveryRequiresReconnect
	}
	// A retained Errand lease belongs to one helper on the saved resource.
	// Provider allocation and cleanup belong to the environment owner.
	instance, err := r.newRecoveredPluginExecutorInstance(ctx, row, state)
	if err != nil {
		return nil, err
	}
	instance.recoveredErrandOwner = owner
	instance.ReusedWorkspace = true
	return instance, nil
}
