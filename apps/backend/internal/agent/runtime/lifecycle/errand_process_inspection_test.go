//go:build linux

package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// The child alone exhausts descriptors; no process-global failpoint is shared
// with other tests. Metadata serialization occurs immediately before OS proof.
type exhaustOnMarshal struct {
	value any
	fds   *[]int
}

func (v exhaustOnMarshal) MarshalJSON() ([]byte, error) {
	raw, err := json.Marshal(v.value)
	if err != nil {
		return nil, err
	}
	for {
		fd, err := syscall.Open("/dev/null", syscall.O_RDONLY, 0)
		if err != nil {
			break
		}
		*v.fds = append(*v.fds, fd)
	}
	return raw, nil
}

type inspectionFailureReader struct {
	*tasksqlite.Repository
	fds *[]int
}

func (r inspectionFailureReader) GetExecutorRunningBySessionID(ctx context.Context, id string) (*models.ExecutorRunning, error) {
	row, err := r.Repository.GetExecutorRunningBySessionID(ctx, id)
	if err == nil {
		row.Metadata[deliveryPromptTurnBindingMetadataKey] = exhaustOnMarshal{row.Metadata[deliveryPromptTurnBindingMetadataKey], r.fds}
	}
	return row, err
}

func TestErrandInterruptedDelegationInspectionFailurePreservesBinding(t *testing.T) {
	if os.Getenv("KANDEV_TEST_INSPECTION_CHILD") != "1" {
		exe, err := os.Executable()
		require.NoError(t, err)
		cmd := exec.CommandContext(t.Context(), exe, "-test.run=^TestErrandInterruptedDelegationInspectionFailurePreservesBinding$", "-test.v")
		cmd.Env = append(os.Environ(), "KANDEV_TEST_INSPECTION_CHILD=1")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
		return
	}
	manager, repo, journal := NewInterruptedDelegationRecoveryFixture(t)
	_, err := repo.DB().Exec(`UPDATE executors_running SET agent_execution_id = 'original' WHERE session_id = 'session'`)
	require.NoError(t, err)
	before, err := repo.GetExecutorRunningBySessionID(t.Context(), "session")
	require.NoError(t, err)
	raw, err := json.Marshal(before.Metadata[deliveryPromptTurnBindingMetadataKey])
	require.NoError(t, err)
	var binding models.InterruptedDelegationAssessment
	require.NoError(t, json.Unmarshal(raw, &binding))
	dead, err := processidentity.OriginalOwnerTerminated(binding.OriginalRuntime)
	require.NoError(t, err)
	require.True(t, dead)
	var limit syscall.Rlimit
	require.NoError(t, syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit))
	reduced := limit
	reduced.Cur = 256
	require.NoError(t, syscall.Setrlimit(syscall.RLIMIT_NOFILE, &reduced))
	var fds []int
	manager.SetExecutorRunningWriter(inspectionFailureReader{repo, &fds})
	execution, exists := manager.GetExecutionBySessionID("session")
	require.True(t, exists)
	persistErr := manager.persistExecutorRunningResult(t.Context(), execution)
	for _, fd := range fds {
		_ = syscall.Close(fd)
	}
	require.NoError(t, syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit))
	require.ErrorIs(t, persistErr, syscall.EMFILE)
	after, err := repo.GetExecutorRunningBySessionID(t.Context(), "session")
	require.NoError(t, err)
	require.Equal(t, before, after)
	retained, err := journal.GetSubmission(t.Context(), "interrupted")
	require.NoError(t, err)
	require.False(t, retained.Retired)
	manager.SetExecutorRunningWriter(repo)
	require.NoError(t, manager.persistExecutorRunningResult(t.Context(), execution))
	effect, err := repo.GetAgentDeliveryEffect(t.Context(), "errand.interruption:original-intent")
	require.NoError(t, err)
	require.NotNil(t, effect)
}
