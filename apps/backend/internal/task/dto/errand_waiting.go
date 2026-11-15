package dto

import "github.com/kandev/kandev/internal/task/models"

// ErrandWaitingProvider reports the errand state of a parent session. It is an
// optional extension of the live provider that serialization already holds, so
// a provider without errand support projects no waiting_on_errands field.
type ErrandWaitingProvider interface {
	// ParentErrandState reports whether the session owns any errand row and
	// whether one of those rows still owes an outcome.
	ParentErrandState(sessionID string) (hasErrands, outstanding bool)
}

// WaitingOnErrands derives the parent report: an idle parent with outstanding
// errands is waiting on them. Any other state reports false.
func WaitingOnErrands(state models.TaskSessionState, outstanding bool) bool {
	return outstanding && state == models.TaskSessionStateWaitingForInput
}

// errandWaitingValue returns the projected waiting_on_errands value, or nil
// when the session owns no errand row so the field is omitted entirely.
func errandWaitingValue(
	sessionID string, state models.TaskSessionState, provider ForegroundActivityProvider,
) *bool {
	waitingProvider, _ := provider.(ErrandWaitingProvider)
	return ErrandWaitingPayload(sessionID, state, waitingProvider)
}

// ErrandWaitingPayload returns the session-event value of waiting_on_errands,
// or nil when the session owns no errand row. The caller stores the value only
// when it is non-nil, so ordinary sessions keep their payloads unchanged.
func ErrandWaitingPayload(
	sessionID string, state models.TaskSessionState, provider ErrandWaitingProvider,
) *bool {
	if provider == nil {
		return nil
	}
	hasErrands, outstanding := provider.ParentErrandState(sessionID)
	if !hasErrands {
		return nil
	}
	waiting := WaitingOnErrands(state, outstanding)
	return &waiting
}
