package errands

import (
	"github.com/kandev/kandev/internal/orchestrator/sessionstate"
	"github.com/kandev/kandev/internal/task/models"
)

// ProjectStatus is the single status reported for an errand. The first
// matching condition wins; permissionPending is only consulted while the
// session is working, so a stale request on an idle session never shows.
func ProjectStatus(errand *models.Errand, session *models.TaskSession, permissionPending bool, recovery ...RecoveryAttention) models.ErrandStatus {
	switch {
	case errand.Lifecycle == models.ErrandLifecycleCancelled:
		return models.ErrandStatusCancelled
	case errand.Lifecycle == models.ErrandLifecycleQueued:
		return models.ErrandStatusQueued
	case session == nil || session.State == models.TaskSessionStateFailed:
		return models.ErrandStatusFailed
	case sessionstate.IsWorking(session.State) && permissionPending:
		return models.ErrandStatusNeedsAttention
	case sessionstate.IsWorking(session.State):
		return models.ErrandStatusRunning
	case len(recovery) > 0 && recovery[0].Unresolved:
		return models.ErrandStatusNeedsAttention
	case errand.LastOutcome == models.ErrandOutcomeDone:
		return models.ErrandStatusDone
	case errand.LastOutcome == models.ErrandOutcomeFailed:
		return models.ErrandStatusFailed
	default:
		return models.ErrandStatusRunning
	}
}

// IsLive reports whether a projected status counts toward the live limit.
func IsLive(status models.ErrandStatus) bool {
	switch status {
	case models.ErrandStatusQueued, models.ErrandStatusRunning, models.ErrandStatusNeedsAttention:
		return true
	default:
		return false
	}
}

// IsOutstanding reports whether the parent still owes the errand an outcome:
// it is queued, awaiting a parent-initiated turn, or holds a pending result.
func IsOutstanding(errand *models.Errand, recovery ...RecoveryAttention) bool {
	if len(recovery) > 0 && recovery[0].Unresolved {
		return true
	}
	if errand.Lifecycle == models.ErrandLifecycleQueued {
		return true
	}
	return errand.AwaitingResult || errand.ResultState == models.ErrandResultPending
}

// RecoveryAttention is a read-derived observation, never settlement authority.
type RecoveryAttention struct{ Unresolved bool }

func attentionReason(status models.ErrandStatus, permission bool, recovery RecoveryAttention) models.ErrandAttentionReason {
	if status != models.ErrandStatusNeedsAttention {
		return ""
	}
	if permission {
		return models.ErrandAttentionPermission
	}
	if recovery.Unresolved {
		return models.ErrandAttentionRecoveryUnresolved
	}
	return ""
}
