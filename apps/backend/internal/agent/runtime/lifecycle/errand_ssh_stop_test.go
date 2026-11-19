package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentruntime"
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

func TestSSHErrandRecoveryRequiresTheExactControllerAndHelper(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	receipt := testSSHLaunchReceiptForServer(t, server, 4242)
	owner := ExecutionOwner{
		Kind: ExecutionOwnerErrand, TaskID: receipt.Owner.TaskID, WorkspaceID: "workspace",
		ErrandID: "child", Generation: 1, AgentProfileID: "profile",
	}
	receipt.Owner.SessionID = models.ErrandExecutionIdentity(owner.ErrandID)
	receipt.Owner.SessionDir = receipt.Owner.TaskDir + "/sessions/" + receipt.Owner.SessionID
	control := newSSHRecoveryInstanceInfoServer(t, agentctl.InstanceInfo{
		ID: receipt.Owner.ControllerInstanceID, TaskID: owner.TaskID, SessionID: receipt.Owner.SessionID,
		Port: 41234, Status: "running", WorkspacePath: receipt.Owner.TaskDir,
		ErrandMCPOwner: errandMCPOwner(owner), ProviderSessionID: "native-conversation",
	})
	server.forwardTo(control.Listener.Addr().String())
	metadata := sshLaunchStopMetadata(t, server, receipt)
	metadata[MetadataKeySSHRemoteAgentctlPort] = "41234"
	metadata[MetadataKeySSHRemoteControlPort] = "41235"
	metadata[runExecutionOwnerMetadataKey] = owner
	row := &models.ExecutorRunning{
		AgentExecutionID: receipt.Owner.ControllerInstanceID, SessionID: receipt.Owner.SessionID,
		Runtime: agentruntime.RuntimeSSH, ExecutionProfileID: owner.AgentProfileID,
		WorktreePath: receipt.Owner.TaskDir, PID: receipt.Process.PID, TransientAuthToken: "refreshed-auth-token",
		Metadata: metadata,
	}
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	t.Cleanup(func() { require.NoError(t, exec.Close()) })
	exec.verifyRemoteIdentity = func(_ context.Context, client *ssh.Client, pid int, sessionDir, taskDir string) (bool, error) {
		require.NotNil(t, client)
		require.Equal(t, receipt.Process.PID, pid)
		require.Equal(t, receipt.Owner.SessionDir, sessionDir)
		require.Equal(t, receipt.Owner.TaskDir, taskDir)
		return true, nil
	}
	var verified int
	exec.reconcileRemote = func(_ context.Context, client *ssh.Client, request sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		require.NotNil(t, client)
		require.Equal(t, sshlaunch.ReconcileModeVerify, request.Mode)
		require.Equal(t, receipt, request.Receipt)
		verified++
		return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeVerified}
	}

	withoutReceipt := *row
	withoutReceipt.Metadata = cloneSSHMetadata(row.Metadata)
	delete(withoutReceipt.Metadata, MetadataKeySSHLaunchReceipt)
	instances, outcomes, err := exec.RecoverInstancesDetailed(t.Context(), []*models.ExecutorRunning{&withoutReceipt})
	require.NoError(t, err)
	require.Empty(t, instances)
	require.Equal(t, RecoveryOutcomeUnknown, outcomes[row.SessionID])
	require.Zero(t, verified)

	instances, outcomes, err = exec.RecoverInstancesDetailed(t.Context(), []*models.ExecutorRunning{row})
	require.NoError(t, err)
	require.Empty(t, outcomes)
	require.Len(t, instances, 1)
	require.Equal(t, owner, instances[0].recoveredErrandOwner)
	require.Empty(t, instances[0].TaskID)
	require.Equal(t, row.SessionID, instances[0].SessionID)
	require.Equal(t, "native-conversation", instances[0].ProviderSessionID)
	require.Equal(t, 1, verified)
}
