package errands

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestIsLiveStatus(t *testing.T) {
	live := map[models.ErrandStatus]bool{
		models.ErrandStatusQueued: true, models.ErrandStatusRunning: true, models.ErrandStatusNeedsAttention: true,
		models.ErrandStatusDone: false, models.ErrandStatusFailed: false, models.ErrandStatusCancelled: false,
	}
	for status, want := range live {
		if got := IsLive(status); got != want {
			t.Fatalf("IsLive(%s) = %v, want %v", status, got, want)
		}
	}
}
