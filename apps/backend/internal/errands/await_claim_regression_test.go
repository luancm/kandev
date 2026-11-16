package errands

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestAwaitReturnsOnlyResultsClaimedByItsToken(t *testing.T) {
	row := &models.Errand{ResultState: models.ErrandResultPending, ResultText: "private pending result", ResultTruncated: true}
	var item AwaitErrand
	fillAwaitResult(&item, row, "attempt")
	if item.Result != "" || item.Truncated {
		t.Fatalf("unclaimed result exposed: %+v", item)
	}
}
