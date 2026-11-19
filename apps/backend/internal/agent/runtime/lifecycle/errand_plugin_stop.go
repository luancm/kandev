package lifecycle

import (
	"context"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *PluginRemoteExecutor) stopErrandInstance(ctx context.Context, row *models.ExecutorRunning, owner agentctltypes.ErrandMCPOwner) error {
	state, err := r.loadErrandPluginExecutorState(ctx, row, owner.TaskID)
	if err != nil {
		return err
	}
	resolver := r.connectionResolver(state.operationContext, state.resource, pluginExecutorRuntimePort)
	control, err := agentctl.NewEndpointControlClient(ctx, resolver, r.logger, agentctl.WithControlAuthToken(row.TransientAuthToken))
	if err != nil {
		return err
	}
	defer control.Close()
	return control.StopErrandInstance(ctx, row.AgentExecutionID, owner)
}

func (r *PluginRemoteExecutor) loadErrandPluginExecutorState(ctx context.Context, row *models.ExecutorRunning, taskID string) (*pluginExecutorRecoveryState, error) {
	if r.operations == nil || r.profileLoader == nil || row.TransientAuthToken == "" {
		return nil, ErrErrandRecoveryRequiresReconnect
	}
	providerRecord := *row
	providerRecord.TaskID = taskID
	state, err := r.loadPluginExecutorRecoveryState(ctx, &providerRecord)
	if err != nil {
		return nil, err
	}
	if state == nil || state.resource == nil {
		return nil, ErrErrandRecoveryRequiresReconnect
	}
	if err := validatePluginExecutorResourceForProvider(state.profile.Provider, state.resource); err != nil {
		return nil, err
	}
	return state, nil
}
