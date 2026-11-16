package orchestrator

import (
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"testing"
)

func admissionClaimInventory(t *testing.T, repo *sqlite.Repository, parent string) []models.ErrandClaimCandidate {
	t.Helper()
	rows, err := repo.ListErrandsByParent(t.Context(), parent)
	if err != nil {
		t.Fatal(err)
	}
	return models.ErrandInventory(rows)
}
