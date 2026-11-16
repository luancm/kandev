package sqlite

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestErrandBatchClaimRejectsStaleInventoryAtomically(t *testing.T) {
	exerciseErrandStaleInventory(t, newRepoForSessionTests)
}

func TestPostgresErrandBatchClaimRejectsStaleInventoryAtomically(t *testing.T) {
	exerciseErrandStaleInventory(t, newPostgresErrandRepo)
}

func exerciseErrandStaleInventory(t *testing.T, create func(*testing.T) *Repository) {
	t.Helper()
	for _, mutation := range []string{"new_result", "new_empty", "deleted", "noncandidate_updated"} {
		t.Run(mutation, func(t *testing.T) {
			repo := create(t)
			row := newAdmissionResult(t, repo, "inventory")
			sibling := sampleErrand("other", row.TaskID, row.ParentSessionID, errandScenarioBase)
			mustCreateErrand(t, repo, sibling)
			inventory := claimInventory(t, repo, row.ParentSessionID)
			mutateClaimInventory(t, repo, sibling, mutation)
			candidates := []models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}
			rows, err := repo.ClaimResults(t.Context(), row.TaskID, row.ParentSessionID, models.ErrandDeliveryTurnContext,
				"stale-inventory", candidates, errandScenarioBase, inventory)
			if !errors.Is(err, models.ErrErrandVersionConflict) || len(rows) != 0 {
				t.Fatalf("stale full inventory accepted: rows=%+v err=%v", rows, err)
			}
			assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultPending)
			inventory = claimInventory(t, repo, row.ParentSessionID)
			rows, err = repo.ClaimResults(t.Context(), row.TaskID, row.ParentSessionID, models.ErrandDeliveryTurnContext,
				"stale-inventory", candidates, errandScenarioBase, inventory)
			if err != nil || len(rows) != 1 {
				t.Fatalf("same-token authoritative retry: rows=%+v err=%v", rows, err)
			}
		})
	}
}

func mutateClaimInventory(t *testing.T, repo *Repository, sibling *models.Errand, mutation string) {
	t.Helper()
	var err error
	switch mutation {
	case "new_result", "new_empty":
		row := sampleErrand("new", sibling.TaskID, sibling.ParentSessionID, errandScenarioBase)
		if mutation == "new_result" {
			row.ResultState, row.ResultText = models.ErrandResultPending, "phantom result"
		}
		err = repo.CreateErrand(t.Context(), row)
	case "deleted":
		err = repo.DeleteErrand(t.Context(), sibling.ID)
	default:
		sibling.AwaitingResult = !sibling.AwaitingResult
		err = repo.UpdateErrand(t.Context(), sibling)
	}
	if err != nil {
		t.Fatal(err)
	}
}
