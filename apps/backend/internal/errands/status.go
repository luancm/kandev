package errands

import "github.com/kandev/kandev/internal/task/models"

// IsLive reports whether a projected status counts toward the live limit.
func IsLive(status models.ErrandStatus) bool {
	switch status {
	case models.ErrandStatusQueued, models.ErrandStatusRunning, models.ErrandStatusNeedsAttention:
		return true
	default:
		return false
	}
}

// IsOutstanding reports whether the parent still owes the Errand an outcome.
func IsOutstanding(errand *models.Errand) bool {
	return errand.Lifecycle == models.ErrandLifecycleQueued ||
		errand.AwaitingResult || errand.ResultState == models.ErrandResultPending
}
