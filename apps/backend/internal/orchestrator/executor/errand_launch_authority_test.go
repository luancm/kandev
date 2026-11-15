package executor

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type errandAuthorityFixture struct {
	db    *sqlx.DB
	path  string
	store *tasksqlite.Repository
	exec  *Executor
	agent *mockAgentManager
	task  *v1.Task
}

func newErrandAuthorityFixture(t *testing.T) *errandAuthorityFixture {
	t.Helper()
	database, path := remoteSelectionSQLiteTemplate.Open(t)
	store := tasksqlite.NewWithInitializedDB(database, database, nil)
	ctx := t.Context()
	require.NoError(t, store.CreateWorkspace(ctx, &models.Workspace{ID: "authority-workspace", Name: "Authority"}))
	task := &models.Task{ID: "authority-task", WorkspaceID: "authority-workspace", Title: "Reserved launch"}
	require.NoError(t, store.CreateTask(ctx, task))
	require.NoError(t, store.CreateExecutor(ctx, &models.Executor{ID: "authority-executor", Name: "Local", Type: models.ExecutorTypeLocal}))
	require.NoError(t, store.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "authority-environment", TaskID: task.ID, ExecutorID: "authority-executor",
		ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady, WorkspacePath: t.TempDir(),
	}))
	require.NoError(t, store.CreateTaskSession(ctx, &models.TaskSession{
		ID: "authority-parent", TaskID: task.ID, AgentProfileID: "parent-profile", IsPrimary: true,
		State: models.TaskSessionStateWaitingForInput, TaskEnvironmentID: "authority-environment",
	}))
	agent := &mockAgentManager{}
	return &errandAuthorityFixture{database, path, store, newTestExecutor(t, agent, store), agent, task.ToAPI()}
}

func errandAuthorityContext() context.Context {
	return WithErrandSession(models.WithReservedLaunchIdentity(context.Background(), models.ReservedLaunchIdentity{
		TaskID: "authority-task", SessionID: "authority-child", TurnID: "authority-first-turn", SubmissionID: "authority-submission",
	}), "authority-parent")
}

func (f *errandAuthorityFixture) prepare(ctx context.Context) (string, error) {
	return f.exec.PrepareSession(ctx, f.task, "authority-profile", "authority-executor", "", "")
}

// @covers AC-TASKS-ERRANDS-001.1 AC-TASKS-ERRANDS-001.2 AC-TASKS-ERRANDS-002.1
func TestErrandLaunchAuthorityPersistsWithPreparedSession(t *testing.T) {
	f := newErrandAuthorityFixture(t)
	id, err := f.prepare(errandAuthorityContext())
	require.NoError(t, err)
	session, err := f.store.GetTaskSession(t.Context(), id)
	require.NoError(t, err)
	assertPreparedErrandAuthority(t, session)
	require.Zero(t, f.agent.launchAgentCallCount)
	require.Zero(t, f.agent.promptAgentCallCount)

	// These are the ordinary keyed and full-map metadata write contracts.
	require.NoError(t, f.store.SetSessionMetadataKey(t.Context(), id, "plan_mode", true))
	session, err = f.store.GetTaskSession(t.Context(), id)
	require.NoError(t, err)
	session.Metadata["prepare_result"] = map[string]interface{}{"exit_code": 0}
	require.NoError(t, f.store.UpdateSessionMetadata(t.Context(), id, session.Metadata))
	require.NoError(t, f.db.Close())
	raw, err := kandevdb.OpenSQLite(f.path)
	require.NoError(t, err)
	reopened := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	store := tasksqlite.NewWithInitializedDB(reopened, reopened, nil)
	session, err = store.GetTaskSession(t.Context(), id)
	require.NoError(t, err)
	assertPreparedErrandAuthority(t, session)
	require.Equal(t, true, session.Metadata["plan_mode"])
	require.Contains(t, session.Metadata, "prepare_result")
}

func assertPreparedErrandAuthority(t *testing.T, session *models.TaskSession) {
	t.Helper()
	require.Equal(t, models.SessionRoleErrand, session.Role)
	require.Equal(t, "authority-parent", session.ParentSessionID)
	require.Equal(t, "authority-environment", session.TaskEnvironmentID)
	require.False(t, session.IsPrimary)
	require.NotEmpty(t, session.QueueIncarnationID)
	encoded, err := json.Marshal(session.Metadata["errand_launch_authority"])
	require.NoError(t, err)
	require.JSONEq(t, `{"version":1,"task_id":"authority-task","session_id":"authority-child","parent_session_id":"authority-parent","first_turn_id":"authority-first-turn","submission_id":"authority-submission","agent_profile_id":"authority-profile"}`, string(encoded))
}

// @covers AC-TASKS-ERRANDS-008.2
func TestPrepareSessionErrandAuthorityRejectsIncompleteContext(t *testing.T) {
	for _, field := range []string{"task", "session", "turn", "submission", "parent", "reservation", "foreign task", "blank turn", "padded parent", "self parent"} {
		t.Run(field, func(t *testing.T) {
			f := newErrandAuthorityFixture(t)
			identity := models.ReservedLaunchIdentity{TaskID: f.task.ID, SessionID: "authority-child", TurnID: "authority-first-turn", SubmissionID: "authority-submission"}
			parent := "authority-parent"
			switch field {
			case "task":
				identity.TaskID = ""
			case "session":
				identity.SessionID = ""
			case "turn":
				identity.TurnID = ""
			case "submission":
				identity.SubmissionID = ""
			case "parent":
				parent = ""
			case "foreign task":
				identity.TaskID = "other-task"
			case "blank turn":
				identity.TurnID = " "
			case "padded parent":
				parent = " authority-parent "
			case "self parent":
				parent = identity.SessionID
			}
			ctx := models.WithReservedLaunchIdentity(t.Context(), identity)
			if field == "reservation" {
				ctx = t.Context()
			}
			_, err := f.prepare(WithErrandSession(ctx, parent))
			require.Error(t, err, "partial context must not create an unreserved or unclassified child")
			sessions, err := f.store.ListTaskSessions(t.Context(), f.task.ID)
			require.NoError(t, err)
			require.Len(t, sessions, 1)
			require.Zero(t, f.agent.launchAgentCallCount)
		})
	}
}

// @covers AC-TASKS-ERRANDS-002.1 AC-TASKS-ERRANDS-008.2
func TestPrepareSessionErrandAuthorityInsertedAtomically(t *testing.T) {
	f := newErrandAuthorityFixture(t)
	_, err := f.db.Exec(`CREATE TRIGGER require_launch_authority BEFORE INSERT ON task_sessions
		WHEN NEW.id = 'authority-child' BEGIN
		SELECT CASE WHEN COALESCE(json_extract(NEW.metadata, '$.errand_launch_authority.first_turn_id'), '') != 'authority-first-turn'
		  OR COALESCE(json_extract(NEW.metadata, '$.errand_launch_authority.submission_id'), '') != 'authority-submission'
		  OR NEW.role != 'errand' OR NEW.parent_session_id != 'authority-parent'
		  OR NEW.task_environment_id != 'authority-environment'
		THEN RAISE(ABORT, 'launch authority must exist with role, parent and workspace at insertion') END;
		END`)
	require.NoError(t, err)
	id, err := f.prepare(errandAuthorityContext())
	require.NoError(t, err)
	session, err := f.store.GetTaskSession(t.Context(), id)
	require.NoError(t, err)
	assertPreparedErrandAuthority(t, session)
}

func TestPrepareSessionErrandAuthorityInsertFailureLeavesNoPartialSession(t *testing.T) {
	f := newErrandAuthorityFixture(t)
	_, err := f.db.Exec(`CREATE TRIGGER reject_launch AFTER INSERT ON task_sessions
		WHEN NEW.id = 'authority-child' BEGIN SELECT RAISE(ABORT, 'insertion fault'); END`)
	require.NoError(t, err)
	_, err = f.prepare(errandAuthorityContext())
	require.ErrorContains(t, err, "insertion fault")
	sessions, err := f.store.ListTaskSessions(t.Context(), f.task.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, "authority-parent", sessions[0].ID)
	require.Zero(t, f.agent.launchAgentCallCount)
}

func TestPrepareSessionErrandAuthorityDoesNotRestampLegacySession(t *testing.T) {
	f := newErrandAuthorityFixture(t)
	legacy := &models.TaskSession{ID: "authority-child", TaskID: f.task.ID, ParentSessionID: "authority-parent",
		Role: models.SessionRoleErrand, AgentProfileID: "legacy-profile", State: models.TaskSessionStateCreated,
		TaskEnvironmentID: "authority-environment", Metadata: map[string]interface{}{"legacy-source": "original-evidence"}}
	require.NoError(t, f.store.CreateTaskSession(t.Context(), legacy))
	_, err := f.prepare(errandAuthorityContext())
	require.Error(t, err)
	reloaded, err := f.store.GetTaskSession(t.Context(), legacy.ID)
	require.NoError(t, err)
	require.Equal(t, legacy.QueueIncarnationID, reloaded.QueueIncarnationID)
	require.Equal(t, "legacy-profile", reloaded.AgentProfileID)
	require.Equal(t, map[string]interface{}{"legacy-source": "original-evidence"}, reloaded.Metadata)
	require.Zero(t, f.agent.launchAgentCallCount)
}

func TestPrepareSessionErrandAuthorityIgnoresTaskMetadata(t *testing.T) {
	for _, errand := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "errand"}[errand], func(t *testing.T) {
			f := newErrandAuthorityFixture(t)
			f.task.Metadata = map[string]interface{}{"errand_launch_authority": map[string]interface{}{"session_id": "task-metadata-impostor"}, "independent": "task-setting"}
			ctx := t.Context()
			if errand {
				ctx = errandAuthorityContext()
			}
			id, err := f.prepare(ctx)
			require.NoError(t, err)
			session, err := f.store.GetTaskSession(t.Context(), id)
			require.NoError(t, err)
			if errand {
				assertPreparedErrandAuthority(t, session)
			} else {
				require.NotContains(t, session.Metadata, "errand_launch_authority")
			}
			require.Equal(t, "task-setting", session.Metadata["independent"])
			require.Equal(t, "task-metadata-impostor", f.task.Metadata["errand_launch_authority"].(map[string]interface{})["session_id"])
		})
	}
}
