//go:build linux

package lifecycle

import (
	"github.com/kandev/kandev/internal/common/processidentity"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-003.6
func NewInterruptedDelegationRecoveryFixture(t *testing.T, admitted ...bool) (*Manager, *tasksqlite.Repository, *journal.Journal) {
	t.Helper()
	return newInterruptedDelegationRecoveryFixture(t, len(admitted) > 0 && admitted[0], 2)
}

func NewEarlierInterruptedDelegationRecoveryFixture(t *testing.T) (*Manager, *tasksqlite.Repository, *journal.Journal) {
	t.Helper()
	return newInterruptedDelegationRecoveryFixture(t, false, 1)
}

func newInterruptedDelegationRecoveryFixture(t *testing.T, admitted bool, generation uint64) (*Manager, *tasksqlite.Repository, *journal.Journal) {
	t.Helper()
	manager, execution, _, store := newProjectionWaitFixture(t)
	repo, _ := openRetentionLifecycleRepository(t, filepath.Join(t.TempDir(), "delivery.db"))
	seedProjectionResumeOwner(t, repo)
	execution.TaskID = "task"
	for sequence := int64(1); sequence <= 2; sequence++ {
		_, err := repo.ProjectAgentDeliveryEvent(t.Context(), projectionSQLReceipt(t, repo, sequence), nil)
		require.NoError(t, err)
	}
	now := time.Now().UTC()
	originalStream := "stream"
	if generation < 2 {
		originalStream = "original-stream"
	}
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "parent", TaskID: "task", State: models.TaskSessionStateWaitingForInput}))
	_, err := repo.DB().Exec(`UPDATE task_sessions SET role = 'errand', parent_session_id = 'parent' WHERE id = 'session'`)
	require.NoError(t, err)
	require.NoError(t, repo.CreateErrand(t.Context(), &models.Errand{ID: "session", TaskID: "task", ParentSessionID: "parent", Title: "recovery", Objective: "recover", AgentProfileID: "profile", FirstTurnID: "original-turn", SubmissionID: "interrupted", Lifecycle: models.ErrandLifecycleLaunched, AwaitingResult: true, ResultState: models.ErrandResultNone, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTurn(t.Context(), &models.Turn{ID: "original-turn", TaskID: "task", TaskSessionID: "session", StartedAt: now}))
	require.NoError(t, repo.ReserveErrandTurnSettlement(t.Context(), "task", &models.AgentDeliveryEffect{EffectKey: "original-intent", EffectType: "errand.turn_settlement", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: int64(generation), StreamID: originalStream, SubmissionID: "interrupted", TurnID: "original-turn", Outcome: "parent"}))
	original := journal.Submission{ID: "interrupted", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: generation, StreamID: originalStream, Hash: "hash", Payload: []byte("original instruction"), State: journal.SubmissionInterruptedUnknown}
	_, err = store.PutSubmission(t.Context(), original)
	require.NoError(t, err)
	state := models.DeliverySubmissionInterruptedUnknown
	if admitted {
		state = models.DeliverySubmissionPrepared
	}
	_, err = repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{ID: original.ID, SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: int64(generation), OwnerGeneration: int64(generation), PayloadHash: original.Hash, Payload: original.Payload, State: state})
	require.NoError(t, err)
	if state == models.DeliverySubmissionPrepared {
		changed, err := repo.TransitionAgentDeliverySubmission(t.Context(), original.ID, models.DeliverySubmissionPrepared, models.DeliverySubmissionAccepted, "", now)
		require.NoError(t, err)
		require.True(t, changed)
		changed, err = repo.TransitionAgentDeliverySubmission(t.Context(), original.ID, models.DeliverySubmissionAccepted, models.DeliverySubmissionDispatching, "", now)
		require.NoError(t, err)
		require.True(t, changed)
	}
	child := exec.CommandContext(t.Context(), "/bin/sleep", "60")
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	require.NoError(t, child.Start())
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	birth, err := processidentity.Capture(child.Process.Pid)
	require.NoError(t, err)
	binding := deliveryPromptTurnBinding{OriginalRuntime: birth, TaskID: "task", SessionID: "session", ExecutionID: "original", IncarnationID: "incarnation", HarnessGeneration: generation, PromptGeneration: 3, StreamID: originalStream, SubmissionID: "interrupted", TurnID: "original-turn", NativeSessionID: "native-conversation"}
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "session", TaskID: "task", SessionID: "session", AgentExecutionID: "execution", Status: models.ExecutorRunningStatusRunning, Metadata: map[string]any{deliveryPromptTurnBindingMetadataKey: binding}}))
	require.NoError(t, child.Process.Kill())
	require.Error(t, child.Wait())
	manager.SetExecutorRunningWriter(repo)
	execution.Status = v1.AgentStatusReady
	execution.DeliveryMode = DurableDeliveryV1
	manager.SetAgentDeliveryRepository(repo)
	return manager, repo, store
}

func TestErrandInterruptedDelegationPersistsBeforeReplacementBinding(t *testing.T) {
	manager, repo, _ := NewInterruptedDelegationRecoveryFixture(t)
	_, err := repo.DB().Exec(`UPDATE executors_running SET agent_execution_id = 'original' WHERE session_id = 'session'`)
	require.NoError(t, err)
	execution, exists := manager.GetExecutionBySessionID("session")
	require.True(t, exists)
	require.NoError(t, manager.persistExecutorRunningResult(t.Context(), execution))
	effect, err := repo.GetAgentDeliveryEffect(t.Context(), "errand.interruption:original-intent")
	require.NoError(t, err)
	require.NotNil(t, effect, "replacement persistence must retain original admission proof")
}
