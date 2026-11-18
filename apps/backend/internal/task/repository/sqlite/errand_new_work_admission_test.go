package sqlite

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestErrandNewWorkAdmission(t *testing.T) {
	exerciseErrandNewWorkAdmission(t, newRepoForSessionTests)
}
func TestPostgresErrandNewWorkAdmission(t *testing.T) {
	exerciseErrandNewWorkAdmission(t, newPostgresErrandRepo)
}

func exerciseErrandNewWorkAdmission(t *testing.T, factory func(*testing.T) *Repository) {
	for _, state := range []struct {
		name      string
		cancelled bool
		cleanup   models.ErrandTeardownIntentState
		allowed   bool
	}{
		{name: "queued", allowed: true},
		{name: "cancelled", cancelled: true},
		{name: "cancelled_completed", cancelled: true, cleanup: models.ErrandTeardownIntentCompleted},
		{name: "completion_pending", cleanup: models.ErrandTeardownIntentPending},
		{name: "completion_closed", cleanup: models.ErrandTeardownIntentClosed},
		{name: "completion_completed", cleanup: models.ErrandTeardownIntentCompleted, allowed: true},
		{name: "completion_superseded", cleanup: models.ErrandTeardownIntentSuperseded, allowed: true},
	} {
		for _, dispatch := range []bool{false, true} {
			name := state.name + "/reserve"
			if dispatch {
				name = state.name + "/dispatch"
			}
			t.Run(name, func(t *testing.T) {
				repo := factory(t)
				ctx := context.Background()
				task := newErrandTask(t, repo, "new-work")
				row := sampleErrand("new-work", task, "parent", errandScenarioBase)
				mustCreateErrand(t, repo, row)
				var admission *models.ErrandDeliveryAdmission
				if dispatch {
					_, err := repo.ReserveErrandExecutionGeneration(ctx, row.ID, 0)
					require.NoError(t, err)
					bound, err := repo.BindErrandExecution(ctx, row.ID, 1, "execution")
					require.NoError(t, err)
					require.True(t, bound)
					require.NoError(t, repo.CreateErrandTurn(ctx, row.ID, &models.Turn{ID: "turn", TaskID: task}))
					require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &models.ErrandHarnessBinding{ErrandID: row.ID, Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream"}, 0))
					admission = &models.ErrandDeliveryAdmission{ErrandID: row.ID, Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", PromptGeneration: 1, TurnID: "turn", Submission: &models.AgentDeliverySubmission{ID: "submission", Payload: []byte("prompt"), PayloadHash: journal.SubmissionHash([]byte("prompt"))}}
				}
				if state.cancelled {
					row.Lifecycle = models.ErrandLifecycleCancelled
					row.CancelSource = models.ErrandCancelSource("user")
				}
				intentJSON := ""
				if state.cleanup != "" {
					purpose := models.ErrandTeardownPurposeCompletion
					if state.cancelled {
						purpose = models.ErrandTeardownPurposeCancellation
					}
					row.TeardownIntent = &models.ErrandTeardownIntent{Version: 1, ID: "cleanup", State: state.cleanup, Purpose: purpose, RequestedAt: errandScenarioBase, Target: models.ErrandTeardownTarget{TaskID: task, SessionID: row.ID, SessionIncarnation: "incarnation", ExecutionID: "execution", TurnID: "turn"}}
					require.NoError(t, row.TeardownIntent.ValidateFor(row))
					raw, err := json.Marshal(row.TeardownIntent)
					require.NoError(t, err)
					intentJSON = string(raw)
				}
				_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE task_session_errands SET lifecycle=?,cancel_source=?,teardown_intent=? WHERE id=?`), row.Lifecycle, row.CancelSource, intentJSON, row.ID)
				require.NoError(t, err)
				if dispatch {
					created, err := repo.PrepareErrandDeliverySubmission(ctx, admission)
					if state.allowed {
						require.NoError(t, err)
						require.True(t, created)
						return
					}
					require.Error(t, err)
					require.False(t, created)
					var count int
					require.NoError(t, repo.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM agent_delivery_submissions`))
					require.Zero(t, count)
					require.NoError(t, repo.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM agent_delivery_effects`))
					require.Zero(t, count)
					descriptor, err := repo.GetErrandExecutionDescriptor(ctx, row.ID)
					require.NoError(t, err)
					require.Empty(t, descriptor.ActiveSubmissionID)
					require.Zero(t, descriptor.PromptGeneration)
					return
				}
				descriptor, err := repo.ReserveErrandExecutionGeneration(ctx, row.ID, 0)
				if state.allowed {
					require.NoError(t, err)
					require.EqualValues(t, 1, descriptor.Generation)
					return
				}
				require.Error(t, err)
				require.Nil(t, descriptor)
				var count int
				require.NoError(t, repo.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM errand_execution_generations`))
				require.Zero(t, count)
			})
		}
	}
}

func TestErrandLateBindingAfterCancellation(t *testing.T) {
	exerciseErrandLateBindingAfterCancellation(t, newRepoForSessionTests(t))
}
func TestPostgresErrandLateBindingAfterCancellation(t *testing.T) {
	exerciseErrandLateBindingAfterCancellation(t, newPostgresErrandRepo(t))
}
func exerciseErrandLateBindingAfterCancellation(t *testing.T, repo *Repository) {
	ctx := context.Background()
	task := newErrandTask(t, repo, "late-binding")
	row := sampleErrand("late-binding", task, "parent", errandScenarioBase)
	mustCreateErrand(t, repo, row)
	_, err := repo.ReserveErrandExecutionGeneration(ctx, row.ID, 0)
	require.NoError(t, err)
	row, err = repo.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	row.Lifecycle = models.ErrandLifecycleCancelled
	row.CancelSource = models.ErrandCancelSource("user")
	require.NoError(t, repo.UpdateErrand(ctx, row))
	bound, err := repo.BindErrandExecution(ctx, row.ID, 1, "late-execution")
	require.NoError(t, err)
	require.True(t, bound)
	binding := &models.ErrandHarnessBinding{ErrandID: row.ID, Generation: 1, ExecutionID: "late-execution", IncarnationID: "late-incarnation", HarnessGeneration: 1, StreamID: "late-stream"}
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	require.NoError(t, repo.CompleteErrandHarnessNativeSession(ctx, binding, "late-native"))
	descriptor, err := repo.GetErrandExecutionDescriptor(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, "late-execution", descriptor.ExecutionID)
	_, err = repo.ReserveErrandExecutionGeneration(ctx, row.ID, 1)
	require.Error(t, err)
}
