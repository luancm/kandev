package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

const errandRecoveryFixtureMarker = "KANDEV_E2E_ERRAND_RECOVERY_FIXTURE:"

type errandRecoveryPromptBinding struct {
	OriginalRuntime   processidentity.Identity `json:"original_runtime"`
	PromptGeneration  uint64                   `json:"prompt_generation"`
	TaskID            string                   `json:"task_id"`
	SessionID         string                   `json:"session_id"`
	ExecutionID       string                   `json:"execution_id"`
	IncarnationID     string                   `json:"incarnation_id"`
	HarnessGeneration uint64                   `json:"harness_generation"`
	StreamID          string                   `json:"stream_id"`
	SubmissionID      string                   `json:"submission_id"`
	TurnID            string                   `json:"turn_id"`
	NativeSessionID   string                   `json:"native_session_id"`
}

type errandRecoveryTraceRecord struct {
	Event        string `json:"event"`
	SessionID    string `json:"session_id"`
	Prompt       string `json:"prompt"`
	ProcessID    string `json:"process_id"`
	ConnectionID string `json:"connection_id"`
}

type errandRecoveryFixtureProof struct {
	Action             string `json:"action"`
	TaskID             string `json:"task_id"`
	SessionID          string `json:"session_id"`
	ParentSessionID    string `json:"parent_session_id"`
	IncarnationID      string `json:"incarnation_id"`
	HarnessGeneration  int64  `json:"harness_generation"`
	NativeSessionID    string `json:"native_session_id"`
	SubmissionID       string `json:"submission_id"`
	TurnID             string `json:"turn_id"`
	PayloadHash        string `json:"payload_hash"`
	WorkflowStepID     string `json:"workflow_step_id"`
	Attention          bool   `json:"attention"`
	MessageCount       int    `json:"message_count"`
	TurnCount          int    `json:"turn_count"`
	Revision           int64  `json:"revision"`
	ResultKind         string `json:"result_kind,omitempty"`
	ResultTurnID       string `json:"result_turn_id,omitempty"`
	ResultText         string `json:"result_text,omitempty"`
	SettlementReceipts int    `json:"settlement_receipts,omitempty"`
	SettlementOutcome  string `json:"settlement_outcome,omitempty"`
}

// TestErrandRecoveryBrowserFixture prepares and inspects a real admitted Errand
// for the managed browser suite. It is compiled into the existing
// e2e-send-now-fixture binary and runs only when that isolated suite supplies
// its temporary database root.
func TestErrandRecoveryBrowserFixture(t *testing.T) {
	root := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_ROOT")
	if root == "" {
		t.Skip("used only by the isolated managed Errand recovery browser fixture")
	}
	root = validateErrandRecoveryFixtureRoot(t, root)
	sessionID := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_SESSION")
	require.NotEmpty(t, sessionID)
	action := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_ACTION")
	tracePath := validateErrandRecoveryTracePath(t, root, os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_TRACE"))
	ctx := context.Background()

	connection, err := db.OpenSQLite(filepath.Join(root, "kandev.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	sqlConnection := sqlx.NewDb(connection, "sqlite3")
	repo, err := tasksqlite.NewWithDB(sqlConnection, sqlConnection, nil)
	require.NoError(t, err)

	var proof errandRecoveryFixtureProof
	switch action {
	case "inspect_admission":
		proof = inspectErrandRecoveryAdmission(t, ctx, repo, tracePath, sessionID, os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_PROMPT_TOKEN"))
	case "prepare_attention":
		proof = inspectErrandRecoveryAdmission(t, ctx, repo, tracePath, sessionID, os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_PROMPT_TOKEN"))
		prepareUnresolvedErrandRecovery(t, ctx, repo, proof)
		proof.Action = action
		proof.Attention, err = repo.ErrandRecoveryAttention(ctx, sessionID)
		require.NoError(t, err)
		require.True(t, proof.Attention, "repository evidence must project unresolved recovery attention")
	case "snapshot":
		proof = snapshotErrandRecoverySession(t, ctx, repo, sessionID)
	case "settle_interrupted":
		proof = settleInterruptedErrandRecovery(t, ctx, sqlConnection, repo, sessionID)
	case "assert_interrupted":
		proof = assertInterruptedErrandRecovery(t, ctx, sqlConnection, repo, sessionID, true)
	case "assert_continuation":
		proof = assertErrandRecoveryContinuation(t, ctx, sqlConnection, repo, tracePath, sessionID)
	default:
		t.Fatalf("unsupported Errand recovery fixture action %q", action)
	}
	encoded, err := json.Marshal(proof)
	require.NoError(t, err)
	fmt.Printf("%s%s\n", errandRecoveryFixtureMarker, encoded)
}

func settleInterruptedErrandRecovery(
	t *testing.T,
	ctx context.Context,
	database *sqlx.DB,
	repo *tasksqlite.Repository,
	sessionID string,
) errandRecoveryFixtureProof {
	t.Helper()
	expectedSubmission := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_SUBMISSION")
	expectedTurn := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_TURN")
	expectedIncarnation := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_INCARNATION")
	expectedHash := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_PAYLOAD_HASH")
	require.NotEmpty(t, expectedSubmission)
	require.NotEmpty(t, expectedTurn)
	require.NotEmpty(t, expectedIncarnation)
	require.NotEmpty(t, expectedHash)

	row, err := repo.GetErrand(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, row)
	require.Equal(t, models.ErrandLifecycleLaunched, row.Lifecycle)
	require.Equal(t, expectedTurn, row.FirstTurnID)
	require.Equal(t, expectedSubmission, row.SubmissionID)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	require.Equal(t, expectedIncarnation, session.QueueIncarnationID)
	submission, err := repo.GetAgentDeliverySubmission(ctx, expectedSubmission)
	require.NoError(t, err)
	require.NotNil(t, submission)
	require.Equal(t, expectedHash, submission.PayloadHash)
	turn, err := repo.GetTurn(ctx, expectedTurn)
	require.NoError(t, err)
	require.NotNil(t, turn)
	require.NotNil(t, turn.CompletedAt, "the browser-stopped admitted turn must be durably interrupted")

	pending, err := repo.PendingErrandTurnSettlements(ctx, sessionID)
	require.NoError(t, err)
	var exact *models.AgentDeliveryEffect
	for _, effect := range pending {
		if effect.TurnID == expectedTurn && effect.SubmissionID == expectedSubmission {
			exact = effect
			break
		}
	}
	if exact == nil {
		return assertInterruptedErrandRecovery(t, ctx, database, repo, sessionID, true)
	}
	require.Equal(t, expectedIncarnation, exact.IncarnationID)
	require.Equal(t, submission.HarnessGeneration, exact.HarnessGeneration)
	origin, _, sealed := strings.Cut(exact.Outcome, ":")
	if !sealed {
		origin = exact.Outcome
	}
	require.Equal(t, "parent", origin, "the original admission must retain its parent-origin terminal intent")

	settlements, err := errands.New(errands.Dependencies{
		Store: repo, Launcher: errandRecoveryFixtureLauncher{}, Stopper: errandRecoveryFixtureStopper{},
		Profiles: errandRecoveryFixtureProfiles{}, MCP: errandRecoveryFixtureMCP{}, Events: errandRecoveryFixtureEvents{},
	})
	require.NoError(t, err)
	settlements.SetTurnReader(errandRecoveryFixtureTurnReader{repo: repo})
	require.NoError(t, settlements.OnErrandTurnSettled(ctx, errands.TurnSettlement{
		SessionID: sessionID, TurnID: expectedTurn, SubmissionID: expectedSubmission,
		Outcome: models.ErrandOutcomeFailed, FailureSummary: "The errand was interrupted before it finished.", Interrupted: true,
	}))
	return assertInterruptedErrandRecovery(t, ctx, database, repo, sessionID, true)
}

type errandRecoveryFixtureLauncher struct{}

func (errandRecoveryFixtureLauncher) LaunchErrand(context.Context, errands.LaunchRequest) (errands.LaunchResult, error) {
	return errands.LaunchResult{}, nil
}

type errandRecoveryFixtureStopper struct{}

func (errandRecoveryFixtureStopper) CancelErrandWithTarget(context.Context, *models.Errand, func(errands.ErrandTeardownTarget) error) error {
	return nil
}

func (errandRecoveryFixtureStopper) RetryErrandTeardown(context.Context, errands.ErrandTeardownIntent) (errands.TeardownOutcome, error) {
	return errands.TeardownCompleted, nil
}

type errandRecoveryFixtureProfiles struct{}

func (errandRecoveryFixtureProfiles) ResolveProfile(context.Context, string, string, string) (errands.ResolvedProfile, error) {
	return errands.ResolvedProfile{}, nil
}

func (errandRecoveryFixtureProfiles) CompatibleProfiles(context.Context, string) ([]errands.ProfileInfo, error) {
	return nil, nil
}

type errandRecoveryFixtureMCP struct{}

func (errandRecoveryFixtureMCP) SessionSnapshot(context.Context, string, string) (errands.MCPSnapshot, error) {
	return errands.MCPSnapshot{}, nil
}

type errandRecoveryFixtureEvents struct{}

func (errandRecoveryFixtureEvents) PublishErrandUpdated(context.Context, *models.Errand, models.ErrandStatus) {
}

type errandRecoveryFixtureTurnReader struct {
	repo *tasksqlite.Repository
}

func (r errandRecoveryFixtureTurnReader) ReadTurn(ctx context.Context, sessionID, turnID string) (errands.TurnView, error) {
	turn, err := r.repo.GetTurn(ctx, turnID)
	if err != nil {
		return errands.TurnView{}, err
	}
	if turn.TaskSessionID != sessionID {
		return errands.TurnView{}, fmt.Errorf("turn %s belongs to another session", turnID)
	}
	return errands.TurnView{Completed: turn.CompletedAt != nil}, nil
}

func (r errandRecoveryFixtureTurnReader) LatestTurn(ctx context.Context, sessionID string) (string, errands.TurnView, error) {
	turns, err := r.repo.ListTurnsBySession(ctx, sessionID)
	if err != nil || len(turns) == 0 {
		return "", errands.TurnView{}, err
	}
	id := turns[len(turns)-1].ID
	view, err := r.ReadTurn(ctx, sessionID, id)
	return id, view, err
}

func validateErrandRecoveryFixtureRoot(t *testing.T, supplied string) string {
	t.Helper()
	require.True(t, filepath.IsAbs(supplied), "fixture root must be absolute")
	require.True(t, strings.HasPrefix(filepath.Base(filepath.Clean(supplied)), "kandev-e2e-"),
		"fixture root must be a supplied kandev-e2e-* directory")
	root, err := filepath.EvalSymlinks(supplied)
	require.NoError(t, err)
	tmpRoot, err := filepath.EvalSymlinks(os.TempDir())
	require.NoError(t, err)
	require.True(t, fixturePathWithin(tmpRoot, root), "fixture root must live under the system temporary directory")
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	homeKandev, err := filepath.Abs(filepath.Join(home, ".kandev"))
	require.NoError(t, err)
	require.False(t, fixturePathWithin(homeKandev, root), "browser fixture must never open ~/.kandev")
	return root
}

func validateErrandRecoveryTracePath(t *testing.T, root, supplied string) string {
	t.Helper()
	require.NotEmpty(t, supplied)
	require.True(t, filepath.IsAbs(supplied), "ACP trace path must be absolute")
	trace, err := filepath.EvalSymlinks(supplied)
	require.NoError(t, err)
	require.True(t, fixturePathWithin(root, trace), "ACP trace must belong to the isolated fixture root")
	return trace
}

func fixturePathWithin(parent, path string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func inspectErrandRecoveryAdmission(
	t *testing.T,
	ctx context.Context,
	repo *tasksqlite.Repository,
	tracePath, sessionID, expectedPromptToken string,
) errandRecoveryFixtureProof {
	t.Helper()
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, session)
	require.Equal(t, models.SessionRoleErrand, session.Role)
	errand, err := repo.GetErrand(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, errand)
	require.Equal(t, session.TaskID, errand.TaskID)
	require.Equal(t, session.ParentSessionID, errand.ParentSessionID)
	require.Equal(t, models.ErrandLifecycleLaunched, errand.Lifecycle)
	running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, running)
	rawBinding, err := json.Marshal(running.Metadata["delivery_prompt_turn_binding"])
	require.NoError(t, err)
	var binding errandRecoveryPromptBinding
	require.NoError(t, json.Unmarshal(rawBinding, &binding))
	require.NotEmpty(t, binding.NativeSessionID)
	require.NotEmpty(t, binding.SubmissionID)
	require.NotEmpty(t, binding.TurnID)
	require.NotEmpty(t, binding.StreamID)
	require.NoError(t, binding.OriginalRuntime.Validate())
	require.Equal(t, session.TaskID, binding.TaskID)
	require.Equal(t, sessionID, binding.SessionID)
	require.Equal(t, running.AgentExecutionID, binding.ExecutionID)
	require.Equal(t, session.QueueIncarnationID, binding.IncarnationID)
	require.Equal(t, errand.FirstTurnID, binding.TurnID)
	require.Equal(t, errand.SubmissionID, binding.SubmissionID)
	generation, err := repo.GetCurrentHarnessSessionGeneration(ctx, sessionID, session.QueueIncarnationID)
	require.NoError(t, err)
	require.NotNil(t, generation)
	require.Equal(t, binding.NativeSessionID, generation.NativeSessionID)
	require.Equal(t, int64(binding.HarnessGeneration), generation.Generation)
	submission, err := repo.GetAgentDeliverySubmission(ctx, binding.SubmissionID)
	require.NoError(t, err)
	require.NotNil(t, submission)
	require.Equal(t, sessionID, submission.SessionID)
	require.Equal(t, session.QueueIncarnationID, submission.IncarnationID)
	require.Equal(t, generation.Generation, submission.HarnessGeneration)
	require.NotEmpty(t, submission.PayloadHash)
	admission, err := repo.GetAgentDeliveryEffect(ctx, "agent_delivery.prompt_admission.v1:"+binding.SubmissionID)
	require.NoError(t, err)
	require.NotNil(t, admission)
	require.Equal(t, sessionID, admission.SessionID)
	require.Equal(t, session.QueueIncarnationID, admission.IncarnationID)
	require.Equal(t, generation.Generation, admission.HarnessGeneration)
	require.Equal(t, binding.TurnID, admission.TurnID)
	require.Equal(t, binding.SubmissionID, admission.SubmissionID)
	turn, err := repo.GetTurn(ctx, binding.TurnID)
	require.NoError(t, err)
	require.NotNil(t, turn)

	trace := readErrandRecoveryTrace(t, tracePath)
	providerPrompts := matchingErrandRecoveryPrompts(trace, binding.NativeSessionID, expectedPromptToken)
	require.Len(t, providerPrompts, 1, "the actual provider must have accepted exactly one matching original prompt")
	require.NotEmpty(t, providerPrompts[0].ProcessID)
	require.NotEmpty(t, providerPrompts[0].ConnectionID)

	revision, err := repo.ReadConversationRevision(ctx, sessionID)
	require.NoError(t, err)
	require.True(t, revision.Exists)
	messages, err := repo.ListMessages(ctx, sessionID)
	require.NoError(t, err)
	turns, err := repo.ListTurnsBySession(ctx, sessionID)
	require.NoError(t, err)
	task, err := repo.GetTask(ctx, session.TaskID)
	require.NoError(t, err)
	require.NotNil(t, task)
	return errandRecoveryFixtureProof{
		Action: "inspect_admission", TaskID: session.TaskID, SessionID: sessionID,
		ParentSessionID: session.ParentSessionID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: generation.Generation, NativeSessionID: generation.NativeSessionID,
		SubmissionID: submission.ID, TurnID: binding.TurnID, PayloadHash: submission.PayloadHash,
		WorkflowStepID: task.WorkflowStepID, MessageCount: len(messages), TurnCount: len(turns),
		Revision: revision.Revision,
	}
}

func prepareUnresolvedErrandRecovery(
	t *testing.T,
	ctx context.Context,
	repo *tasksqlite.Repository,
	proof errandRecoveryFixtureProof,
) {
	t.Helper()
	errand, err := repo.GetErrand(ctx, proof.SessionID)
	require.NoError(t, err)
	running, err := repo.GetExecutorRunningBySessionID(ctx, proof.SessionID)
	require.NoError(t, err)
	require.NotNil(t, running)
	rawBinding, err := json.Marshal(running.Metadata["delivery_prompt_turn_binding"])
	require.NoError(t, err)
	var binding errandRecoveryPromptBinding
	require.NoError(t, json.Unmarshal(rawBinding, &binding))
	require.Equal(t, proof.SubmissionID, binding.SubmissionID)
	require.Equal(t, proof.TurnID, binding.TurnID)
	require.Equal(t, proof.NativeSessionID, binding.NativeSessionID)
	submission, err := repo.GetAgentDeliverySubmission(ctx, binding.SubmissionID)
	require.NoError(t, err)
	require.NotNil(t, submission)
	require.NotEqual(t, models.DeliverySubmissionCompleted, submission.State)
	require.NotEqual(t, models.DeliverySubmissionFailed, submission.State)
	require.NotEqual(t, models.DeliverySubmissionCancelled, submission.State)
	turn, err := repo.GetTurn(ctx, binding.TurnID)
	require.NoError(t, err)
	require.NotNil(t, turn)
	require.Nil(t, turn.CompletedAt, "the provider prompt is still unresolved before this fixture action")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, proof.SessionID, models.TaskSessionStateWaitingForInput, ""))
	require.NoError(t, repo.AbandonTurn(ctx, binding.TurnID))
	recovery := &models.AgentDeliveryRecovery{
		OriginalRuntime: binding.OriginalRuntime, Phase: models.AgentDeliveryRecoveryUncertain,
		SessionID: proof.SessionID, AgentExecutionID: binding.ExecutionID, SubmissionID: binding.SubmissionID,
		StreamID: binding.StreamID, IncarnationID: binding.IncarnationID,
		HarnessGeneration: int64(binding.HarnessGeneration), PromptGeneration: binding.PromptGeneration,
		Message: "The admitted provider prompt has no terminal receipt.",
	}
	stored, err := repo.UpsertAgentDeliveryRecovery(ctx, recovery, &models.SessionRecoveryBlock{
		SessionID: proof.SessionID, IncarnationID: binding.IncarnationID,
		ExpectedGeneration: int64(binding.HarnessGeneration), Reason: "unknown_prompt_outcome",
		State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery",
		DeliverySubmissionID: binding.SubmissionID, DeliveryStreamID: binding.StreamID,
	})
	require.NoError(t, err)
	require.True(t, stored, "repository must commit the identity-bound recovery notice")
	recovery.Phase = models.AgentDeliveryRecoveryRecovered
	resolved, err := repo.ResolveAgentDeliveryRecovery(ctx, recovery, "retry_connection")
	require.NoError(t, err)
	require.True(t, resolved, "repository must release the resolved runtime-recovery block")
	updatedErrand, err := repo.GetErrand(ctx, errand.ID)
	require.NoError(t, err)
	require.Equal(t, errand.FirstTurnID, updatedErrand.FirstTurnID)
	require.Equal(t, errand.SubmissionID, updatedErrand.SubmissionID)
}

func snapshotErrandRecoverySession(
	t *testing.T,
	ctx context.Context,
	repo *tasksqlite.Repository,
	sessionID string,
) errandRecoveryFixtureProof {
	t.Helper()
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, session)
	errand, err := repo.GetErrand(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, errand)
	attention, err := repo.ErrandRecoveryAttention(ctx, sessionID)
	require.NoError(t, err)
	messages, err := repo.ListMessages(ctx, sessionID)
	require.NoError(t, err)
	turns, err := repo.ListTurnsBySession(ctx, sessionID)
	require.NoError(t, err)
	revision, err := repo.ReadConversationRevision(ctx, sessionID)
	require.NoError(t, err)
	require.True(t, revision.Exists)
	return errandRecoveryFixtureProof{
		Action: "snapshot", TaskID: session.TaskID, SessionID: sessionID,
		ParentSessionID: session.ParentSessionID, Attention: attention,
		MessageCount: len(messages), TurnCount: len(turns), Revision: revision.Revision,
		ResultKind: string(errand.ResultKind), ResultTurnID: errand.ResultTurnID, ResultText: errand.ResultText,
	}
}

func assertInterruptedErrandRecovery(
	t *testing.T,
	ctx context.Context,
	database *sqlx.DB,
	repo *tasksqlite.Repository,
	sessionID string,
	requireWaiting bool,
) errandRecoveryFixtureProof {
	t.Helper()
	expectedSubmission := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_SUBMISSION")
	expectedTurn := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_TURN")
	expectedNative := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_NATIVE")
	expectedIncarnation := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_INCARNATION")
	expectedHash := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_PAYLOAD_HASH")
	expectedGeneration := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_GENERATION")
	expectedWorkflowStep := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_WORKFLOW_STEP")
	require.NotEmpty(t, expectedSubmission)
	require.NotEmpty(t, expectedTurn)
	require.NotEmpty(t, expectedNative)
	require.NotEmpty(t, expectedIncarnation)
	require.NotEmpty(t, expectedHash)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, session)
	if requireWaiting {
		require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	}
	errand, err := repo.GetErrand(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, errand)
	require.Equal(t, expectedTurn, errand.FirstTurnID)
	require.Equal(t, expectedSubmission, errand.SubmissionID)
	require.Equal(t, models.ErrandResultInterrupted, errand.ResultKind)
	require.Equal(t, expectedTurn, errand.ResultTurnID)
	require.Equal(t, expectedTurn, errand.LatestSettledTurnID)
	require.Equal(t, "The errand was interrupted before it finished.", errand.ResultText)
	generation, err := repo.GetCurrentHarnessSessionGeneration(ctx, sessionID, expectedIncarnation)
	require.NoError(t, err)
	require.NotNil(t, generation)
	require.Equal(t, expectedNative, generation.NativeSessionID)
	require.Equal(t, expectedIncarnation, generation.IncarnationID)
	parsedGeneration, err := parseErrandRecoveryGeneration(expectedGeneration)
	require.NoError(t, err)
	require.Equal(t, parsedGeneration, generation.Generation)
	admission, err := repo.GetAgentDeliveryEffect(ctx, "agent_delivery.prompt_admission.v1:"+expectedSubmission)
	require.NoError(t, err)
	require.NotNil(t, admission)
	require.Equal(t, sessionID, admission.SessionID)
	require.Equal(t, expectedIncarnation, admission.IncarnationID)
	require.Equal(t, parsedGeneration, admission.HarnessGeneration)
	require.Equal(t, expectedTurn, admission.TurnID)
	require.Equal(t, expectedSubmission, admission.SubmissionID)
	submission, err := repo.GetAgentDeliverySubmission(ctx, expectedSubmission)
	require.NoError(t, err)
	require.NotNil(t, submission)
	require.Equal(t, expectedHash, submission.PayloadHash, "the interrupted submission remains immutable")
	var receipts int
	require.NoError(t, database.GetContext(ctx, &receipts, database.Rebind(`SELECT COUNT(*)
		FROM agent_delivery_effects WHERE session_id = ? AND incarnation_id = ? AND turn_id = ?
		AND effect_type = 'errand.turn_settlement'`), sessionID, expectedIncarnation, expectedTurn))
	require.Equal(t, 1, receipts, "the exact observed turn has one settlement receipt")
	var settlement struct {
		State   string       `db:"state"`
		Outcome string       `db:"outcome"`
		DoneAt  sql.NullTime `db:"completed_at"`
	}
	require.NoError(t, database.GetContext(ctx, &settlement, database.Rebind(`SELECT state, outcome, completed_at
		FROM agent_delivery_effects WHERE session_id = ? AND incarnation_id = ? AND turn_id = ?
		AND submission_id = ? AND effect_type = 'errand.turn_settlement'`),
		sessionID, expectedIncarnation, expectedTurn, expectedSubmission))
	require.Equal(t, models.DeliveryEffectCompleted, settlement.State)
	require.Equal(t, "parent:interrupted", settlement.Outcome)
	require.True(t, settlement.DoneAt.Valid)
	attention, err := repo.ErrandRecoveryAttention(ctx, sessionID)
	require.NoError(t, err)
	require.False(t, attention, "the exact interrupted receipt resolves the prior uncertainty")
	task, err := repo.GetTask(ctx, session.TaskID)
	require.NoError(t, err)
	require.NotNil(t, task)
	require.Equal(t, expectedWorkflowStep, task.WorkflowStepID)
	return errandRecoveryFixtureProof{
		Action: "assert_interrupted", TaskID: session.TaskID, SessionID: sessionID,
		ParentSessionID: session.ParentSessionID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: generation.Generation, NativeSessionID: generation.NativeSessionID,
		SubmissionID: submission.ID, TurnID: expectedTurn, PayloadHash: submission.PayloadHash,
		WorkflowStepID: task.WorkflowStepID,
		ResultKind:     string(errand.ResultKind), ResultTurnID: errand.ResultTurnID,
		ResultText: errand.ResultText, SettlementReceipts: receipts, SettlementOutcome: settlement.Outcome,
	}
}

func assertErrandRecoveryContinuation(
	t *testing.T,
	ctx context.Context,
	database *sqlx.DB,
	repo *tasksqlite.Repository,
	tracePath, sessionID string,
) errandRecoveryFixtureProof {
	t.Helper()
	expectedSubmission := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_SUBMISSION")
	expectedTurn := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_TURN")
	expectedNative := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_NATIVE")
	expectedIncarnation := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_INCARNATION")
	expectedHash := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_PAYLOAD_HASH")
	expectedGeneration := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_GENERATION")
	expectedWorkflowStep := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_EXPECT_WORKFLOW_STEP")
	require.NotEmpty(t, expectedSubmission)
	require.NotEmpty(t, expectedTurn)
	require.NotEmpty(t, expectedNative)
	require.NotEmpty(t, expectedIncarnation)
	require.NotEmpty(t, expectedHash)
	parsedGeneration, err := parseErrandRecoveryGeneration(expectedGeneration)
	require.NoError(t, err)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, session)
	require.Equal(t, expectedIncarnation, session.QueueIncarnationID)
	errand, err := repo.GetErrand(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, errand)
	require.Equal(t, expectedTurn, errand.FirstTurnID)
	require.Equal(t, expectedSubmission, errand.SubmissionID)
	generation, err := repo.GetCurrentHarnessSessionGeneration(ctx, sessionID, expectedIncarnation)
	require.NoError(t, err)
	require.NotNil(t, generation)
	require.Equal(t, expectedNative, generation.NativeSessionID)
	require.Equal(t, parsedGeneration, generation.Generation)
	oldSubmission, err := repo.GetAgentDeliverySubmission(ctx, expectedSubmission)
	require.NoError(t, err)
	require.NotNil(t, oldSubmission)
	require.Equal(t, expectedHash, oldSubmission.PayloadHash)
	var receipts int
	require.NoError(t, database.GetContext(ctx, &receipts, database.Rebind(`SELECT COUNT(*)
		FROM agent_delivery_effects WHERE session_id = ? AND incarnation_id = ? AND turn_id = ?
		AND effect_type = 'errand.turn_settlement'`), sessionID, expectedIncarnation, expectedTurn))
	require.Equal(t, 1, receipts)
	var settlement struct {
		State   string       `db:"state"`
		Outcome string       `db:"outcome"`
		DoneAt  sql.NullTime `db:"completed_at"`
	}
	require.NoError(t, database.GetContext(ctx, &settlement, database.Rebind(`SELECT state, outcome, completed_at
		FROM agent_delivery_effects WHERE session_id = ? AND incarnation_id = ? AND turn_id = ?
		AND submission_id = ? AND effect_type = 'errand.turn_settlement'`),
		sessionID, expectedIncarnation, expectedTurn, expectedSubmission))
	require.Equal(t, models.DeliveryEffectCompleted, settlement.State)
	require.Equal(t, "parent:interrupted", settlement.Outcome)
	require.True(t, settlement.DoneAt.Valid)
	prior := errandRecoveryFixtureProof{
		TaskID: session.TaskID, SessionID: sessionID, ParentSessionID: session.ParentSessionID,
		IncarnationID: expectedIncarnation, HarnessGeneration: parsedGeneration,
		NativeSessionID: expectedNative, SubmissionID: expectedSubmission, TurnID: expectedTurn,
		PayloadHash: expectedHash, WorkflowStepID: expectedWorkflowStep,
		SettlementReceipts: receipts, SettlementOutcome: settlement.Outcome,
	}
	expectedPromptToken := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_CONTINUATION_TOKEN")
	originalPromptToken := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_PROMPT_TOKEN")
	require.NotEmpty(t, expectedPromptToken)
	require.NotEmpty(t, originalPromptToken)
	running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, running)
	rawBinding, err := json.Marshal(running.Metadata["delivery_prompt_turn_binding"])
	require.NoError(t, err)
	var binding errandRecoveryPromptBinding
	require.NoError(t, json.Unmarshal(rawBinding, &binding))
	require.NotEmpty(t, binding.TurnID)
	require.NotEmpty(t, binding.SubmissionID)
	require.Equal(t, sessionID, binding.SessionID)
	require.Equal(t, session.TaskID, binding.TaskID)
	require.Equal(t, running.AgentExecutionID, binding.ExecutionID)
	require.Equal(t, prior.IncarnationID, binding.IncarnationID)
	require.Equal(t, prior.HarnessGeneration, int64(binding.HarnessGeneration))
	require.Equal(t, prior.NativeSessionID, binding.NativeSessionID)
	require.NotEqual(t, prior.TurnID, binding.TurnID, "the explicit new instruction creates a new turn")
	require.NotEqual(t, prior.SubmissionID, binding.SubmissionID, "the explicit new instruction has a new admission")
	generation, err = repo.GetCurrentHarnessSessionGeneration(ctx, sessionID, binding.IncarnationID)
	require.NoError(t, err)
	require.NotNil(t, generation)
	require.Equal(t, prior.NativeSessionID, generation.NativeSessionID)
	require.Equal(t, prior.HarnessGeneration, generation.Generation)
	oldSubmission, err = repo.GetAgentDeliverySubmission(ctx, prior.SubmissionID)
	require.NoError(t, err)
	require.NotNil(t, oldSubmission)
	require.Equal(t, prior.PayloadHash, oldSubmission.PayloadHash)
	newSubmission, err := repo.GetAgentDeliverySubmission(ctx, binding.SubmissionID)
	require.NoError(t, err)
	require.NotNil(t, newSubmission)
	require.NotEqual(t, oldSubmission.PayloadHash, newSubmission.PayloadHash)
	trace := readErrandRecoveryTrace(t, tracePath)
	require.Len(t, matchingErrandRecoveryPrompts(trace, prior.NativeSessionID, originalPromptToken), 1,
		"the original admitted instruction must not replay")
	followups := matchingErrandRecoveryPrompts(trace, prior.NativeSessionID, expectedPromptToken)
	require.Len(t, followups, 1, "the new explicit instruction must reach the retained native conversation once")
	revision, err := repo.ReadConversationRevision(ctx, sessionID)
	require.NoError(t, err)
	messages, err := repo.ListMessages(ctx, sessionID)
	require.NoError(t, err)
	turns, err := repo.ListTurnsBySession(ctx, sessionID)
	require.NoError(t, err)
	task, err := repo.GetTask(ctx, session.TaskID)
	require.NoError(t, err)
	require.NotNil(t, task)
	require.Equal(t, prior.WorkflowStepID, task.WorkflowStepID, "a user continuation cannot advance the task workflow")
	return errandRecoveryFixtureProof{
		Action: "assert_continuation", TaskID: session.TaskID, SessionID: sessionID,
		ParentSessionID: session.ParentSessionID, IncarnationID: binding.IncarnationID,
		HarnessGeneration: generation.Generation, NativeSessionID: generation.NativeSessionID,
		SubmissionID: binding.SubmissionID, TurnID: binding.TurnID, PayloadHash: newSubmission.PayloadHash,
		WorkflowStepID: task.WorkflowStepID, MessageCount: len(messages), TurnCount: len(turns),
		Revision: revision.Revision, ResultKind: string(models.ErrandResultInterrupted),
		ResultTurnID: prior.ResultTurnID, SettlementReceipts: prior.SettlementReceipts,
		SettlementOutcome: prior.SettlementOutcome,
	}
}

func readErrandRecoveryTrace(t *testing.T, path string) []errandRecoveryTraceRecord {
	t.Helper()
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	var records []errandRecoveryTraceRecord
	for _, line := range strings.Split(string(contents), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record errandRecoveryTraceRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}

func matchingErrandRecoveryPrompts(
	records []errandRecoveryTraceRecord,
	nativeSessionID, promptToken string,
) []errandRecoveryTraceRecord {
	var matches []errandRecoveryTraceRecord
	for _, record := range records {
		if record.Event == "prompt" && record.SessionID == nativeSessionID && strings.Contains(record.Prompt, promptToken) {
			matches = append(matches, record)
		}
	}
	return matches
}

func parseErrandRecoveryGeneration(value string) (int64, error) {
	return strconv.ParseInt(value, 10, 64)
}
