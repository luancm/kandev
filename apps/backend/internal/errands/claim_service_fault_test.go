package errands

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	_ "github.com/mattn/go-sqlite3"
)

// @covers AC-TASKS-ERRANDS-004.4
func TestComposeParentTurnBatchClaimAtomicOnSecondWriteFailure(t *testing.T) {
	exerciseServiceClaimFault(t, false, false)
}

func TestAwaitBatchClaimAtomicOnSecondWriteFailure(t *testing.T) {
	exerciseServiceClaimFault(t, false, true)
}

func TestPostgresComposeParentTurnBatchClaimAtomicOnSecondWriteFailure(t *testing.T) {
	exerciseServiceClaimFault(t, true, false)
}

func TestPostgresAwaitBatchClaimAtomicOnSecondWriteFailure(t *testing.T) {
	exerciseServiceClaimFault(t, true, true)
}

func exerciseServiceClaimFault(t *testing.T, postgres, await bool) {
	t.Helper()
	conn := serviceClaimDB(t, postgres)
	repo, err := sqlite.NewWithDB(conn, conn, nil)
	if err != nil {
		t.Fatal(err)
	}
	seedServiceClaimRows(t, repo)
	h := newHarness(t)
	h.service.store = repo
	h.service.deliverer = nil
	installServiceClaimFault(t, conn)
	var text string
	if await {
		result, claimErr := h.service.Await(t.Context(), AwaitRequest{ParentSessionID: testParent,
			ErrandIDs: []string{"claim-a", "claim-b"}, Mode: "all", Token: "faulted-await"})
		err = claimErr
		for _, item := range result.Errands {
			text += item.Result
		}
	} else {
		result, claimErr := h.service.ComposeParentTurn(t.Context(), testParent, models.ErrandDeliveryTurnContext)
		err, text = claimErr, result.Block
	}
	if err == nil || text != "" {
		t.Fatalf("failed batch exposed a success: text=%q err=%v", text, err)
	}
	if statuses := h.publisher.statuses(); len(statuses) != 0 {
		t.Fatalf("rolled-back batch published updates: %v", statuses)
	}
	for _, id := range []string{"claim-a", "claim-b"} {
		row, err := repo.GetErrand(t.Context(), id)
		if err != nil || row.ResultState != models.ErrandResultPending || row.DeliveryToken != "" {
			t.Fatalf("service stranded a partial claim: row=%+v err=%v", row, err)
		}
	}
	for _, table := range []string{"errand_result_claim", "errand_result_claim_item", "errand_await_attempt_item"} {
		var count int
		if err := conn.Get(&count, `SELECT COUNT(*) FROM `+table); err != nil || count != 0 {
			t.Fatalf("service batch left %s: count=%d err=%v", table, count, err)
		}
	}
	query := `DROP TRIGGER service_claim_fault`
	if postgres {
		query += ` ON task_session_errands`
	}
	if _, err := conn.Exec(query); err != nil {
		t.Fatal(err)
	}
	result, err := h.service.ComposeParentTurn(t.Context(), testParent, models.ErrandDeliveryTurnContext)
	if err != nil || !strings.Contains(result.Block, "full result claim-a") || !strings.Contains(result.Block, "full result claim-b") {
		t.Fatalf("successful batch control: %+v err=%v", result, err)
	}
	if statuses := h.publisher.statuses(); len(statuses) != 2 {
		t.Fatalf("committed batch control published %v", statuses)
	}
}

func serviceClaimDB(t *testing.T, postgres bool) *sqlx.DB {
	t.Helper()
	if postgres {
		conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
		if _, err := conn.Exec(`CREATE TABLE kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
			t.Fatal(err)
		}
		return conn
	}
	conn, err := sqlx.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "service.db")+"?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	conn.SetMaxOpenConns(1)
	return conn
}

func seedServiceClaimRows(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "claim-ws", Name: "Claim faults"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: testTask, WorkspaceID: "claim-ws", Title: "Claim faults"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: testParent, TaskID: testTask}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"claim-a", "claim-b"} {
		row := &models.Errand{ID: id, TaskID: testTask, ParentSessionID: testParent,
			Title: id, Objective: id, AgentProfileID: "profile", FirstTurnID: "turn-" + id, SubmissionID: "submit-" + id,
			Lifecycle: models.ErrandLifecycleCancelled, CancelSource: models.ErrandCancelSourceUser,
			ResultState: models.ErrandResultPending, ResultKind: models.ErrandResultCancelled, ResultText: "full result " + id}
		if err := repo.CreateErrand(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
}

func installServiceClaimFault(t *testing.T, conn *sqlx.DB) {
	t.Helper()
	statements := []string{`CREATE TRIGGER service_claim_fault BEFORE UPDATE ON task_session_errands
		WHEN NEW.id = 'claim-b' AND NEW.result_state = 'delivered' BEGIN SELECT RAISE(ABORT, 'second claim write'); END`}
	if dialect.IsPostgres(conn.DriverName()) {
		statements = []string{
			`CREATE FUNCTION service_claim_fault_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'second claim write'; END $$`,
			`CREATE TRIGGER service_claim_fault BEFORE UPDATE ON task_session_errands FOR EACH ROW
			WHEN (NEW.id = 'claim-b' AND NEW.result_state = 'delivered') EXECUTE FUNCTION service_claim_fault_fn()`,
		}
	}
	for _, statement := range statements {
		if _, err := conn.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}
