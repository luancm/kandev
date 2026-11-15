package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

const errandHistoryFixtureMarker = "KANDEV_E2E_ERRAND_HISTORY_FIXTURE:"

type errandHistoryFixtureTask struct {
	TaskID             string   `json:"task_id"`
	ParentSessionID    string   `json:"parent_session_id"`
	OrdinarySessionIDs []string `json:"ordinary_session_ids"`
	AgentProfileID     string   `json:"agent_profile_id"`
	FinishedCount      int      `json:"finished_count"`
	IncludeOutstanding bool     `json:"include_outstanding"`
}

type errandHistoryFixtureInput struct {
	Tasks []errandHistoryFixtureTask `json:"tasks"`
}

type errandHistoryFixtureResult struct {
	Tasks []errandHistoryFixtureTaskResult `json:"tasks"`
}

type errandHistoryFixtureTaskResult struct {
	TaskID                  string `json:"task_id"`
	FinishedCount           int    `json:"finished_count"`
	OrdinarySessionCount    int    `json:"ordinary_session_count"`
	OutstandingControlCount int    `json:"outstanding_control_count"`
	RecoveryAttention       bool   `json:"recovery_attention"`
	PendingResultState      string `json:"pending_result_state"`
}

// TestErrandHistoryBrowserFixture seeds completed Errand identities directly
// into the managed E2E worker database without starting agent processes.
func TestErrandHistoryBrowserFixture(t *testing.T) {
	root := os.Getenv("KANDEV_E2E_ERRAND_HISTORY_ROOT")
	if root == "" {
		t.Skip("used only by the isolated managed Errand history baseline browser fixture")
	}
	root = validateErrandHistoryFixtureRoot(t, root)
	rawInput := os.Getenv("KANDEV_E2E_ERRAND_HISTORY_TASKS")
	require.NotEmpty(t, rawInput)
	var input errandHistoryFixtureInput
	require.NoError(t, json.Unmarshal([]byte(rawInput), &input))
	require.NotEmpty(t, input.Tasks)

	connection, err := db.OpenSQLite(filepath.Join(root, "kandev.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	sqlConnection := sqlx.NewDb(connection, "sqlite3")
	repo, err := tasksqlite.NewWithDB(sqlConnection, sqlConnection, nil)
	require.NoError(t, err)
	ctx := context.Background()

	result := errandHistoryFixtureResult{Tasks: make([]errandHistoryFixtureTaskResult, 0, len(input.Tasks))}
	for _, seed := range input.Tasks {
		require.NotEmpty(t, seed.TaskID)
		require.NotEmpty(t, seed.ParentSessionID)
		require.Len(t, seed.OrdinarySessionIDs, 2)
		require.GreaterOrEqual(t, seed.FinishedCount, 0)
		require.LessOrEqual(t, seed.FinishedCount, 1000)
		_, err := repo.GetTask(ctx, seed.TaskID)
		require.NoError(t, err, "seed task must already exist in the isolated E2E database")
		existing, err := repo.ListErrandsByTask(ctx, seed.TaskID)
		require.NoError(t, err)
		require.Empty(t, existing, "baseline fixture task must start without Errands")
		sessions, err := repo.ListTaskSessions(ctx, seed.TaskID)
		require.NoError(t, err)
		require.Empty(t, sessions, "baseline fixture task must start without sessions")
		base := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
		createHistorySession(t, ctx, repo, seed.TaskID, seed.ParentSessionID, seed.AgentProfileID, "", "",
			models.TaskSessionStateWaitingForInput, base.Add(-10*time.Minute), nil, true)
		for index, sessionID := range seed.OrdinarySessionIDs {
			require.NotEmpty(t, sessionID)
			createHistorySession(t, ctx, repo, seed.TaskID, sessionID, seed.AgentProfileID, "", "",
				models.TaskSessionStateWaitingForInput, base.Add(-time.Duration(index+8)*time.Minute), nil, false)
		}

		if seed.IncludeOutstanding {
			seedPendingErrand(t, ctx, repo, seed, base)
			seedRecoveryErrand(t, ctx, repo, seed, base)
		}
		for index := 0; index < seed.FinishedCount; index++ {
			seedFinishedErrand(t, ctx, repo, seed, base, index)
		}

		recoveryAttention := false
		pendingState := ""
		controls := 0
		if seed.IncludeOutstanding {
			controls = 2
			var err error
			recoveryAttention, err = repo.ErrandRecoveryAttention(ctx, historyRecoveryID(seed.ParentSessionID))
			require.NoError(t, err)
			pending, err := repo.GetErrand(ctx, historyPendingID(seed.ParentSessionID))
			require.NoError(t, err)
			pendingState = string(pending.ResultState)
		}
		result.Tasks = append(result.Tasks, errandHistoryFixtureTaskResult{
			TaskID: seed.TaskID, FinishedCount: seed.FinishedCount,
			OrdinarySessionCount:    len(seed.OrdinarySessionIDs) + 1,
			OutstandingControlCount: controls, RecoveryAttention: recoveryAttention,
			PendingResultState: pendingState,
		})
	}

	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	fmt.Printf("%s%s\n", errandHistoryFixtureMarker, encoded)
}

func validateErrandHistoryFixtureRoot(t *testing.T, root string) string {
	t.Helper()
	absolute, err := filepath.Abs(root)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(filepath.Base(absolute), "kandev-e2e-"),
		"fixture root must be managed by the isolated E2E backend")
	info, err := os.Stat(absolute)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	return absolute
}

func createHistorySession(
	t *testing.T,
	ctx context.Context,
	repo *tasksqlite.Repository,
	taskID, sessionID, agentProfileID, role, parentID string,
	state models.TaskSessionState,
	startedAt time.Time,
	completedAt *time.Time,
	primary bool,
) {
	t.Helper()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, AgentProfileID: agentProfileID,
		Role: models.SessionRole(role), ParentSessionID: parentID,
		State: state, StartedAt: startedAt, CompletedAt: completedAt,
		UpdatedAt: completedAtOr(startedAt, completedAt), IsPrimary: primary,
	}))
}

func completedAtOr(startedAt time.Time, completedAt *time.Time) time.Time {
	if completedAt != nil {
		return *completedAt
	}
	return startedAt
}

func seedFinishedErrand(
	t *testing.T,
	ctx context.Context,
	repo *tasksqlite.Repository,
	seed errandHistoryFixtureTask,
	base time.Time,
	index int,
) {
	t.Helper()
	id := fmt.Sprintf("%s-history-%04d", seed.ParentSessionID, index)
	createdAt := base.Add(-time.Duration(index) * time.Second)
	completedAt := createdAt.Add(-time.Minute)
	createHistorySession(t, ctx, repo, seed.TaskID, id, seed.AgentProfileID, string(models.SessionRoleErrand),
		seed.ParentSessionID, models.TaskSessionStateCompleted, createdAt.Add(-10*time.Minute), &completedAt, false)
	deliveredAt := completedAt
	row := &models.Errand{
		ID: id, TaskID: seed.TaskID, ParentSessionID: seed.ParentSessionID,
		Title:          fmt.Sprintf("Retained review %04d: deterministic delivered Errand result", index),
		Objective:      strings.Repeat("Review the finished Errand output and retain its parent identity. ", 5),
		AgentProfileID: seed.AgentProfileID,
		Lifecycle:      models.ErrandLifecycleLaunched, LastOutcome: models.ErrandOutcomeDone,
		ResultKind: models.ErrandResultOutput, ResultState: models.ErrandResultDelivered,
		DeliveredRoute: models.ErrandDeliveryNotification, DeliveryToken: "history-delivery-" + id,
		DeliveredAt: &deliveredAt, SettledAt: &completedAt,
		ResultText: strings.Repeat("The retained review completed successfully with a concise, reproducible result. ", 19),
		CreatedAt:  createdAt, UpdatedAt: createdAt,
	}
	require.NoError(t, repo.CreateErrand(ctx, row))
}

func seedPendingErrand(
	t *testing.T,
	ctx context.Context,
	repo *tasksqlite.Repository,
	seed errandHistoryFixtureTask,
	base time.Time,
) {
	t.Helper()
	id := historyPendingID(seed.ParentSessionID)
	completedAt := base.Add(-time.Minute)
	createHistorySession(t, ctx, repo, seed.TaskID, id, seed.AgentProfileID, string(models.SessionRoleErrand),
		seed.ParentSessionID, models.TaskSessionStateCompleted, base.Add(-10*time.Minute), &completedAt, false)
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{
		ID: id, TaskID: seed.TaskID, ParentSessionID: seed.ParentSessionID,
		Title: "Terminal Errand with an outstanding result", Objective: "Keep this result visible while measuring finished history.",
		AgentProfileID: seed.AgentProfileID, Lifecycle: models.ErrandLifecycleLaunched,
		LastOutcome: models.ErrandOutcomeDone, ResultKind: models.ErrandResultOutput,
		ResultState: models.ErrandResultPending, ResultText: "A terminal result still awaits parent delivery.",
		SettledAt: &completedAt, CreatedAt: base.Add(-2 * time.Second), UpdatedAt: base.Add(-2 * time.Second),
	}))
}

func seedRecoveryErrand(
	t *testing.T,
	ctx context.Context,
	repo *tasksqlite.Repository,
	seed errandHistoryFixtureTask,
	base time.Time,
) {
	t.Helper()
	id := historyRecoveryID(seed.ParentSessionID)
	turnID := id + "-turn"
	submissionID := "history-recovery-submission-" + id
	startedAt := base.Add(-5 * time.Minute)
	createHistorySession(t, ctx, repo, seed.TaskID, id, seed.AgentProfileID, string(models.SessionRoleErrand),
		seed.ParentSessionID, models.TaskSessionStateWaitingForInput, startedAt, nil, false)
	row := &models.Errand{
		ID: id, TaskID: seed.TaskID, ParentSessionID: seed.ParentSessionID,
		Title: "Errand recovery needs verification", Objective: "Hold one fixed unresolved recovery case outside finished history.",
		AgentProfileID: seed.AgentProfileID, FirstTurnID: turnID, SubmissionID: submissionID,
		Lifecycle: models.ErrandLifecycleLaunched, LastOutcome: models.ErrandOutcomeDone,
		ResultKind: models.ErrandResultOutput, ResultState: models.ErrandResultSuperseded,
		ResultText: "Prior external effects need verification before this Errand is treated as finished.",
		CreatedAt:  base.Add(-time.Second), UpdatedAt: base.Add(-time.Second),
	}
	require.NoError(t, repo.CreateErrand(ctx, row))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: turnID, TaskID: seed.TaskID, TaskSessionID: id,
		StartedAt: startedAt, CreatedAt: startedAt, UpdatedAt: startedAt,
	}))
	session, err := repo.GetTaskSession(ctx, id)
	require.NoError(t, err)
	effect := &models.AgentDeliveryEffect{
		EffectKey: "history-recovery-settlement-" + id, EffectType: "errand.turn_settlement",
		SessionID: id, IncarnationID: session.QueueIncarnationID, StreamID: "history-recovery-stream-" + id,
		HarnessGeneration: 1, SubmissionID: submissionID, TurnID: turnID, Outcome: "parent", State: "pending",
	}
	require.NoError(t, repo.ReserveErrandTurnSettlement(ctx, seed.TaskID, effect))
	require.NoError(t, repo.CompleteTurn(ctx, turnID))
	session.State = models.TaskSessionStateWaitingForInput
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	session.Metadata[models.SessionMetaKeyAgentDeliveryRecovery] = models.AgentDeliveryRecovery{
		Phase: models.AgentDeliveryRecoveryRecovered, Revision: 1,
		SessionID: id, AgentExecutionID: "baseline-fixture", SubmissionID: submissionID,
		StreamID: effect.StreamID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
		UpdatedAt: base,
	}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, id, models.SessionMetaKeyAgentDeliveryRecovery,
		session.Metadata[models.SessionMetaKeyAgentDeliveryRecovery]))
}

func historyPendingID(parentSessionID string) string {
	return parentSessionID + "-pending-result"
}

func historyRecoveryID(parentSessionID string) string {
	return parentSessionID + "-recovery"
}
