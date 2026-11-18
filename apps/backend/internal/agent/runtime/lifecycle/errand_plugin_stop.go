package lifecycle

import (
	"context"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *PluginRemoteExecutor) stopErrandInstance(ctx context.Context, row *models.ExecutorRunning, owner agentctltypes.ErrandMCPOwner) error {
	if r.operations == nil || r.profileLoader == nil || row.TransientAuthToken == "" {
		return ErrErrandRecoveryRequiresReconnect
	}
	// Provider operations remain scoped to the task environment that owns
	// the shared resource. The execution inventory keeps its independent owner.
	providerRecord := *row
	providerRecord.TaskID = owner.TaskID
	state, err := r.loadPluginExecutorRecoveryState(ctx, &providerRecord)
	if err != nil {
		return err
	}
	if state == nil || state.resource == nil {
		return ErrErrandRecoveryRequiresReconnect
	}
	if err := validatePluginExecutorResourceForProvider(state.profile.Provider, state.resource); err != nil {
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
