package orchestrator

import (
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func prepareReservedErrand(t *testing.T, f *errandLaunchFixture, req errands.LaunchRequest, withTurn bool) {
	t.Helper()
	ctx := executor.WithErrandSession(models.WithReservedLaunchIdentity(t.Context(), models.ReservedLaunchIdentity{
		TaskID: req.TaskID, SessionID: req.ErrandID, TurnID: req.TurnID, SubmissionID: req.SubmissionID,
	}), req.ParentSessionID)
	task, err := f.repo.GetTask(ctx, req.TaskID)
	require.NoError(t, err)
	id, err := f.svc.executor.PrepareSession(ctx, task.ToAPI(), req.AgentProfileID, "", "", "")
	require.NoError(t, err)
	require.Equal(t, req.ErrandID, id)
	if withTurn {
		turn, err := f.svc.turnService.ReserveTurn(ctx, id, nil)
		require.NoError(t, err)
		require.Equal(t, req.TurnID, turn.ID)
	}
}

func reopenLaunchRepository(t *testing.T, f *errandLaunchFixture) {
	t.Helper()
	var sequence int
	var name, path string
	require.NoError(t, f.repo.DB().QueryRow("PRAGMA database_list").Scan(&sequence, &name, &path))
	raw, err := kandevdb.OpenSQLite(path)
	require.NoError(t, err)
	opened := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, opened.Close()) })
	f.repo = sqliterepo.NewWithInitializedDB(opened, opened, nil)
	f.svc.repo = f.repo
	turnBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(turnBus.Close)
	f.svc.SetTurnService(taskservice.NewService(taskservice.Repos{Tasks: f.repo, TaskRepos: f.repo, Workspaces: f.repo,
		Workflows: f.repo, Messages: f.repo, Turns: f.repo, Sessions: f.repo, Executors: f.repo, TaskEnvironments: f.repo,
	}, turnBus, testLogger(), taskservice.RepositoryDiscoveryConfig{}))
}

// @covers AC-TASKS-ERRANDS-001.1 AC-TASKS-ERRANDS-002.1 AC-TASKS-ERRANDS-008.2
func TestErrandLaunchRecoveryAfterPreparedSessionBeforeFirstDispatch(t *testing.T) {
	for _, withTurn := range []bool{false, true} {
		t.Run(boolString(withTurn), func(t *testing.T) {
			f := newErrandLaunchFixture(t)
			req := f.request()
			prepareReservedErrand(t, f, req, withTurn)
			initial, err := models.NewInitialPromptSubmission("ACCEPTED-PROMPT-SNAPSHOT", true, nil)
			require.NoError(t, err)
			require.NoError(t, f.repo.SetSessionMetadataKey(t.Context(), req.ErrandID, models.SessionMetaKeyInitialPromptSubmission, initial))
			var reservedBefore *models.Turn
			if withTurn {
				reservedBefore, err = f.repo.GetTurn(t.Context(), req.TurnID)
				require.NoError(t, err)
			}
			before, err := f.repo.GetTaskSession(t.Context(), req.ErrandID)
			require.NoError(t, err)
			reopenLaunchRepository(t, f)
			admitted := installLaunchAdoptionProvider(t, f)
			fresh := launchRecoveryService(t, f, f.repo)
			require.NoError(t, fresh.Reconcile(t.Context()))
			row, err := f.repo.GetErrand(t.Context(), req.ErrandID)
			require.NoError(t, err)
			require.Equal(t, models.ErrandLifecycleLaunched, row.Lifecycle, "prepared launch must resume its reserved identity")
			require.NoError(t, <-admitted)
			f.svc.ceilingSweepTick(t.Context())
			after, err := f.repo.GetTaskSession(t.Context(), req.ErrandID)
			require.NoError(t, err)
			require.Equal(t, before.QueueIncarnationID, after.QueueIncarnationID)
			require.Equal(t, before.TaskEnvironmentID, after.TaskEnvironmentID)
			require.Equal(t, before.AgentProfileID, after.AgentProfileID)
			turn, err := f.repo.GetTurn(t.Context(), req.TurnID)
			require.NoError(t, err)
			require.Equal(t, req.ErrandID, turn.TaskSessionID)
			if reservedBefore != nil {
				require.Equal(t, reservedBefore.CreatedAt, turn.CreatedAt)
				require.Equal(t, reservedBefore.StartedAt, turn.StartedAt)
			}
			stored, found, err := models.LoadInitialPromptSubmission(after.Metadata)
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, models.InitialPromptSubmissionAccepted, stored.State)
			turns, err := f.repo.ListTurnsBySession(t.Context(), req.ErrandID)
			require.NoError(t, err)
			require.Len(t, turns, 1)
			f.manager.captured.Lock()
			require.Len(t, f.manager.launchedWith, 1)
			require.Equal(t, req.SubmissionID, f.manager.launchedWith[0].InitialDeliverySubmissionID)
			require.Contains(t, f.manager.launchedWith[0].TaskDescription, "ACCEPTED-PROMPT-SNAPSHOT")
			require.NotContains(t, f.manager.launchedWith[0].TaskDescription, req.Objective)
			require.Equal(t, executor.McpModeErrand, f.manager.launchedWith[0].McpMode)
			f.manager.captured.Unlock()
		})
	}
}

// @covers AC-TASKS-ERRANDS-004.8
func TestErrandLaunchRecoveryUncertainSubmissionNeverReplays(t *testing.T) {
	f := newErrandLaunchFixture(t)
	admitted := installLaunchAdoptionProvider(t, f)
	service := launchRecoveryService(t, f, &launchRecordFaultStore{Repository: f.repo, admitted: admitted})
	_, err := service.Start(t.Context(), errands.StartRequest{TaskID: f.taskID, ParentSessionID: f.parent, Title: "response loss", Objective: "only once"})
	require.Error(t, err)
	rows, err := f.repo.ListErrandsByTask(t.Context(), f.taskID)
	require.NoError(t, err)
	row := rows[0]
	changed, err := f.repo.TransitionAgentDeliverySubmission(t.Context(), row.SubmissionID, models.DeliverySubmissionDispatching, models.DeliverySubmissionInterruptedUnknown, "response_lost", row.UpdatedAt)
	require.NoError(t, err)
	require.True(t, changed)
	require.Eventually(t, func() bool {
		current, readErr := f.repo.GetTaskSession(t.Context(), row.ID)
		return readErr == nil && current.State == models.TaskSessionStateRunning
	}, 5*time.Second, 10*time.Millisecond, "join the initial runtime running projection before injecting response loss")
	// The uncertain native response has no exact terminal proof. A failed/idle
	// projection cannot manufacture an interrupted or failed result.
	session, err := f.repo.GetTaskSession(t.Context(), row.ID)
	require.NoError(t, err)
	session.State = models.TaskSessionStateFailed
	require.NoError(t, f.repo.UpdateTaskSession(t.Context(), session))
	fresh := launchRecoveryService(t, f, f.repo)
	require.NoError(t, fresh.Reconcile(t.Context()))
	f.svc.ceilingSweepTick(t.Context())
	require.NoError(t, fresh.Reconcile(t.Context()))
	recovered, err := f.repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleLaunched, recovered.Lifecycle)
	require.True(t, recovered.AwaitingResult)
	require.Equal(t, models.ErrandResultNone, recovered.ResultState)
	require.Empty(t, recovered.LastOutcome)
	assertOneErrandLaunch(t, f)
}

// The prepared positive control proves the effect path is reachable with the
// same canonical rows. Corrupting immutable provenance must block both APIs.
func TestErrandLaunchRecoveryRejectsMismatchedAuthority(t *testing.T) {
	for _, field := range []string{"task_id", "parent_session_id", "first_turn_id", "submission_id", "agent_profile_id"} {
		t.Run(field, func(t *testing.T) {
			f := newErrandLaunchFixture(t)
			req := f.request()
			prepareReservedErrand(t, f, req, true)
			session, err := f.repo.GetTaskSession(t.Context(), req.ErrandID)
			require.NoError(t, err)
			authority := session.Metadata[models.SessionMetaKeyErrandLaunchAuthority].(map[string]interface{})
			authority[field] = "foreign-" + field
			require.NoError(t, f.repo.UpdateSessionMetadata(t.Context(), session.ID, session.Metadata))
			result, err := NewErrandAdapter(f.svc).LaunchErrand(t.Context(), req)
			require.Error(t, err)
			require.Equal(t, errands.LaunchBlocked, result.Disposition)
			_, err = f.svc.LaunchErrandSession(t.Context(), req)
			require.Error(t, err)
			require.Empty(t, f.manager.launchedWith)
			f.manager.mu.Lock()
			require.Empty(t, f.manager.startAgentProcessCalls)
			require.Empty(t, f.manager.capturedPromptCalls)
			require.Empty(t, f.manager.stopAgentWithReasonArgs)
			f.manager.mu.Unlock()
			row, err := f.repo.GetErrand(t.Context(), req.ErrandID)
			require.NoError(t, err)
			require.Equal(t, models.ErrandLifecycleQueued, row.Lifecycle)
			require.True(t, row.AwaitingResult)
		})
	}
}
