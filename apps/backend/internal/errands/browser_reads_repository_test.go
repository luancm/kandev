package errands

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/sessionstate"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

func TestErrandBrowserSummaryMatchesCompleteServiceOracle(t *testing.T) {
	exerciseErrandBrowserSummaryOracle(t, newBrowserOracleSQLiteRepo(t))
}

func TestPostgresErrandBrowserSummaryMatchesCompleteServiceOracle(t *testing.T) {
	conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create kandev_meta: %v", err)
	}
	repo, err := sqlite.NewWithDB(conn, conn, nil)
	if err != nil {
		t.Fatalf("new Postgres repository: %v", err)
	}
	exerciseErrandBrowserSummaryOracle(t, repo)
}

func newBrowserOracleSQLiteRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "errand-browser-oracle.db"))
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	database := sqlx.NewDb(conn, "sqlite3")
	repo, err := sqlite.NewWithDB(database, database, nil)
	if err != nil {
		_ = database.Close()
		t.Fatalf("new SQLite repository: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return repo
}

func exerciseErrandBrowserSummaryOracle(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprintf("oracle-%d", time.Now().UnixNano())
	workspaceID, taskID, parentID := "workspace-"+suffix, "task-"+suffix, "parent-"+suffix
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: suffix}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: suffix}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	base := time.Date(2026, time.November, 13, 10, 0, 0, 0, time.UTC)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: parentID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("create parent: %v", err)
	}

	rows := []browserOracleRow{
		{id: "queued", lifecycle: models.ErrandLifecycleQueued},
		{id: "continuation", lifecycle: models.ErrandLifecycleLaunched, outcome: models.ErrandOutcomeDone, result: models.ErrandResultDelivered, state: models.TaskSessionStateRunning},
		{id: "awaiting", lifecycle: models.ErrandLifecycleLaunched, outcome: models.ErrandOutcomeDone, awaiting: true, state: models.TaskSessionStateCompleted},
		{id: "pending", lifecycle: models.ErrandLifecycleLaunched, outcome: models.ErrandOutcomeDone, result: models.ErrandResultPending, state: models.TaskSessionStateCompleted},
		{id: "done", lifecycle: models.ErrandLifecycleLaunched, outcome: models.ErrandOutcomeDone, result: models.ErrandResultDelivered, state: models.TaskSessionStateCompleted},
		{id: "failed", lifecycle: models.ErrandLifecycleLaunched, outcome: models.ErrandOutcomeFailed, state: models.TaskSessionStateFailed},
		{id: "cancelled", lifecycle: models.ErrandLifecycleCancelled, result: models.ErrandResultDiscarded, state: models.TaskSessionStateCancelled},
		{id: "missing", lifecycle: models.ErrandLifecycleLaunched, outcome: models.ErrandOutcomeDone},
	}
	for _, row := range rows {
		seedBrowserOracleErrand(t, repo, taskID, parentID, base, row)
	}
	permission := seedBrowserOracleErrand(t, repo, taskID, parentID, base.Add(time.Minute), browserOracleRow{
		id: "permission", lifecycle: models.ErrandLifecycleLaunched, state: models.TaskSessionStateRunning,
	})
	permissionTurn := "turn-permission-" + suffix
	createBrowserOracleTurn(t, repo, taskID, permission.ID, permissionTurn, base)
	createBrowserOracleMessage(t, repo, taskID, permission.ID, permissionTurn, "permission-message-"+suffix,
		models.MessageTypePermissionRequest, map[string]interface{}{"status": "pending"}, base)

	clarification := seedBrowserOracleErrand(t, repo, taskID, parentID, base.Add(2*time.Minute), browserOracleRow{
		id: "clarification", lifecycle: models.ErrandLifecycleLaunched, state: models.TaskSessionStateRunning,
	})
	clarificationTurn := "turn-clarification-" + suffix
	createBrowserOracleTurn(t, repo, taskID, clarification.ID, clarificationTurn, base)
	createBrowserOracleMessage(t, repo, taskID, clarification.ID, clarificationTurn, "priority-permission-"+suffix,
		models.MessageTypePermissionRequest, map[string]interface{}{"status": "pending"}, base)
	createBrowserOracleMessage(t, repo, taskID, clarification.ID, clarificationTurn, "priority-clarification-"+suffix,
		models.MessageTypeClarificationRequest, map[string]interface{}{"status": "pending"}, base.Add(time.Second))

	unreadRow := seedBrowserOracleErrand(t, repo, taskID, parentID, base.Add(3*time.Minute), browserOracleRow{
		id: "unread-history", lifecycle: models.ErrandLifecycleLaunched, outcome: models.ErrandOutcomeDone,
		result: models.ErrandResultDelivered,
	})
	unreadTurn := "turn-unread-" + suffix
	if err := repo.CreateErrandTurn(ctx, unreadRow.ID, &models.Turn{ID: unreadTurn}); err != nil {
		t.Fatalf("create unread Errand turn: %v", err)
	}
	if err := repo.CreateErrandMessage(ctx, unreadRow.ID, &models.Message{
		ID: "unread-message-" + suffix, TurnID: unreadTurn, AuthorType: models.MessageAuthorAgent,
		Content: "output", CreatedAt: base.Add(4 * time.Minute),
	}); err != nil {
		t.Fatalf("create unread Errand output: %v", err)
	}

	recovery := seedBrowserOracleRecovery(t, repo, taskID, parentID, suffix, base.Add(5*time.Minute))

	summary, err := repo.GetErrandBrowserSummary(ctx, taskID)
	if err != nil {
		t.Fatalf("read browser summary: %v", err)
	}
	observed := make(map[string]*models.ErrandBrowserObservation, len(summary.Errands))
	for _, row := range summary.Errands {
		observed[row.ID] = row
	}
	complete, err := repo.ListErrandsByTask(ctx, taskID)
	if err != nil {
		t.Fatalf("read complete errand list: %v", err)
	}
	working := make([]string, 0)
	sessions := make(map[string]*models.TaskSession, len(complete))
	for _, row := range complete {
		if row.Lifecycle != models.ErrandLifecycleLaunched {
			continue
		}
		session, err := repo.GetTaskSession(ctx, row.ID)
		if errors.Is(err, models.ErrTaskSessionNotFound) {
			continue
		}
		if err != nil {
			t.Fatalf("read session %s: %v", row.ID, err)
		}
		sessions[row.ID] = session
		if sessionstate.IsWorking(session.State) {
			working = append(working, row.ID)
		}
	}
	pendingActions, err := repo.GetPendingActionsBySessionIDs(ctx, working)
	if err != nil {
		t.Fatalf("read current pending actions: %v", err)
	}
	want := models.ErrandBrowserCounts{}
	for _, row := range complete {
		session := sessions[row.ID]
		permissionPending := session != nil && sessionstate.IsWorking(session.State) && pendingActions[row.ID] == models.TaskPendingActionPermission
		recoveryAttention := RecoveryAttention{}
		if row.Lifecycle == models.ErrandLifecycleLaunched && session != nil && !sessionstate.IsWorking(session.State) && session.State != models.TaskSessionStateFailed {
			recoveryAttention.Unresolved, err = repo.ErrandRecoveryAttention(ctx, row.ID)
			if err != nil {
				t.Fatalf("read recovery attention %s: %v", row.ID, err)
			}
		}
		status := ProjectStatus(row, session, permissionPending, recoveryAttention)
		live, outstanding := IsLive(status), IsOutstanding(row, recoveryAttention)
		isUnread := row.ID == unreadRow.ID
		actual := observed[row.ID]
		if live || outstanding {
			want := expectedErrandBrowserObservation(status, live, outstanding, isUnread, permissionPending, recoveryAttention)
			assertErrandBrowserObservation(t, row.ID, actual, want)
		} else if actual != nil {
			t.Errorf("summary included finished errand %s with status %s", row.ID, actual.Status)
		}
		if live {
			want.Live++
		}
		if row.Lifecycle == models.ErrandLifecycleQueued {
			want.Queued++
		}
		if outstanding {
			want.Outstanding++
		}
		if !live && !outstanding {
			want.Finished++
		}
		if row.ResultState == models.ErrandResultPending {
			want.PendingResults++
		}
		if isUnread {
			want.Unread++
		}
	}
	if summary.Counts != want {
		t.Errorf("summary counts = %+v; complete-state oracle wants %+v", summary.Counts, want)
	}
	if len(complete) != len(rows)+4 {
		t.Errorf("complete list has %d errands, want %d including all unloaded rows", len(complete), len(rows)+4)
	}
	if observed[recovery.ID].AttentionReason != models.ErrandAttentionRecoveryUnresolved {
		t.Errorf("recovery row is absent or lacks unresolved attention: %+v", observed[recovery.ID])
	}
	parentCounts := summary.ParentCounts
	if len(parentCounts) != 1 || parentCounts[0].Counts != want {
		t.Errorf("parent counts = %+v; want one parent with %+v", parentCounts, want)
	}
}

type expectedErrandBrowserRow struct {
	status          models.ErrandStatus
	live            bool
	outstanding     bool
	unread          bool
	attentionReason models.ErrandAttentionReason
}

func expectedErrandBrowserObservation(status models.ErrandStatus, live, outstanding, unread, permissionPending bool, recovery RecoveryAttention) expectedErrandBrowserRow {
	reason := models.ErrandAttentionReason("")
	if status == models.ErrandStatusNeedsAttention {
		if permissionPending {
			reason = models.ErrandAttentionPermission
		} else if recovery.Unresolved {
			reason = models.ErrandAttentionRecoveryUnresolved
		}
	}
	return expectedErrandBrowserRow{
		status: status, live: live, outstanding: outstanding, unread: unread,
		attentionReason: models.ProjectErrandAttentionReason(status, reason),
	}
}

func assertErrandBrowserObservation(t *testing.T, id string, actual *models.ErrandBrowserObservation, want expectedErrandBrowserRow) {
	t.Helper()
	if actual == nil {
		t.Fatalf("summary omitted live/outstanding errand %s (%s)", id, want.status)
		return
	}
	if actual.Status != want.status || actual.Live != want.live || actual.Outstanding != want.outstanding || actual.Unread != want.unread {
		t.Errorf("summary %s = status %s live %t outstanding %t unread %t; oracle wants %s %t %t %t",
			id, actual.Status, actual.Live, actual.Outstanding, actual.Unread, want.status, want.live, want.outstanding, want.unread)
	}
	if actual.AttentionReason != want.attentionReason {
		t.Errorf("summary %s attention reason = %q; oracle wants %q", id, actual.AttentionReason, want.attentionReason)
	}
}

type browserOracleRow struct {
	id           string
	lifecycle    models.ErrandLifecycle
	outcome      models.ErrandOutcome
	awaiting     bool
	result       models.ErrandResultState
	state        models.TaskSessionState
	firstTurnID  string
	submissionID string
}

func seedBrowserOracleErrand(
	t *testing.T,
	repo *sqlite.Repository,
	taskID, parentID string,
	createdAt time.Time,
	scenario browserOracleRow,
) *models.Errand {
	t.Helper()
	if scenario.state != "" {
		if err := repo.CreateTaskSession(t.Context(), &models.TaskSession{
			ID: "errand-" + scenario.id, TaskID: taskID, ParentSessionID: parentID,
			Role: models.SessionRoleErrand, State: scenario.state,
		}); err != nil {
			t.Fatalf("create errand session %s: %v", scenario.id, err)
		}
	}
	row := &models.Errand{
		ID: "errand-" + scenario.id, TaskID: taskID, ParentSessionID: parentID,
		Title: scenario.id, AgentProfileID: "profile-test", Lifecycle: scenario.lifecycle,
		LastOutcome: scenario.outcome, AwaitingResult: scenario.awaiting, ResultState: scenario.result,
		FirstTurnID: scenario.firstTurnID, SubmissionID: scenario.submissionID,
		CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	if row.ResultState == "" {
		row.ResultState = models.ErrandResultNone
	}
	if err := repo.CreateErrand(t.Context(), row); err != nil {
		t.Fatalf("create errand %s: %v", row.ID, err)
	}
	return row
}

func createBrowserOracleTurn(t *testing.T, repo *sqlite.Repository, taskID, sessionID, turnID string, startedAt time.Time) {
	t.Helper()
	if err := repo.CreateTurn(t.Context(), &models.Turn{ID: turnID, TaskID: taskID, TaskSessionID: sessionID, StartedAt: startedAt}); err != nil {
		t.Fatalf("create turn %s: %v", turnID, err)
	}
}

func createBrowserOracleMessage(
	t *testing.T,
	repo *sqlite.Repository,
	taskID, sessionID, turnID, messageID string,
	kind models.MessageType,
	metadata map[string]interface{},
	createdAt time.Time,
) {
	t.Helper()
	if err := repo.CreateMessage(t.Context(), &models.Message{
		ID: messageID, TaskID: taskID, TaskSessionID: sessionID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent, Type: kind, Metadata: metadata, Content: messageID, CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("create message %s: %v", messageID, err)
	}
}

func seedBrowserOracleRecovery(t *testing.T, repo *sqlite.Repository, taskID, parentID, suffix string, at time.Time) *models.Errand {
	t.Helper()
	firstTurnID, submissionID := "recovery-turn-"+suffix, "recovery-submission-"+suffix
	row := seedBrowserOracleErrand(t, repo, taskID, parentID, at, browserOracleRow{
		id: "recovery-" + suffix, lifecycle: models.ErrandLifecycleLaunched,
		result: models.ErrandResultNone, state: models.TaskSessionStateRunning,
		firstTurnID: firstTurnID, submissionID: submissionID,
	})
	createBrowserOracleTurn(t, repo, taskID, row.ID, row.FirstTurnID, at)
	session, err := repo.GetTaskSession(t.Context(), row.ID)
	if err != nil {
		t.Fatalf("read recovery session: %v", err)
	}
	effect := &models.AgentDeliveryEffect{
		EffectKey: "recovery-effect-" + suffix, EffectType: "errand.turn_settlement", SessionID: row.ID,
		IncarnationID: session.QueueIncarnationID, StreamID: "recovery-stream-" + suffix,
		HarnessGeneration: 1, SubmissionID: row.SubmissionID, TurnID: row.FirstTurnID,
		Outcome: "parent", State: "pending",
	}
	if err := repo.ReserveErrandTurnSettlement(t.Context(), taskID, effect); err != nil {
		t.Fatalf("reserve recovery settlement: %v", err)
	}
	if err := repo.CompleteTurn(t.Context(), row.FirstTurnID); err != nil {
		t.Fatalf("complete recovery turn: %v", err)
	}
	if err := repo.UpdateTaskSessionState(t.Context(), row.ID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("idle recovery session: %v", err)
	}
	recovery := models.AgentDeliveryRecovery{
		Phase: models.AgentDeliveryRecoveryRecovered, Revision: 1, SessionID: row.ID,
		IncarnationID: session.QueueIncarnationID, SubmissionID: row.SubmissionID, HarnessGeneration: 1,
		AgentExecutionID: "recovery-execution-" + suffix, StreamID: effect.StreamID,
	}
	if err := repo.SetSessionMetadataKey(t.Context(), row.ID, models.SessionMetaKeyAgentDeliveryRecovery, recovery); err != nil {
		t.Fatalf("set recovery metadata: %v", err)
	}
	row, err = repo.GetErrand(t.Context(), row.ID)
	if err != nil {
		t.Fatalf("reload recovery errand: %v", err)
	}
	row.LastOutcome, row.AwaitingResult = models.ErrandOutcomeDone, false
	if err := repo.UpdateErrand(t.Context(), row); err != nil {
		t.Fatalf("update recovery result: %v", err)
	}
	return row
}
