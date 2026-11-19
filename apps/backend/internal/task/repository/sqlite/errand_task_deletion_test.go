package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

func TestErrandTaskDeletionRequiresExactTeardown(t *testing.T) {
	exerciseErrandDeletionRequiresExactTeardown(t, newRepoForSessionTests(t), (*Repository).DeleteTask)
}

func TestPostgresErrandTaskDeletionRequiresExactTeardown(t *testing.T) {
	exerciseErrandDeletionRequiresExactTeardown(t, newPostgresErrandRepo(t), (*Repository).DeleteTask)
}

func TestErrandWorkspaceDeletionRequiresExactTeardown(t *testing.T) {
	exerciseErrandDeletionRequiresExactTeardown(t, newRepoForSessionTests(t), deleteErrandWorkspace)
}

func TestPostgresErrandWorkspaceDeletionRequiresExactTeardown(t *testing.T) {
	exerciseErrandDeletionRequiresExactTeardown(t, newPostgresErrandRepo(t), deleteErrandWorkspace)
}

func deleteErrandWorkspace(repo *Repository, ctx context.Context, taskID string) error {
	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	return repo.DeleteWorkspace(ctx, task.WorkspaceID)
}

func exerciseErrandDeletionRequiresExactTeardown(t *testing.T, repo *Repository, remove func(*Repository, context.Context, string) error) {
	t.Helper()
	ctx := t.Context()
	taskID := newErrandTask(t, repo, "delete-owner")
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: "parent", TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
		StartedAt: errandScenarioBase,
	})
	row := sampleErrand("delete-owner", taskID, "parent", errandScenarioBase)
	mustCreateErrand(t, repo, row)
	first, err := repo.ReserveErrandExecutionGeneration(ctx, row.ID, 0)
	require.NoError(t, err)

	require.ErrorIs(t, remove(repo, ctx, taskID), repoerrors.ErrTaskCleanupInProgress, "an unbound reservation can still allocate an owned runtime")
	_, err = repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, row.ID, first.Generation, "owned-execution")
	require.NoError(t, err)
	require.True(t, bound)

	row.Lifecycle, row.CancelSource = models.ErrandLifecycleCancelled, models.ErrandCancelSourceCascade
	row.AwaitingResult = false
	row.TeardownIntent = &models.ErrandTeardownIntent{
		Version: models.ErrandTeardownIntentVersion, ID: "delete-owner-stop", RequestedAt: errandScenarioBase,
		State: models.ErrandTeardownIntentPending, Purpose: models.ErrandTeardownPurposeCancellation,
		Target: models.ErrandTeardownTarget{TaskID: taskID, ErrandID: row.ID, OwnerGeneration: first.Generation, ExecutionID: "owned-execution", NoTurn: true},
	}
	require.NoError(t, repo.UpdateErrand(ctx, row))
	require.ErrorIs(t, remove(repo, ctx, taskID), repoerrors.ErrTaskCleanupInProgress, "logical cancellation alone must retain exact Stop evidence")
	protected, err := repo.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, row.TeardownIntent, protected.TeardownIntent)
	_, err = repo.TransitionErrandTeardownIntent(ctx, row.ID, row.TeardownIntent.ID, protected.Version, models.ErrandTeardownIntentCompleted)
	require.NoError(t, err)

	require.NoError(t, remove(repo, ctx, taskID))
	_, err = repo.GetErrand(ctx, row.ID)
	require.ErrorIs(t, err, models.ErrErrandNotFound)
}

func TestErrandEphemeralDeletionPreservesUnconfirmedOwners(t *testing.T) {
	exerciseErrandEphemeralDeletionPreservesUnconfirmedOwners(t, newRepoForSessionTests(t))
}

func TestPostgresErrandEphemeralDeletionPreservesUnconfirmedOwners(t *testing.T) {
	exerciseErrandEphemeralDeletionPreservesUnconfirmedOwners(t, newPostgresErrandRepo(t))
}

func exerciseErrandEphemeralDeletionPreservesUnconfirmedOwners(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-ephemeral", Name: "Quick chat"}))
	const profileID = "quick-chat-profile"
	const taskID = "quick-chat-owned"
	seed := func(t *testing.T, id string) {
		t.Helper()
		require.NoError(t, repo.CreateTask(ctx, &models.Task{
			ID: id, WorkspaceID: "ws-ephemeral", IsEphemeral: true, Title: id,
		}))
		createErrandScenarioSession(t, repo, &models.TaskSession{
			ID: "parent-" + id, TaskID: id, State: models.TaskSessionStateWaitingForInput,
			AgentProfileID: profileID, StartedAt: errandScenarioBase,
		})
		row := sampleErrand("errand-"+id, id, "parent-"+id, errandScenarioBase)
		mustCreateErrand(t, repo, row)
		_, err := repo.ReserveErrandExecutionGeneration(ctx, row.ID, 0)
		require.NoError(t, err)
	}
	t.Run("expiration", func(t *testing.T) {
		id := taskID + "-expire"
		seed(t, id)
		deleted, err := repo.DeleteExpiredQuickChatTask(ctx, id, time.Now().Add(time.Hour))
		require.NoError(t, err)
		require.False(t, deleted)
		_, err = repo.GetTask(ctx, id)
		require.NoError(t, err)
	})
	t.Run("profile removal", func(t *testing.T) {
		id := taskID + "-profile"
		seed(t, id)
		deleted, err := repo.DeleteEphemeralTasksByAgentProfile(ctx, profileID)
		require.NoError(t, err)
		require.Zero(t, deleted)
		_, err = repo.GetTask(ctx, id)
		require.NoError(t, err)
	})
}

func TestErrandAdmissionRespectsTaskCleanupBarrier(t *testing.T) {
	exerciseErrandAdmissionRespectsTaskCleanupBarrier(t, newRepoForSessionTests(t))
}

func TestPostgresErrandAdmissionRespectsTaskCleanupBarrier(t *testing.T) {
	exerciseErrandAdmissionRespectsTaskCleanupBarrier(t, newPostgresErrandRepo(t))
}

func exerciseErrandAdmissionRespectsTaskCleanupBarrier(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	taskID := newErrandTask(t, repo, "cleanup-fence")
	row := sampleErrand("cleanup-fence", taskID, "parent", errandScenarioBase)
	mustCreateErrand(t, repo, row)
	first, err := repo.ReserveErrandExecutionGeneration(ctx, row.ID, 0)
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskResourceCleanupJob(ctx, &models.TaskResourceCleanupJob{
		OperationID: "delete:cleanup-fence", TaskID: taskID, Trigger: models.TaskResourceCleanupTriggerDelete,
		State: models.TaskResourceCleanupStatePrepared,
	}))

	bound, err := repo.BindErrandExecution(ctx, row.ID, first.Generation, "late-runtime")
	require.NoError(t, err)
	require.True(t, bound, "the reserved runtime must remain recoverable after cleanup starts")
	binding := &models.ErrandHarnessBinding{
		ErrandID: row.ID, ExecutionID: "late-runtime", Generation: first.Generation, StreamID: "stream",
		IncarnationID: "incarnation", HarnessGeneration: 1,
	}
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	require.NoError(t, repo.CreateErrandTurn(ctx, row.ID, &models.Turn{ID: row.FirstTurnID, TaskID: taskID}))
	payload := []byte(`{"text":"Inspect"}`)
	request := &models.ErrandDeliveryAdmission{
		ErrandID: row.ID, Generation: first.Generation, ExecutionID: binding.ExecutionID,
		IncarnationID: binding.IncarnationID, HarnessGeneration: binding.HarnessGeneration,
		StreamID: binding.StreamID, PromptGeneration: 1, TurnID: row.FirstTurnID,
		Submission: &models.AgentDeliverySubmission{
			ID: row.SubmissionID, Payload: payload, PayloadHash: journal.SubmissionHash(payload),
			DispatchAttemptID: binding.ExecutionID, State: models.DeliverySubmissionDispatching,
		},
	}
	t.Run("dispatch", func(t *testing.T) {
		admitted, err := repo.PrepareErrandDeliverySubmission(ctx, request)
		require.ErrorIs(t, err, repoerrors.ErrTaskCleanupInProgress)
		require.False(t, admitted)
		current, err := repo.GetErrandExecutionDescriptor(ctx, row.ID)
		require.NoError(t, err)
		require.Equal(t, binding.ExecutionID, current.ExecutionID)
		require.Empty(t, current.ActiveSubmissionID)
	})
	t.Run("reserve", func(t *testing.T) {
		_, err := repo.ReserveErrandExecutionGeneration(ctx, row.ID, first.Generation)
		require.ErrorIs(t, err, repoerrors.ErrTaskCleanupInProgress)
		current, err := repo.GetErrandExecutionDescriptor(ctx, row.ID)
		require.NoError(t, err)
		require.Equal(t, first.Generation, current.Generation)
	})
}
