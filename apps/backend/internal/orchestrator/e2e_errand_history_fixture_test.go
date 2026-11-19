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
	"github.com/kandev/kandev/internal/agent/executor"
	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
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
		task, err := repo.GetTask(ctx, seed.TaskID)
		require.NoError(t, err, "seed task must already exist in the isolated E2E database")
		existing, err := repo.ListErrandsByTask(ctx, seed.TaskID)
		require.NoError(t, err)
		require.Empty(t, existing, "baseline fixture task must start without Errands")
		sessions, err := repo.ListTaskSessions(ctx, seed.TaskID)
		require.NoError(t, err)
		require.Empty(t, sessions, "baseline fixture task must start without sessions")
		base := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
		createHistorySession(t, ctx, repo, seed.TaskID, seed.ParentSessionID, seed.AgentProfileID,
			models.TaskSessionStateWaitingForInput, base.Add(-10*time.Minute), nil, true)
		for index, sessionID := range seed.OrdinarySessionIDs {
			require.NotEmpty(t, sessionID)
			createHistorySession(t, ctx, repo, seed.TaskID, sessionID, seed.AgentProfileID,
				models.TaskSessionStateWaitingForInput, base.Add(-time.Duration(index+8)*time.Minute), nil, false)
		}

		seedFinishedErrands(t, ctx, repo, seed, base)
		if seed.IncludeOutstanding {
			seedPendingErrand(t, ctx, repo, seed, base)
			seedRecoveryErrand(t, ctx, repo, seed, task.WorkspaceID, base)
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
	taskID, sessionID, agentProfileID string,
	state models.TaskSessionState,
	startedAt time.Time,
	completedAt *time.Time,
	primary bool,
) {
	t.Helper()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, AgentProfileID: agentProfileID,
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

func seedFinishedErrands(t *testing.T, ctx context.Context, repo *tasksqlite.Repository, seed errandHistoryFixtureTask, base time.Time) {
	t.Helper()
	if seed.FinishedCount == 0 {
		return
	}
	rows := make([]*models.Errand, 0, seed.FinishedCount)
	for index := 0; index < seed.FinishedCount; index++ {
		rows = append(rows, seedFinishedErrand(t, ctx, repo, seed, base, index))
	}
	candidates := models.ErrandInventory(rows)
	claimed, err := repo.ClaimResults(ctx, seed.TaskID, seed.ParentSessionID, models.ErrandDeliveryNotification,
		"history-delivery-"+seed.ParentSessionID, candidates, base, candidates)
	require.NoError(t, err)
	require.Len(t, claimed, seed.FinishedCount)
}

func seedFinishedErrand(t *testing.T, ctx context.Context, repo *tasksqlite.Repository, seed errandHistoryFixtureTask, base time.Time, index int) *models.Errand {
	t.Helper()
	id := fmt.Sprintf("%s-history-%04d", seed.ParentSessionID, index)
	createdAt := base.Add(-time.Duration(index) * time.Second)
	completedAt := createdAt.Add(-time.Minute)
	row := &models.Errand{
		ID: id, TaskID: seed.TaskID, ParentSessionID: seed.ParentSessionID,
		Title:          fmt.Sprintf("Retained review %04d: deterministic delivered Errand result", index),
		Objective:      strings.Repeat("Review the finished Errand output and retain its parent identity. ", 5),
		AgentProfileID: seed.AgentProfileID, FirstTurnID: id + "-turn", SubmissionID: id + "-submission",
		Lifecycle: models.ErrandLifecycleLaunched, ResultState: models.ErrandResultNone, AwaitingResult: true,
		CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	require.NoError(t, repo.CreateErrand(ctx, row))
	binding := seedHistoryErrandExecution(t, ctx, repo, row, createdAt.Add(-10*time.Minute))
	target := seedHistoryErrandTerminal(t, ctx, repo, row, binding, completedAt)
	row.LatestSettledTurnID, row.ResultTurnID = target.TurnID, target.TurnID
	row.AwaitingResult, row.LastOutcome, row.SettledAt = false, models.ErrandOutcomeDone, &completedAt
	row.ResultState, row.ResultKind = models.ErrandResultPending, models.ErrandResultOutput
	row.ResultText = strings.Repeat("The retained review completed successfully with a concise, reproducible result. ", 19)
	row.TeardownIntent = &models.ErrandTeardownIntent{
		Version: models.ErrandTeardownIntentVersion, ID: id + "-close", RequestedAt: completedAt,
		State: models.ErrandTeardownIntentPending, Purpose: models.ErrandTeardownPurposeCompletion, Target: target,
	}
	require.NoError(t, repo.UpdateErrand(ctx, row))
	closed, err := repo.TransitionErrandTeardownIntent(ctx, id, row.TeardownIntent.ID, row.Version, models.ErrandTeardownIntentCompleted)
	require.NoError(t, err)
	return closed
}

func seedHistoryErrandTerminal(t *testing.T, ctx context.Context, repo *tasksqlite.Repository, row *models.Errand, binding models.ErrandHarnessBinding, completedAt time.Time) models.ErrandTeardownTarget {
	t.Helper()
	payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeComplete, SessionID: binding.NativeSessionID, TurnID: row.FirstTurnID, PromptGeneration: 1})
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{
		SessionID: models.ErrandExecutionIdentity(row.ID), TaskID: row.TaskID, ExecutionID: binding.ExecutionID,
		IncarnationID: binding.IncarnationID, HarnessGeneration: binding.HarnessGeneration, OwnerGeneration: int64(binding.Generation),
		StreamID: binding.StreamID, ExpectedStreamID: binding.StreamID, Sequence: 1, SubmissionID: row.SubmissionID,
		EventType: streams.EventTypeComplete, Terminal: true, Payload: payload, ReceivedAt: completedAt,
	}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	_, err = repo.SettleAgentDeliveryTerminal(ctx, binding.StreamID, 1, models.DeliverySubmissionCompleted, completedAt)
	require.NoError(t, err)
	target, _, err := repo.ReadOwnedErrandLatestTerminal(ctx, row.ID)
	require.NoError(t, err)
	return target
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
	workspaceID string,
	base time.Time,
) {
	t.Helper()
	id := historyRecoveryID(seed.ParentSessionID)
	turnID := id + "-turn"
	submissionID := "history-recovery-submission-" + id
	startedAt := base.Add(-5 * time.Minute)
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
	binding := seedHistoryErrandExecution(t, ctx, repo, row, startedAt)
	changed, err := repo.RecordErrandRecoveryObservation(ctx, models.ErrandRecoveryObservation{
		TaskID: seed.TaskID, Binding: binding, TurnID: turnID, SubmissionID: submissionID, PromptGeneration: 1, Unresolved: true,
	})
	require.NoError(t, err)
	require.True(t, changed)
	seedRecoveryErrandInventory(t, ctx, repo, row, binding, workspaceID)
}

func seedHistoryErrandExecution(t *testing.T, ctx context.Context, repo *tasksqlite.Repository, row *models.Errand, startedAt time.Time) models.ErrandHarnessBinding {
	t.Helper()
	id := row.ID
	generation, err := repo.ReserveErrandExecutionGeneration(ctx, id, 0)
	require.NoError(t, err)
	binding := models.ErrandHarnessBinding{
		ErrandID: id, Generation: generation.Generation, ExecutionID: id + "-execution",
		IncarnationID: id + "-incarnation", HarnessGeneration: 1, StreamID: id + "-stream", NativeSessionID: id + "-native",
	}
	bound, err := repo.BindErrandExecution(ctx, id, binding.Generation, binding.ExecutionID)
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &binding, 0))
	require.NoError(t, repo.CreateErrandTurn(ctx, id, &models.Turn{ID: row.FirstTurnID, StartedAt: startedAt}))
	prompt := []byte(row.Objective)
	admitted, err := repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{
		ErrandID: id, Generation: binding.Generation, ExecutionID: binding.ExecutionID, IncarnationID: binding.IncarnationID,
		HarnessGeneration: 1, StreamID: binding.StreamID, NativeSessionID: binding.NativeSessionID,
		PromptGeneration: 1, TurnID: row.FirstTurnID,
		Submission: &models.AgentDeliverySubmission{
			ID: row.SubmissionID, DispatchAttemptID: binding.ExecutionID, State: models.DeliverySubmissionAccepted,
			Payload: prompt, PayloadHash: journal.SubmissionHash(prompt),
		},
	})
	require.NoError(t, err)
	require.True(t, admitted)
	return binding
}

func seedRecoveryErrandInventory(t *testing.T, ctx context.Context, repo *tasksqlite.Repository, row *models.Errand, binding models.ErrandHarnessBinding, workspaceID string) {
	t.Helper()
	// Reset uses the current control runtime to confirm absence of this exact fixture execution.
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: binding.ExecutionID, SessionID: models.ErrandExecutionIdentity(row.ID),
		Runtime: executor.NameStandalone, Status: models.ExecutorRunningStatusStopped, AgentExecutionID: binding.ExecutionID,
		Metadata: map[string]any{"execution_owner": runtimeapi.ExecutionOwner{
			Kind: runtimeapi.ExecutionOwnerErrand, WorkspaceID: workspaceID, TaskID: row.TaskID, ErrandID: row.ID,
			Generation: binding.Generation, AgentProfileID: row.AgentProfileID,
		}},
	}))
}

func historyPendingID(parentSessionID string) string {
	return parentSessionID + "-pending-result"
}

func historyRecoveryID(parentSessionID string) string {
	return parentSessionID + "-recovery"
}
