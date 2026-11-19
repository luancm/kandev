package errands

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

func TestErrandBrowserSummaryMatchesOwnedScenarios(t *testing.T) {
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "errand-browser.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	repo, err := sqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	exerciseErrandBrowserSummary(t, repo)
}

func TestPostgresErrandBrowserSummaryMatchesOwnedScenarios(t *testing.T) {
	conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	_, err := conn.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`)
	require.NoError(t, err)
	repo, err := sqlite.NewWithDB(conn, conn, nil)
	require.NoError(t, err)
	exerciseErrandBrowserSummary(t, repo)
}

type browserScenario struct {
	id          string
	lifecycle   models.ErrandLifecycle
	outcome     models.ErrandOutcome
	result      models.ErrandResultState
	awaiting    bool
	status      models.ErrandStatus
	attention   models.ErrandAttentionReason
	live        bool
	outstanding bool
	unread      bool
}

func exerciseErrandBrowserSummary(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	ctx := t.Context()
	const taskID, parentID = "task-browser", "parent-browser"
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-browser", Name: "Browser"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "workspace-browser", Title: "Browser"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: parentID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	}))
	rows := []browserScenario{
		{id: "queued", lifecycle: models.ErrandLifecycleQueued, status: models.ErrandStatusQueued, live: true, outstanding: true},
		{id: "active", lifecycle: models.ErrandLifecycleLaunched, status: models.ErrandStatusRunning, live: true},
		{id: "awaiting", lifecycle: models.ErrandLifecycleLaunched, outcome: models.ErrandOutcomeDone, awaiting: true, status: models.ErrandStatusDone, outstanding: true},
		{id: "pending", lifecycle: models.ErrandLifecycleLaunched, outcome: models.ErrandOutcomeDone, result: models.ErrandResultPending, status: models.ErrandStatusDone, outstanding: true},
		{id: "done", lifecycle: models.ErrandLifecycleLaunched, outcome: models.ErrandOutcomeDone, result: models.ErrandResultDelivered, status: models.ErrandStatusDone},
		{id: "failed", lifecycle: models.ErrandLifecycleLaunched, outcome: models.ErrandOutcomeFailed, status: models.ErrandStatusFailed},
		{id: "cancelled", lifecycle: models.ErrandLifecycleCancelled, result: models.ErrandResultDiscarded, status: models.ErrandStatusCancelled},
		{id: "unallocated", lifecycle: models.ErrandLifecycleLaunched, status: models.ErrandStatusFailed},
		{id: "unread", lifecycle: models.ErrandLifecycleLaunched, outcome: models.ErrandOutcomeDone, result: models.ErrandResultDelivered, status: models.ErrandStatusDone, unread: true},
		{id: "permission", lifecycle: models.ErrandLifecycleLaunched, status: models.ErrandStatusNeedsAttention, attention: models.ErrandAttentionPermission, live: true},
		{id: "recovery", lifecycle: models.ErrandLifecycleLaunched, status: models.ErrandStatusNeedsAttention, attention: models.ErrandAttentionRecoveryUnresolved, live: true, outstanding: true},
	}
	base := time.Date(2026, time.November, 13, 10, 0, 0, 0, time.UTC)
	for i, scenario := range rows {
		seedBrowserScenario(t, repo, taskID, parentID, base.Add(time.Duration(i)*time.Minute), scenario)
	}

	summary, err := repo.GetErrandBrowserSummary(ctx, taskID)
	require.NoError(t, err)
	want := models.ErrandBrowserCounts{Live: 4, Queued: 1, Outstanding: 4, Finished: 5, PendingResults: 1, Unread: 1}
	require.Equal(t, want, summary.Counts)
	require.Equal(t, []models.ErrandParentBrowserCounts{{ParentSessionID: parentID, Counts: want}}, summary.ParentCounts)
	assertBrowserScenarioObservations(t, repo, taskID, rows, summary.Errands)
}

func assertBrowserScenarioObservations(t *testing.T, repo *sqlite.Repository, taskID string, rows []browserScenario, summary []*models.ErrandBrowserObservation) {
	t.Helper()
	ctx := t.Context()
	h := newHarness(t)
	h.service.store = repo
	snapshots, err := h.service.List(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, snapshots, len(rows))
	byID := make(map[string]Snapshot, len(snapshots))
	for _, snapshot := range snapshots {
		byID[snapshot.Errand.ID] = snapshot
	}
	visible := make(map[string]bool, len(summary))
	for _, row := range summary {
		visible[row.ID] = true
	}
	for _, row := range rows {
		observed, err := repo.GetErrandInspection(ctx, taskID, row.id)
		require.NoError(t, err)
		require.Equal(t, row.status, observed.Errand.Status, row.id)
		require.Equal(t, row.attention, observed.Errand.AttentionReason, row.id)
		require.Equal(t, row.live, observed.Errand.Live, row.id)
		require.Equal(t, row.outstanding, observed.Errand.Outstanding, row.id)
		require.Equal(t, row.unread, observed.Errand.Unread, row.id)
		require.Equal(t, row.live || row.outstanding, visible[row.id], row.id)
		require.Equal(t, row.status, byID[row.id].Status, row.id)
		require.Equal(t, row.attention, byID[row.id].AttentionReason, row.id)
		projected, err := h.service.ProjectSnapshot(ctx, byID[row.id].Errand)
		require.NoError(t, err)
		require.Equal(t, row.status, projected.Status, row.id)
		require.Equal(t, row.attention, projected.AttentionReason, row.id)
	}
}

func seedBrowserScenario(t *testing.T, repo *sqlite.Repository, taskID, parentID string, at time.Time, scenario browserScenario) {
	t.Helper()
	ctx := t.Context()
	row := &models.Errand{
		ID: scenario.id, TaskID: taskID, ParentSessionID: parentID, Title: scenario.id,
		AgentProfileID: "profile-test", Lifecycle: scenario.lifecycle, LastOutcome: scenario.outcome,
		AwaitingResult: scenario.awaiting, ResultState: scenario.result, CreatedAt: at, UpdatedAt: at,
		FirstTurnID: "turn-" + scenario.id, SubmissionID: "prompt:" + scenario.id,
	}
	if row.ResultState == "" {
		row.ResultState = models.ErrandResultNone
	}
	require.NoError(t, repo.CreateErrand(ctx, row))
	if scenario.unread {
		require.NoError(t, repo.CreateErrandTurn(ctx, row.ID, &models.Turn{ID: row.FirstTurnID}))
		require.NoError(t, repo.CreateErrandMessage(ctx, row.ID, &models.Message{
			ID: "unread-output", TurnID: row.FirstTurnID, AuthorType: models.MessageAuthorAgent, Content: "output",
		}))
	}
	if !scenario.live || row.Lifecycle == models.ErrandLifecycleQueued {
		return
	}
	_, err := repo.ReserveErrandExecutionGeneration(ctx, row.ID, 0)
	require.NoError(t, err)
	if scenario.attention == "" {
		return
	}
	binding := admitOwnedErrandFixture(t, repo, row)
	if scenario.attention == models.ErrandAttentionPermission {
		projectBrowserPermission(t, repo, row, binding)
		return
	}
	changed, err := repo.RecordErrandRecoveryObservation(ctx, models.ErrandRecoveryObservation{
		TaskID: taskID, TurnID: row.FirstTurnID, SubmissionID: row.SubmissionID,
		PromptGeneration: 1, Unresolved: true, Binding: binding,
	})
	require.NoError(t, err)
	require.True(t, changed)
}

func admitOwnedErrandFixture(t *testing.T, repo *sqlite.Repository, row *models.Errand) models.ErrandHarnessBinding {
	t.Helper()
	ctx := t.Context()
	binding := models.ErrandHarnessBinding{
		ErrandID: row.ID, Generation: 1, ExecutionID: row.ID + "-execution", IncarnationID: row.ID + "-incarnation",
		HarnessGeneration: 1, StreamID: row.ID + "-stream", NativeSessionID: row.ID + "-native",
	}
	bound, err := repo.BindErrandExecution(ctx, row.ID, 1, binding.ExecutionID)
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &binding, 0))
	require.NoError(t, repo.CreateErrandTurn(ctx, row.ID, &models.Turn{ID: row.FirstTurnID}))
	prompt := []byte("audit")
	created, err := repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{
		ErrandID: row.ID, Generation: 1, ExecutionID: binding.ExecutionID, IncarnationID: binding.IncarnationID,
		HarnessGeneration: 1, StreamID: binding.StreamID, NativeSessionID: binding.NativeSessionID,
		PromptGeneration: 1, TurnID: row.FirstTurnID,
		Submission: &models.AgentDeliverySubmission{
			ID: row.SubmissionID, State: models.DeliverySubmissionAccepted, DispatchAttemptID: binding.ExecutionID,
			Payload: prompt, PayloadHash: journal.SubmissionHash(prompt),
		},
	})
	require.NoError(t, err)
	require.True(t, created)
	return binding
}

func projectBrowserPermission(t *testing.T, repo *sqlite.Repository, row *models.Errand, binding models.ErrandHarnessBinding) {
	t.Helper()
	ctx := t.Context()
	agent := streams.AgentEvent{
		Type: streams.EventTypePermissionRequest, SessionID: binding.NativeSessionID, RequestID: "request",
		PendingID: "pending", ToolCallID: "tool", PermissionTitle: "Allow?", PermissionHumanInput: true,
		PermissionOptions: []streams.PermissionOption{{OptionID: "allow", Name: "Allow", Kind: streams.PermissionOptionKindAllowOnce}},
		TurnID:            row.FirstTurnID, PromptGeneration: 1,
	}
	payload, err := json.Marshal(agent)
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{
		SessionID: models.ErrandExecutionIdentity(row.ID), TaskID: row.TaskID, ExecutionID: binding.ExecutionID,
		IncarnationID: binding.IncarnationID, HarnessGeneration: 1, OwnerGeneration: 1,
		ExpectedStreamID: binding.StreamID, StreamID: binding.StreamID, Sequence: 1,
		SubmissionID: row.SubmissionID, EventType: agent.Type, Payload: payload,
	}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	projected, err := repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	require.True(t, projected)
}
