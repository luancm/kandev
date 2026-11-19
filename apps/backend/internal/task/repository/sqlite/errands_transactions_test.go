package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func TestErrandAdmissionDecisionAndDeferredReleaseRollbackTogether(t *testing.T) {
	exerciseErrandAdmissionRollback(t, newRepoForSessionTests(t))
}

func exerciseErrandAdmissionRollback(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	store := admissionRepository(t, repo)
	row := newAdmissionResult(t, repo, "rollback")
	claim := claimAdmissionResult(t, repo, row, models.ErrandDeliveryAwait)
	a := parentAdmission(row, "rollback")
	if _, err := store.ReserveErrandParentAdmission(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := store.DecideErrandParentAdmission(ctx, a, models.ErrandAdmissionUnknown); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AbortAwait(ctx, row.TaskID, row.ParentSessionID, claim.Token); err != nil {
		t.Fatal(err)
	}
	removeFault := failErrandUpdate(t, repo, "NEW.result_state = 'pending'")
	if err := store.DecideErrandParentAdmission(ctx, a, models.ErrandAdmissionRefused); err == nil {
		t.Fatal("injected release write failure did not roll back admission")
	}
	assertAdmissionState(t, repo, a, "unknown")
	assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultDelivered)
	removeFault()
	if err := store.DecideErrandParentAdmission(ctx, a, models.ErrandAdmissionRefused); err != nil {
		t.Fatal(err)
	}
	assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultPending)
	assertAdmissionState(t, repo, a, "refused")
	// A failure after the decision write must also roll back generation.
	a = parentAdmission(row, "accepted-fault")
	if _, err := store.ReserveErrandParentAdmission(ctx, a); err != nil {
		t.Fatal(err)
	}
	removeFault = failErrandUpdate(t, repo, "NEW.parent_admission_generation > OLD.parent_admission_generation")
	if err := store.DecideErrandParentAdmission(ctx, a, models.ErrandAdmissionAccepted); err == nil {
		t.Fatal("injected generation write failure did not roll back decision")
	}
	assertAdmissionState(t, repo, a, "reserved")
	assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultPending)
	removeFault()
	if err := store.DecideErrandParentAdmission(ctx, a, models.ErrandAdmissionAccepted); err != nil {
		t.Fatal(err)
	}
	assertAdmissionResult(t, repo, row.ID, 1, models.ErrandResultSuperseded)
}

func TestErrandClaimAbortBatchRollsBackOnSecondWriteFailure(t *testing.T) {
	exerciseErrandAbortRollback(t, newRepoForSessionTests(t))
}

func exerciseErrandAbortRollback(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	store := admissionRepository(t, repo)
	first := newAdmissionResult(t, repo, "batch-abort")
	second := sampleErrand("z-second", first.TaskID, first.ParentSessionID, errandScenarioBase)
	second.ResultState, second.ResultText = models.ErrandResultPending, "second result"
	mustCreateErrand(t, repo, second)
	candidates := []models.ErrandClaimCandidate{{ID: first.ID, Version: first.Version}, {ID: second.ID, Version: second.Version}}
	if _, err := repo.ClaimResults(ctx, first.TaskID, first.ParentSessionID, models.ErrandDeliveryAwait, "batch-abort", candidates, errandScenarioBase, claimInventory(t, repo, first.ParentSessionID)); err != nil {
		t.Fatal(err)
	}
	removeFault := failErrandUpdate(t, repo, "NEW.id = 'z-second' AND NEW.result_state = 'pending'")
	if _, err := store.AbortAwait(ctx, first.TaskID, first.ParentSessionID, "batch-abort"); err == nil {
		t.Fatal("abort unexpectedly committed")
	}
	for _, id := range []string{first.ID, second.ID} {
		assertAdmissionResult(t, repo, id, 0, models.ErrandResultDelivered)
	}
	var state string
	if err := repo.db.Get(&state, `SELECT state FROM errand_await_attempt WHERE token = 'batch-abort'`); err != nil || state != "claimed" {
		t.Fatalf("abort tombstone partially committed: state=%s err=%v", state, err)
	}
	removeFault()
	if _, err := store.AbortAwait(ctx, first.TaskID, first.ParentSessionID, "batch-abort"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first.ID, second.ID} {
		assertAdmissionResult(t, repo, id, 0, models.ErrandResultPending)
	}
}

func assertAdmissionState(t *testing.T, repo *Repository, a models.ErrandParentAdmission, want string) {
	t.Helper()
	var state string
	err := repo.db.Get(&state, repo.db.Rebind(`SELECT state FROM errand_parent_admission WHERE errand_id = ? AND submission_id = ?`), a.ErrandID, a.SubmissionID)
	if err != nil || state != want {
		t.Fatalf("admission state=%s err=%v, want %s", state, err, want)
	}
}

func failErrandUpdate(t *testing.T, repo *Repository, condition string) func() {
	t.Helper()
	statements := []string{`CREATE TRIGGER storage_fault BEFORE UPDATE ON task_session_errands WHEN ` + condition + ` BEGIN SELECT RAISE(ABORT, 'injected errand update failure'); END`}
	if dialect.IsPostgres(repo.db.DriverName()) {
		statements = []string{
			`CREATE OR REPLACE FUNCTION storage_fault_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected errand update failure'; END $$`,
			`CREATE TRIGGER storage_fault BEFORE UPDATE ON task_session_errands FOR EACH ROW WHEN (` + condition + `) EXECUTE FUNCTION storage_fault_fn()`,
		}
	}
	for _, statement := range statements {
		if _, err := repo.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return func() {
		t.Helper()
		query := `DROP TRIGGER storage_fault`
		if dialect.IsPostgres(repo.db.DriverName()) {
			query += ` ON task_session_errands`
		}
		if _, err := repo.db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
}
