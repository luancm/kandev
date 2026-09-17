package models

import (
	"encoding/json"
	"strconv"
	"time"
)

// SessionMetaKeyGitPushAlert stores the bounded current Git push alert for a
// session. The transcript remains historical; this projection is the source of
// truth for whether the alert is currently actionable.
const SessionMetaKeyGitPushAlert = "git_push_alert"

// GitPushAlertState is the single current Git push alert projection persisted
// in TaskSession.Metadata. An inactive state keeps its revision so delayed
// events cannot overwrite a newer transition.
type GitPushAlertState struct {
	Active           bool      `json:"active"`
	Revision         uint64    `json:"revision"`
	TaskRepositoryID string    `json:"task_repository_id,omitempty"`
	MessageID        string    `json:"message_id,omitempty"`
	Diagnostic       string    `json:"diagnostic,omitempty"`
	OccurredAt       time.Time `json:"occurred_at"`
	Remote           string    `json:"remote,omitempty"`
	Branch           string    `json:"branch,omitempty"`
	StampValue       string    `json:"stamp,omitempty"`
}

// LoadGitPushAlert decodes both in-process and JSON-rehydrated metadata.
func LoadGitPushAlert(metadata map[string]interface{}) (GitPushAlertState, bool) {
	if metadata == nil {
		return GitPushAlertState{}, false
	}
	raw, ok := metadata[SessionMetaKeyGitPushAlert]
	if !ok || raw == nil {
		return GitPushAlertState{}, false
	}
	if state, ok := raw.(GitPushAlertState); ok {
		return normalizeGitPushAlert(state), true
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return GitPushAlertState{}, false
	}
	var state GitPushAlertState
	if err := json.Unmarshal(payload, &state); err != nil {
		return GitPushAlertState{}, false
	}
	return normalizeGitPushAlert(state), true
}

// Stamp returns the monotonic identity used by metadata CAS helpers.
func (state GitPushAlertState) Stamp() string {
	if state.StampValue != "" {
		return state.StampValue
	}
	return strconv.FormatUint(state.Revision, 10)
}

func normalizeGitPushAlert(state GitPushAlertState) GitPushAlertState {
	if state.StampValue == "" && state.Revision > 0 {
		state.StampValue = strconv.FormatUint(state.Revision, 10)
	}
	return state
}
