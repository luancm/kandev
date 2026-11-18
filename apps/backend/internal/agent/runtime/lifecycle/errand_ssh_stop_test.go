package lifecycle

import (
	"context"
	"testing"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/sshlaunch"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestSSHErrandTeardownRequiresExactControllerReceipt(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	receipt := testSSHLaunchReceiptForServer(t, server, 4242)
	receipt.Owner.SessionID = "errand:child"
	receipt.Owner.SessionDir = receipt.Owner.TaskDir + "/sessions/errand:child"
	owner := agentctltypes.ErrandMCPOwner{TaskID: receipt.Owner.TaskID, ErrandID: "child", Generation: 1}
	row := &models.ExecutorRunning{
		AgentExecutionID: receipt.Owner.ControllerInstanceID, SessionID: receipt.Owner.SessionID,
		PID: receipt.Process.PID, Metadata: sshLaunchStopMetadata(t, server, receipt),
	}
	backend := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exact, ok := any(backend).(errandInstanceStopper)
	require.True(t, ok, "SSH teardown must support the owned controller receipt")
	calls := 0
	result := sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeVerified}
	backend.reconcileRemote = func(_ context.Context, client *ssh.Client, request sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		calls++
		require.NotNil(t, client, "stop must use the SSH connection to the persisted target")
		require.Equal(t, sshlaunch.ReconcileModeTerminate, request.Mode)
		require.Equal(t, receipt, request.Receipt)
		return result
	}
	foreign := owner
	foreign.TaskID = "foreign-task"
	require.Error(t, exact.stopErrandInstance(t.Context(), row, foreign))
	withoutReceipt := *row
	withoutReceipt.Metadata = sshLaunchStopMetadata(t, server, receipt)
	delete(withoutReceipt.Metadata, MetadataKeySSHLaunchReceipt)
	require.Error(t, exact.stopErrandInstance(t.Context(), &withoutReceipt, owner))
	require.Zero(t, calls)

	require.Error(t, exact.stopErrandInstance(t.Context(), row, owner), "a verification result cannot settle teardown")
	for _, outcome := range []sshlaunch.ReconcileOutcome{sshlaunch.OutcomeStopped, sshlaunch.OutcomeAbsent} {
		result.Outcome = outcome
		require.NoError(t, exact.stopErrandInstance(t.Context(), row, owner))
	}
	require.Equal(t, 3, calls)
}
