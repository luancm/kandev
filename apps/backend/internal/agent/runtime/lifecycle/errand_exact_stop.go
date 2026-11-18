package lifecycle

import (
	"context"
	"errors"
	"fmt"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
)

type errandInstanceStopper interface {
	stopErrandInstance(context.Context, *models.ExecutorRunning, agentctltypes.ErrandMCPOwner) error
}

func (m *Manager) stopRecordedErrandInstance(ctx context.Context, row *models.ExecutorRunning, owner ExecutionOwner) error {
	cas, ok := m.runningWriter.(executorRunningCASWriter)
	if !ok {
		return errors.New("errand recovery requires conditional inventory repair")
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
