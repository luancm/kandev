package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func listTaskSessions(t *testing.T, repo *sqliterepo.Repository, taskID string) []*models.TaskSession {
	t.Helper()
	sessions, err := repo.ListTaskSessions(context.Background(), taskID)
	require.NoError(t, err)
	return sessions
}

func errandTestPrimaryID(t *testing.T, repo *sqliterepo.Repository, taskID string) string {
	t.Helper()
	for _, session := range listTaskSessions(t, repo, taskID) {
		if session.IsPrimary {
			return session.ID
		}
	}
	return ""
}

// @covers AC-TASKS-ERRANDS-005.2
func TestErrandNeverPrimarySessionCandidate(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "parent", "")
	seedOwnedErrand(t, repo, "t1", "parent", "errand", models.TaskSessionStateRunning)
	seedOrdinarySession(t, repo, "t1", "sibling", models.TaskSessionStateWaitingForInput, 30*time.Second)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	for _, activeOnly := range []bool{true, false} {
		got := svc.bestPrimarySessionCandidate(ctx, "t1", listTaskSessions(t, repo, "t1"), "parent", activeOnly)
		require.Equal(t, "sibling", got, "a running errand must not outrank an idle ordinary sibling (activeOnly=%t)", activeOnly)
	}
	setSessionState(t, ctx, repo, "sibling", models.TaskSessionStateCompleted)
	for _, activeOnly := range []bool{true, false} {
		got := svc.bestPrimarySessionCandidate(ctx, "t1", listTaskSessions(t, repo, "t1"), "parent", activeOnly)
		require.Empty(t, got, "an errand alone must never be a primary candidate (activeOnly=%t)", activeOnly)
	}
}

// @covers AC-TASKS-ERRANDS-005.2
func TestErrandNotPromotedPrimarySessionAfterRemovalOrStop(t *testing.T) {
	for _, path := range []string{"removal", "stop"} {
		t.Run(path, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "parent", "")
			require.NoError(t, repo.SetSessionPrimary(ctx, "parent"))
			seedOwnedErrand(t, repo, "t1", "parent", "errand", models.TaskSessionStateRunning)
			svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
			promote := func() {
				if path == "removal" {
					svc.promoteNextPrimaryAfterRemoval(ctx, "t1", "parent")
					return
				}
				svc.maybePromotePrimary(ctx, "t1", "parent", models.TaskSessionStateCompleted)
			}

			promote()
			require.Equal(t, "parent", errandTestPrimaryID(t, repo, "t1"), "an errand must not inherit the primary role")

			seedOrdinarySession(t, repo, "t1", "sibling", models.TaskSessionStateWaitingForInput, -time.Minute)
			promote()
			require.Equal(t, "sibling", errandTestPrimaryID(t, repo, "t1"), "an ordinary sibling still inherits the primary role")
		})
	}
}

// @covers AC-TASKS-ERRANDS-005.2
func TestErrandNeverClaimsGeneratedTaskTitle(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "parent", "")
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{
		ID: "errand", TaskID: "t1", ParentSessionID: "parent", Title: "Audit", AgentProfileID: "profile",
		FirstTurnID: "errand-turn", SubmissionID: "prompt:errand", Lifecycle: models.ErrandLifecycleQueued,
		ResultState: models.ErrandResultNone,
	}))
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "t1", models.MetaKeyAgentTitlePending, true))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	owned, err := svc.ClaimTaskTitleSession(ctx, "t1", "errand")
	require.NoError(t, err)
	require.False(t, owned)
	owned, err = svc.ClaimTaskTitleSession(ctx, "t1", "parent")
	require.NoError(t, err)
	require.True(t, owned)
}

// @covers AC-TASKS-ERRANDS-005.3
func TestErrandSkippedByCIAndPullRequestAutomation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "parent", "")
	setSessionState(t, ctx, repo, "parent", models.TaskSessionStateWaitingForInput)
	seedOwnedErrand(t, repo, "t1", "parent", "errand", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	errandSession, err := repo.GetTaskSession(ctx, "errand")
	require.Error(t, err)
	require.Nil(t, errandSession)
	require.True(t, ciAutomationSessionCanReceivePrompt(mustGetSession(t, repo, "parent")))

	errandID := "errand"
	session, err := svc.resolveAutoFixSession(ctx, "t1", &errandID)
	require.NoError(t, err)
	require.Equal(t, "parent", session.ID, "a previous auto-fix pointer at an errand must fall back to the task")
	session, err = svc.resolveAutoFixSession(ctx, "t1", nil)
	require.NoError(t, err)
	require.Equal(t, "parent", session.ID)
	session, err = svc.resolveTaskPRAgentSession(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, "parent", session.ID)
}

// @covers AC-TASKS-ERRANDS-005.3
func TestErrandSkippedByEnsureSessionNewestFallback(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "parent", "")
	seedOwnedErrand(t, repo, "t1", "parent", "errand", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	resp := svc.findExistingSession(context.Background(), "t1")
	require.NotNil(t, resp)
	require.Equal(t, "parent", resp.SessionID)
	require.Equal(t, "existing_newest", resp.Source)
}

// @covers AC-TASKS-ERRANDS-005.3
func TestErrandSkippedByChildCompletionParentPrompt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent-task", "parent", "step1")
	setSessionState(t, ctx, repo, "parent", models.TaskSessionStateWaitingForInput)
	seedOwnedErrand(t, repo, "parent-task", "parent", "errand", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	_, session, ok := svc.parentCompletionContext(ctx, "parent-task")
	require.True(t, ok)
	require.Equal(t, "parent", session.ID, "an on_children_completed prompt must target the parent task's own session")
}

// @covers AC-TASKS-ERRANDS-005.4
func TestOwnedErrandActivityBlocksFeederPromotion(t *testing.T) {
	for _, test := range []struct {
		name    string
		working bool
	}{{"active", true}, {"settled", false}} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step-limited")
			for _, id := range []string{"task-first", "task-second"} {
				require.NoError(t, repo.CreateTask(ctx, &models.Task{
					ID: id, WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step-feeder", Title: id, State: "TODO",
				}))
			}
			seedOrdinarySession(t, repo, "task-first", "first-parent", models.TaskSessionStateWaitingForInput, 0)
			row := &models.Errand{
				ID: "first-errand", TaskID: "task-first", ParentSessionID: "first-parent",
				FirstTurnID: "first-turn", SubmissionID: "first-submission",
				Lifecycle: models.ErrandLifecycleLaunched, ResultState: models.ErrandResultNone,
				AwaitingResult: test.working,
			}
			if !test.working {
				row.LastOutcome = models.ErrandOutcomeDone
				row.LatestSettledTurnID = row.FirstTurnID
			}
			require.NoError(t, repo.CreateErrand(ctx, row))
			if test.working {
				_, err := repo.ReserveErrandExecutionGeneration(ctx, row.ID, 0)
				require.NoError(t, err)
			}
			stepGetter := newMockStepGetter()
			stepGetter.steps["step-limited"] = &wfmodels.WorkflowStep{
				ID: "step-limited", WorkflowID: "wf1", Name: "Limited", Position: 0, WIPLimit: 1, PullFromStepID: "step-feeder",
			}
			stepGetter.steps["step-next"] = &wfmodels.WorkflowStep{ID: "step-next", WorkflowID: "wf1", Name: "Next", Position: 1}
			var movedTaskID string
			store := newWorkflowStore(repo, stepGetter, nil, noopPublisher, testLogger(), &operationLedger{},
				func(_ context.Context, task *models.Task, _, _, _, _ string) { movedTaskID = task.ID })

			require.NoError(t, store.ApplyTransition(ctx, "t1", "s1", "step-limited", "step-next", "on_turn_complete"))

			want := "task-first"
			if test.working {
				want = "task-second"
			}
			require.Equal(t, want, movedTaskID)
		})
	}
}
