package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

func TestErrandClaimSnapshotSerializesInventoryWriters(t *testing.T) {
	exerciseErrandInventoryWriters(t, newRepoForSessionTests)
}

func TestPostgresErrandClaimSnapshotSerializesInventoryWriters(t *testing.T) {
	exerciseErrandInventoryWriters(t, newPostgresErrandRepo)
}

func exerciseErrandInventoryWriters(t *testing.T, create func(*testing.T) *Repository) {
	t.Helper()
	for _, writer := range []string{"create", "migration", "replace", "cancel", "release", "abort", "delete", "session_delete", "task_delete", "workspace_delete", "workspace_simple", "ephemeral_profile", "ephemeral_expired", "prepared_queue"} {
		t.Run(writer, func(t *testing.T) { exerciseErrandInventoryWriter(t, create(t), writer) })
	}
}

func exerciseErrandInventoryWriter(t *testing.T, repo *Repository, writer string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	row := newAdmissionResult(t, repo, "inventory-writer")
	parent := &models.TaskSession{ID: row.ParentSessionID, TaskID: row.TaskID}
	createErrandScenarioSession(t, repo, parent)
	sibling := sampleErrand("sibling", row.TaskID, row.ParentSessionID, errandScenarioBase)
	if writer == "release" || writer == "abort" {
		sibling.ResultText, sibling.ResultState = "release sibling", models.ErrandResultPending
	}
	mustCreateErrand(t, repo, sibling)
	if writer == "release" || writer == "abort" {
		route := models.ErrandDeliveryTurnContext
		if writer == "abort" {
			route = models.ErrandDeliveryAwait
		}
		claimAdmissionResult(t, repo, sibling, route)
	}
	session := &models.TaskSession{ID: sibling.ID, TaskID: row.TaskID, Role: models.SessionRoleErrand, ParentSessionID: parent.ID, AgentProfileID: sibling.AgentProfileID}
	createErrandScenarioSession(t, repo, session)
	if writer == "migration" {
		if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_session_errands SET result_text = 'legacy result', result_revision = '' WHERE id = ?`), sibling.ID); err != nil {
			t.Fatal(err)
		}
	}
	if writer == "ephemeral_profile" || writer == "ephemeral_expired" {
		if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE tasks SET is_ephemeral = 1, updated_at = ? WHERE id = ?`), errandScenarioBase, row.TaskID); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_sessions SET updated_at = ? WHERE task_id = ?`), errandScenarioBase, row.TaskID); err != nil {
			t.Fatal(err)
		}
	}
	guard := "UPDATE tasks SET id = id WHERE 0"
	if dialect.IsPostgres(repo.db.DriverName()) {
		guard = "SELECT pg_advisory_xact_lock"
	}
	if writer == "session_delete" || writer == "prepared_queue" {
		guard = "UPDATE tasks SET updated_at"
	}
	if dialect.IsPostgres(repo.db.DriverName()) && deletesInventoryTask(writer) {
		guard = "SELECT id FROM tasks"
		if writer == "task_delete" {
			guard = "SELECT workflow_step_id FROM tasks WHERE id = $1 FOR UPDATE"
		}
	}
	writerGate := newErrandSQLGate(guard, false)
	other := independentErrandRepository(t, repo, writerGate)
	var queue *messagequeue.Service
	if writer == "prepared_queue" {
		queueRepo, err := messagequeue.NewSQLiteRepository(other.db, other.ro)
		if err != nil {
			t.Fatal(err)
		}
		queue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, logger.Default())
	}
	inventory := claimInventory(t, repo, parent.ID)
	claimGate := newErrandSQLGate("UPDATE task_session_errands SET result_state", true)
	defer claimGate.unblock()
	claimer := independentErrandRepository(t, repo, claimGate)
	claimed := make(chan error, 1)
	go func() {
		_, err := claimer.ClaimResults(ctx, row.TaskID, parent.ID, models.ErrandDeliveryTurnContext, "inventory-token",
			[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, errandScenarioBase, inventory)
		claimed <- err
	}()
	awaitSignal(t, claimGate.entered, "claim holds full inventory")
	finished := make(chan error, 1)
	go func() { finished <- runErrandInventoryWriter(ctx, other, writer, sibling, session, parent, queue) }()
	select {
	case <-writerGate.entered:
	case err := <-finished:
		t.Fatalf("inventory writer bypassed the task guard while claim was paused: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	claimGate.unblock()
	if err := awaitError(t, claimed, "claim commit"); err != nil {
		t.Fatal(err)
	}
	if err := awaitError(t, finished, "independent inventory writer"); err != nil {
		t.Fatal(err)
	}
	if deletesInventoryTask(writer) {
		if _, err := repo.GetErrand(ctx, row.ID); err == nil {
			t.Fatal("task deletion left the claimed errand behind")
		}
		return
	}
	// The captured result stays immutable even when another inventory member
	// has disappeared or a new member has been inserted after the commit.
	readback, err := claimer.ClaimResults(ctx, row.TaskID, parent.ID, models.ErrandDeliveryTurnContext, "inventory-token",
		[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, errandScenarioBase, inventory)
	if err != nil || len(readback) != 1 || readback[0].ResultText != row.ResultText {
		t.Fatalf("inventory writer changed committed output: %+v err=%v", readback, err)
	}
}

func runErrandInventoryWriter(ctx context.Context, repo *Repository, writer string, sibling *models.Errand, session, parent *models.TaskSession, queue *messagequeue.Service) error {
	switch writer {
	case "migration":
		return repo.backfillErrandResultRevisions(ctx)
	case "create":
		row := sampleErrand("new-sibling", sibling.TaskID, parent.ID, errandScenarioBase)
		return repo.CreateErrand(ctx, row)
	case "release":
		claims, err := repo.ListErrandResultClaims(ctx, sibling.TaskID, parent.ID, sibling.DeliveryToken)
		if err != nil {
			return err
		}
		_, err = repo.ReleaseErrandResultClaims(ctx, sibling.TaskID, parent.ID, claims)
		return err
	case "abort":
		_, err := repo.AbortAwait(ctx, sibling.TaskID, parent.ID, sibling.DeliveryToken)
		return err
	case "replace", "cancel":
		row := *sibling
		row.ResultState, row.ResultText = models.ErrandResultPending, "new sibling result"
		if writer == "cancel" {
			row.Lifecycle, row.CancelSource, row.ResultKind = models.ErrandLifecycleCancelled, models.ErrandCancelSourceUser, models.ErrandResultCancelled
		}
		return repo.UpdateErrand(ctx, &row)
	case "delete":
		return repo.DeleteErrand(ctx, sibling.ID)
	case "session_delete":
		return repo.DeleteTaskSession(ctx, session)
	case "task_delete":
		return repo.DeleteTask(ctx, sibling.TaskID)
	case "ephemeral_profile":
		_, err := repo.DeleteEphemeralTasksByAgentProfile(ctx, session.AgentProfileID)
		return err
	case "ephemeral_expired":
		_, err := repo.DeleteExpiredQuickChatTask(ctx, sibling.TaskID, errandScenarioBase.Add(time.Hour))
		return err
	case "workspace_delete", "workspace_simple":
		task, err := repo.GetTask(ctx, sibling.TaskID)
		if err != nil {
			return err
		}
		if writer == "workspace_simple" {
			return repo.DeleteWorkspace(ctx, task.WorkspaceID)
		}
		_, _, err = repo.DeleteWorkspaceCascade(ctx, task.WorkspaceID)
		return err
	default:
		_, err := queue.QueueMessageWithMetadataForSession(ctx, messagequeue.QueueSessionIdentity{
			TaskID: sibling.TaskID, SessionID: parent.ID, SessionIncarnationID: parent.QueueIncarnationID,
		}, "complete prepared cancellation output", "", messagequeue.QueuedByServer, false, nil,
			map[string]interface{}{"errand_delivery_token": "prepared-token"})
		return err
	}
}

func deletesInventoryTask(writer string) bool {
	switch writer {
	case "task_delete", "workspace_delete", "workspace_simple", "ephemeral_profile", "ephemeral_expired":
		return true
	default:
		return false
	}
}
