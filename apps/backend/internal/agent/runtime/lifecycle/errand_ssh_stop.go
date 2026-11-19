package lifecycle

import (
	"context"
	"fmt"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *SSHExecutor) stopErrandInstance(ctx context.Context, row *models.ExecutorRunning, owner agentctltypes.ErrandMCPOwner) error {
	if row.SessionID != models.ErrandExecutionIdentity(owner.ErrandID) {
		return errSSHLaunchIdentityConflict
	}
	receipt, err := persistedSSHLaunchReceipt(row.Metadata, owner.TaskID, row.SessionID)
	if err != nil {
		return err
	}
	if receipt == nil {
		return errSSHLaunchIdentityMissing
	}
	if receipt.Process.PID != row.PID || receipt.Owner.ControllerInstanceID != row.AgentExecutionID {
		return errSSHLaunchIdentityConflict
	}

	target, err := r.targetFromMetadata(row.Metadata)
	if err != nil {
		return err
	}
	ctx, cancel := sshRemoteCleanupContext(ctx)
	defer cancel()
	client, err := dialSSH(ctx, target)
	if err != nil {
		return err
	}
	defer func() { _ = r.closeSSHClient(client) }()

	result := r.reconcileRecordedController(ctx, client, sshReconcileRequest(*receipt))
	if err := result.Validate(); err != nil {
		return fmt.Errorf("%w: %w", errSSHLaunchReconciliationUnresolved, err)
	}
	if !result.Terminal() {
		return fmt.Errorf("%w: errand controller stop outcome %s", errSSHLaunchReconciliationUnresolved, result.Outcome)
	}
	return nil
}
