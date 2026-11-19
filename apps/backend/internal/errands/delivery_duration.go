package errands

import "github.com/kandev/kandev/internal/task/models"

func deliveryDurationMillis(errand *models.Errand) *int64 {
	if errand == nil || errand.CreatedAt.IsZero() || errand.SettledAt == nil || errand.SettledAt.IsZero() {
		return nil
	}
	elapsed := errand.SettledAt.Sub(errand.CreatedAt)
	if elapsed < 0 {
		return nil
	}
	milliseconds := elapsed.Milliseconds()
	return &milliseconds
}
