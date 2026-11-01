package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestErrandClaimIdentityCannotBeReused(t *testing.T) {
	exerciseErrandClaimIdentityCannotBeReused(t, newRepoForSessionTests(t))
}

func exerciseErrandClaimIdentityCannotBeReused(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	row := sampleErrand("immutable-claim", newErrandTask(t, repo, "immutable-claim"), "parent", errandScenarioBase)
	row.ResultState, row.ResultText = models.ErrandResultPending, "first"
	mustCreateErrand(t, repo, row)
	claim := func() ([]*models.Errand, error) {
		return repo.ClaimResults(ctx, row.TaskID, row.ParentSessionID, models.ErrandDeliveryTurnContext, "same-token",
			[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, errandScenarioBase, claimInventory(t, repo, row.ParentSessionID))
	}
	claimed, err := claim()
	if err != nil {
		t.Fatal(err)
	}
	row = claimed[0]
	row.ResultText, row.ResultState = "replacement", models.ErrandResultPending
	row.DeliveredRoute, row.DeliveryToken, row.DeliveredAt = "", "", nil
	if err := repo.UpdateErrand(ctx, row); err != nil {
		t.Fatal(err)
	}
	if _, err := claim(); !errors.Is(err, models.ErrErrandVersionConflict) {
		t.Fatalf("old token claimed a replacement: %v", err)
	}
	current, err := repo.GetErrand(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.ResultState != models.ErrandResultPending || current.ResultRevision != row.ResultRevision {
		t.Fatalf("rejected replay mutated replacement: %+v", current)
	}
}

func TestErrandBatchClaimAtomicOnSecondWriteFailure(t *testing.T) {
	exerciseErrandBatchClaimAtomic(t, newRepoForSessionTests(t))
}

func exerciseErrandBatchClaimAtomic(t *testing.T, repo *Repository) {
	t.Helper()
	taskID := newErrandTask(t, repo, "claim-failure")
	candidates := make([]models.ErrandClaimCandidate, 0, 2)
	for _, id := range []string{"claim-a", "claim-b"} {
		row := sampleErrand(id, taskID, "parent", errandScenarioBase)
		row.ResultState, row.ResultText = models.ErrandResultPending, id
		mustCreateErrand(t, repo, row)
		candidates = append(candidates, models.ErrandClaimCandidate{ID: id, Version: row.Version})
	}
	failErrandUpdate(t, repo, "NEW.id = 'claim-b' AND NEW.result_state = 'delivered'")
	_, err := repo.ClaimResults(context.Background(), taskID, "parent", models.ErrandDeliveryAwait, "token", candidates, time.Now(), claimInventory(t, repo, "parent"))
	if err == nil {
		t.Fatal("expected claim failure")
	}
	for _, candidate := range candidates {
		row, err := repo.GetErrand(context.Background(), candidate.ID)
		if err != nil {
			t.Fatal(err)
		}
		if row.ResultState != models.ErrandResultPending || row.DeliveryToken != "" {
			t.Fatalf("claim was not rolled back: %+v", row)
		}
	}
	for _, table := range []string{"errand_result_claim", "errand_result_claim_item", "errand_await_attempt", "errand_await_attempt_item"} {
		var count int
		if err := repo.db.Get(&count, `SELECT COUNT(*) FROM `+table); err != nil || count != 0 {
			t.Fatalf("failed batch leaked %s: count=%d err=%v", table, count, err)
		}
	}
}

func TestErrandBatchClaimRejectsInvalidSnapshotAtomically(t *testing.T) {
	exerciseErrandInvalidClaim(t, newRepoForSessionTests(t))
}

func TestErrandClaimWaitsForAdmissionDecision(t *testing.T) {
	exerciseErrandClaimAdmissionFence(t, newRepoForSessionTests(t))
}

func exerciseErrandClaimAdmissionFence(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	store := admissionRepository(t, repo)
	row := newAdmissionResult(t, repo, "claim-reserved")
	a := parentAdmission(row, "dispatch")
	if _, err := store.ReserveErrandParentAdmission(ctx, a); err != nil {
		t.Fatal(err)
	}
	claim := func() error {
		_, err := repo.ClaimResults(ctx, row.TaskID, row.ParentSessionID, models.ErrandDeliveryAwait, "reserved-token",
			[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, errandScenarioBase, claimInventory(t, repo, row.ParentSessionID))
		return err
	}
	if err := claim(); !errors.Is(err, models.ErrErrandVersionConflict) {
		t.Fatalf("claim escaped unresolved admission: %v", err)
	}
	if err := store.DecideErrandParentAdmission(ctx, a, models.ErrandAdmissionRefused); err != nil {
		t.Fatal(err)
	}
	if err := claim(); err != nil {
		t.Fatal(err)
	}
}

func exerciseErrandInvalidClaim(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	row := newAdmissionResult(t, repo, "invalid-batch")
	second := sampleErrand("z-invalid", row.TaskID, row.ParentSessionID, errandScenarioBase)
	second.ResultText, second.ResultState = "second", models.ErrandResultPending
	mustCreateErrand(t, repo, second)
	for _, scenario := range []string{"stale", "duplicate", "parent"} {
		candidates := []models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}, {ID: second.ID, Version: second.Version}}
		parent := row.ParentSessionID
		switch scenario {
		case "stale":
			candidates[1].Version++
		case "duplicate":
			candidates[1] = candidates[0]
		case "parent":
			parent = "other-parent"
		}
		if _, err := repo.ClaimResults(ctx, row.TaskID, parent, models.ErrandDeliveryTurnContext, scenario, candidates, errandScenarioBase, claimInventory(t, repo, parent)); err == nil {
			t.Fatalf("%s claim unexpectedly succeeded", scenario)
		}
		assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultPending)
		assertAdmissionResult(t, repo, second.ID, 0, models.ErrandResultPending)
	}
	inventory := claimInventory(t, repo, row.ParentSessionID)
	claimed, err := repo.ClaimResults(ctx, row.TaskID, row.ParentSessionID, models.ErrandDeliveryTurnContext, "replay",
		[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, errandScenarioBase, inventory)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := repo.ClaimResults(ctx, row.TaskID, row.ParentSessionID, models.ErrandDeliveryTurnContext, "replay",
		[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, errandScenarioBase, inventory)
	if err != nil {
		t.Fatal(err)
	}
	assertErrandFields(t, replayed[0], claimed[0])
}

// Result identity must survive unrelated CAS updates and change on replacement.
func TestErrandResultRevisionTracksReplacement(t *testing.T) {
	exerciseErrandRevision(t, newRepoForSessionTests(t))
}

func exerciseErrandRevision(t *testing.T, repo *Repository) {
	t.Helper()
	row := sampleErrand("revision", newErrandTask(t, repo, "revision"), "parent", errandScenarioBase)
	row.ResultState, row.ResultText = models.ErrandResultPending, "first"
	mustCreateErrand(t, repo, row)
	readRevision := func() string {
		t.Helper()
		var revision string
		if err := repo.db.Get(&revision, repo.db.Rebind(`SELECT result_revision FROM task_session_errands WHERE id = ?`), row.ID); err != nil {
			t.Fatal(err)
		}
		if revision == "" {
			t.Fatal("result has no revision")
		}
		return revision
	}
	first := readRevision()
	if err := repo.migrateErrandResultRevisions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if readRevision() != first {
		t.Fatal("migration replay replaced revision")
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_session_errands SET result_revision = '' WHERE id = ?`), row.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.migrateErrandResultRevisions(context.Background()); err != nil {
		t.Fatal(err)
	}
	first = readRevision()
	if err := repo.migrateErrandResultRevisions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if readRevision() != first {
		t.Fatal("backfill was not stable")
	}
	row.AwaitingResult = true
	if err := repo.UpdateErrand(context.Background(), row); err != nil {
		t.Fatal(err)
	}
	if readRevision() != first {
		t.Fatal("unrelated update replaced result identity")
	}
	row.ResultText = "replacement"
	if err := repo.UpdateErrand(context.Background(), row); err != nil {
		t.Fatal(err)
	}
	if readRevision() == first {
		t.Fatal("replacement retained old result identity")
	}
}
