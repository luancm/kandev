//go:build linux

package sqlite

import (
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestErrandInterruptedDelegationStorage(t *testing.T) {
	exerciseInterruptedDelegationStorage(t, newRepoForSessionTests(t))
}
func TestPostgresErrandInterruptedDelegationStorage(t *testing.T) {
	exerciseInterruptedDelegationStorage(t, newPostgresErrandRepo(t))
}

func exerciseInterruptedDelegationStorage(t *testing.T, repo *Repository) {
	t.Helper()
	row, intent := seedExactSettlement(t, repo)
	require.NoError(t, repo.ReserveErrandTurnSettlement(t.Context(), row.TaskID, intent))
	process := exec.CommandContext(t.Context(), "/bin/sleep", "60")
	process.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	require.NoError(t, process.Start())
	t.Cleanup(func() { _ = process.Process.Kill(); _ = process.Wait() })
	birth, err := processidentity.Capture(process.Process.Pid)
	require.NoError(t, err)
	binding := models.InterruptedDelegationAssessment{OriginalRuntime: birth, TaskID: row.TaskID, SessionID: row.ID, ExecutionID: "original", IncarnationID: intent.IncarnationID, HarnessGeneration: 1, PromptGeneration: 1, StreamID: intent.StreamID, SubmissionID: intent.SubmissionID, TurnID: intent.TurnID, NativeSessionID: "native"}
	running := &models.ExecutorRunning{ID: row.ID, TaskID: row.TaskID, SessionID: row.ID, AgentExecutionID: "replacement", Status: models.ExecutorRunningStatusRunning, Metadata: map[string]any{"delivery_prompt_turn_binding": binding}}
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), running))
	_, err = repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{ID: intent.SubmissionID, SessionID: row.ID, IncarnationID: intent.IncarnationID, HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash", Payload: []byte("instruction"), State: models.DeliverySubmissionInterruptedUnknown})
	require.NoError(t, err)
	require.NoError(t, repo.PreserveErrandInterruptedDelegation(t.Context(), running))
	_, err = repo.GetAgentDeliveryEffect(t.Context(), "errand.interruption:"+intent.EffectKey)
	require.Error(t, err, "live owner must never produce an assessment")
	require.NoError(t, process.Process.Kill())
	require.Error(t, process.Wait())
	require.NoError(t, repo.PreserveErrandInterruptedDelegation(t.Context(), running))
	require.NoError(t, repo.PreserveErrandInterruptedDelegation(t.Context(), running))
	now := time.Now().UTC()
	_, err = repo.CommitHarnessSessionGeneration(t.Context(), &models.HarnessSessionGeneration{SessionID: row.ID, IncarnationID: intent.IncarnationID, Generation: 1, NativeSessionID: "native", CreationReason: "initial", CreatedAt: now, CommittedAt: now}, 0)
	require.NoError(t, err)
	proof := &models.NativeResumeDeliveryEvidence{SessionID: row.ID, IncarnationID: intent.IncarnationID, HarnessGeneration: 1, ExecutionID: "replacement", NativeSessionID: "native", StreamID: intent.StreamID, Submissions: []models.NativeResumeSubmissionEvidence{{ID: intent.SubmissionID, State: models.DeliverySubmissionInterruptedUnknown, HarnessGeneration: 1, StreamID: intent.StreamID, PayloadHash: "hash"}}}
	require.NoError(t, repo.ConsumeErrandInterruptedDelegations(t.Context(), proof))
	require.NoError(t, repo.ConsumeErrandInterruptedDelegations(t.Context(), proof))
	pending, err := repo.PendingErrandTurnSettlements(t.Context(), row.ID)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "parent:interrupted", pending[0].Outcome)
	after, err := repo.GetAgentDeliverySubmission(t.Context(), intent.SubmissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, after.State)
	require.Equal(t, []byte("instruction"), after.Payload)
}
