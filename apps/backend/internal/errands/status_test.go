package errands

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-ERRANDS-002.1
func TestProjectStatusFirstMatchWins(t *testing.T) {
	session := func(state models.TaskSessionState) *models.TaskSession {
		return &models.TaskSession{ID: "errand-1", State: state}
	}
	tests := []struct {
		name       string
		errand     models.Errand
		session    *models.TaskSession
		permission bool
		want       models.ErrandStatus
	}{
		{"cancelled beats everything", models.Errand{Lifecycle: models.ErrandLifecycleCancelled, LastOutcome: models.ErrandOutcomeDone},
			session(models.TaskSessionStateRunning), true, models.ErrandStatusCancelled},
		{"queued has no session", models.Errand{Lifecycle: models.ErrandLifecycleQueued}, nil, false, models.ErrandStatusQueued},
		{"launched without a session failed", models.Errand{Lifecycle: models.ErrandLifecycleLaunched}, nil, false, models.ErrandStatusFailed},
		{"failed session", models.Errand{Lifecycle: models.ErrandLifecycleLaunched, LastOutcome: models.ErrandOutcomeDone},
			session(models.TaskSessionStateFailed), false, models.ErrandStatusFailed},
		{"starting is running", models.Errand{Lifecycle: models.ErrandLifecycleLaunched},
			session(models.TaskSessionStateStarting), false, models.ErrandStatusRunning},
		{"working with a permission request waits for the user", models.Errand{Lifecycle: models.ErrandLifecycleLaunched},
			session(models.TaskSessionStateRunning), true, models.ErrandStatusNeedsAttention},
		{"a permission request on an idle session is not needs_attention",
			models.Errand{Lifecycle: models.ErrandLifecycleLaunched, LastOutcome: models.ErrandOutcomeDone},
			session(models.TaskSessionStateWaitingForInput), true, models.ErrandStatusDone},
		{"user turn running after a done turn reports running", models.Errand{Lifecycle: models.ErrandLifecycleLaunched, LastOutcome: models.ErrandOutcomeDone},
			session(models.TaskSessionStateRunning), false, models.ErrandStatusRunning},
		{"settled done", models.Errand{Lifecycle: models.ErrandLifecycleLaunched, LastOutcome: models.ErrandOutcomeDone},
			session(models.TaskSessionStateWaitingForInput), false, models.ErrandStatusDone},
		{"settled failed", models.Errand{Lifecycle: models.ErrandLifecycleLaunched, LastOutcome: models.ErrandOutcomeFailed},
			session(models.TaskSessionStateWaitingForInput), false, models.ErrandStatusFailed},
		{"idle without an outcome is running until reconciled", models.Errand{Lifecycle: models.ErrandLifecycleLaunched},
			session(models.TaskSessionStateWaitingForInput), false, models.ErrandStatusRunning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errand := tt.errand
			if got := ProjectStatus(&errand, tt.session, tt.permission); got != tt.want {
				t.Fatalf("ProjectStatus = %s, want %s", got, tt.want)
			}
		})
	}
}

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
