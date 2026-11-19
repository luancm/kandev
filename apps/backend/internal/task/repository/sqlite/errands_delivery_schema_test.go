package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func TestErrandSchemaInitializesCurrentColumns(t *testing.T) {
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "errands.db"))
	require.NoError(t, err)
	pool := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	_, err = pool.ExecContext(t.Context(), `CREATE TABLE tasks (id TEXT PRIMARY KEY)`)
	require.NoError(t, err)
	repo := &Repository{db: pool, migrate: db.NewRequiredMigrateLogger(pool, nil)}
	require.NoError(t, repo.initErrandsSchema())

	for _, column := range []string{"result_revision", "parent_admission_generation", "teardown_intent"} {
		exists, err := db.ColumnExistsContext(t.Context(), pool, "task_session_errands", column)
		require.NoError(t, err)
		require.True(t, exists, "initial Errand schema must include %s", column)
	}
	exists, err := db.ColumnExistsContext(t.Context(), pool, "errand_result_claim", "snapshot")
	require.NoError(t, err)
	require.True(t, exists, "initial claim schema must include its immutable snapshot")
}

func TestErrandAdmissionGenerationIsRepositoryOwned(t *testing.T) {
	exerciseErrandAdmissionGenerationOwnership(t, newRepoForSessionTests(t))
}

func exerciseErrandAdmissionGenerationOwnership(t *testing.T, repo *Repository) {
	t.Helper()
	row := newAdmissionResult(t, repo, "generation-owner")
	row.ParentAdmissionGeneration = 9
	if err := repo.UpdateErrand(context.Background(), row); !errors.Is(err, models.ErrInvalidErrand) {
		t.Fatalf("CAS caller changed admission generation: %v", err)
	}
	assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultPending)
	row.ParentAdmissionGeneration = 0
	taskID, parentID := row.TaskID, row.ParentSessionID
	row.TaskID, row.ParentSessionID = "ignored-task", "ignored-parent"
	if err := repo.UpdateErrand(context.Background(), row); err != nil {
		t.Fatalf("immutable fields must remain compatible with full-row CAS: %v", err)
	}
	if row.TaskID != taskID || row.ParentSessionID != parentID {
		t.Fatalf("CAS returned uncommitted identity: %+v", row)
	}
	row.ID = "injected-generation"
	row.ParentAdmissionGeneration = 9
	if err := repo.CreateErrand(context.Background(), row); !errors.Is(err, models.ErrInvalidErrand) {
		t.Fatalf("insert caller injected admission generation: %v", err)
	}
}

func TestErrandAdmissionSchemaReplaysAndRetainsEvidence(t *testing.T) {
	exerciseErrandDeliverySchemaReplay(t, newRepoForSessionTests(t))
}

func exerciseErrandDeliverySchemaReplay(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	store := admissionRepository(t, repo)
	row := newAdmissionResult(t, repo, "delivery-migration")
	claim := claimAdmissionResult(t, repo, row, models.ErrandDeliveryAwait)
	a := parentAdmission(row, "migration")
	if _, err := store.ReserveErrandParentAdmission(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AbortAwait(ctx, row.TaskID, row.ParentSessionID, claim.Token); err != nil {
		t.Fatal(err)
	}
	if err := repo.initErrandsSchema(); err != nil {
		t.Fatal(err)
	}
	if err := store.DecideErrandParentAdmission(ctx, a, models.ErrandAdmissionAccepted); err != nil {
		t.Fatal(err)
	}
	if err := repo.initErrandsSchema(); err != nil {
		t.Fatal(err)
	}
	assertAdmissionResult(t, repo, row.ID, 1, models.ErrandResultSuperseded)
	ordinary := sampleErrand("no-result", row.TaskID, row.ParentSessionID, errandScenarioBase)
	mustCreateErrand(t, repo, ordinary)
	if ordinary.ResultRevision != "" || ordinary.ParentAdmissionGeneration != 0 {
		t.Fatalf("ordinary no-result row gained result identity: %+v", ordinary)
	}
	// Session/errand deletion cannot destroy a historical attempt or receipt.
	if err := repo.DeleteErrand(ctx, row.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"errand_parent_admission", "errand_result_claim", "errand_result_claim_item", "errand_await_attempt", "errand_await_attempt_item"} {
		var count int
		if err := repo.db.Get(&count, `SELECT COUNT(*) FROM `+table); err != nil || count != 1 {
			t.Fatalf("migration/deletion lost %s evidence: count=%d err=%v", table, count, err)
		}
	}
	// Existing task cleanup remains the retention boundary for all new records.
	if _, err := repo.db.Exec(repo.db.Rebind(`DELETE FROM tasks WHERE id = ?`), row.TaskID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"errand_parent_admission", "errand_result_claim", "errand_result_claim_item", "errand_await_attempt", "errand_await_attempt_item"} {
		var count int
		if err := repo.db.Get(&count, `SELECT COUNT(*) FROM `+table); err != nil || count != 0 {
			t.Fatalf("task deletion retained %s: count=%d err=%v", table, count, err)
		}
	}
}
