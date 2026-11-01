package sqlite

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-ERRANDS-004.4
func TestErrandClaimSnapshotIsImmutable(t *testing.T) {
	exerciseErrandImmutableSnapshot(t, newRepoForSessionTests(t))
}

func TestPostgresErrandClaimSnapshotIsImmutable(t *testing.T) {
	exerciseErrandImmutableSnapshot(t, newPostgresErrandRepo(t))
}

func exerciseErrandImmutableSnapshot(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	row := newAdmissionResult(t, repo, "immutable-snapshot")
	candidates := []models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}
	inventory := claimInventory(t, repo, row.ParentSessionID)
	claim := func() ([]*models.Errand, error) {
		return repo.ClaimResults(ctx, row.TaskID, row.ParentSessionID, models.ErrandDeliveryTurnContext,
			"immutable-snapshot", candidates, errandScenarioBase, inventory)
	}
	first, err := claim()
	if err != nil {
		t.Fatal(err)
	}
	updated := *first[0]
	updated.AwaitingResult = !updated.AwaitingResult
	if err := repo.UpdateErrand(ctx, &updated); err != nil {
		t.Fatal(err)
	}
	replayed, err := claim()
	if err != nil || !reflect.DeepEqual(first, replayed) {
		t.Fatalf("claim readback changed with mutable row: first=%+v replay=%+v err=%v", first[0], replayed, err)
	}
	if err := repo.DeleteErrand(ctx, row.ID); err != nil {
		t.Fatal(err)
	}
	replayed, err = claim()
	if err != nil || !reflect.DeepEqual(first, replayed) {
		t.Fatalf("committed snapshot lost after row deletion: got=%+v err=%v", replayed, err)
	}
}

func claimInventory(t *testing.T, repo *Repository, parent string) []models.ErrandClaimCandidate {
	t.Helper()
	rows, err := repo.ListErrandsByParent(t.Context(), parent)
	if err != nil {
		t.Fatal(err)
	}
	return models.ErrandInventory(rows)
}
