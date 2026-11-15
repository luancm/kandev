package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const SessionMetaKeyErrandLaunchAuthority = "errand_launch_authority"
const ErrandLaunchAuthorityVersion = 1

var ErrInvalidErrandLaunchAuthority = errors.New("invalid errand launch authority")

// ErrandLaunchAuthority is immutable initial provenance, never current-turn
// authority. Role and parent remain canonical indexed session columns.
type ErrandLaunchAuthority struct {
	Version         int    `json:"version"`
	TaskID          string `json:"task_id"`
	SessionID       string `json:"session_id"`
	ParentSessionID string `json:"parent_session_id"`
	FirstTurnID     string `json:"first_turn_id"`
	SubmissionID    string `json:"submission_id"`
	AgentProfileID  string `json:"agent_profile_id"`
}

// BindErrandLaunchAuthority binds a newly constructed errand before insertion.
// A repeat binding must preserve the complete original provenance.
func BindErrandLaunchAuthority(session *TaskSession, identity ReservedLaunchIdentity, parentID string) error {
	if session == nil || session.Role != SessionRoleErrand || session.IsPrimary ||
		session.TaskID != identity.TaskID || session.ID != identity.SessionID || session.ParentSessionID != parentID {
		return fmt.Errorf("%w: canonical session identity differs", ErrInvalidErrandLaunchAuthority)
	}
	want := ErrandLaunchAuthority{Version: ErrandLaunchAuthorityVersion,
		TaskID: identity.TaskID, SessionID: identity.SessionID, ParentSessionID: parentID,
		FirstTurnID: identity.TurnID, SubmissionID: identity.SubmissionID, AgentProfileID: session.AgentProfileID}
	if err := want.Validate(); err != nil {
		return err
	}
	existing, found, err := LoadErrandLaunchAuthority(session.Metadata)
	if err != nil {
		return err
	}
	if found {
		if *existing != want {
			return fmt.Errorf("%w: initial provenance cannot change", ErrInvalidErrandLaunchAuthority)
		}
		return nil
	}
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	session.Metadata[SessionMetaKeyErrandLaunchAuthority] = want
	return nil
}

// LoadErrandLaunchAuthority returns an independent copy. A present malformed
// record is an error, including null and unsupported versions.
func LoadErrandLaunchAuthority(metadata map[string]interface{}) (*ErrandLaunchAuthority, bool, error) {
	raw, found := metadata[SessionMetaKeyErrandLaunchAuthority]
	if !found {
		return nil, false, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, true, fmt.Errorf("%w: encode provenance: %w", ErrInvalidErrandLaunchAuthority, err)
	}
	var authority ErrandLaunchAuthority
	if err := json.Unmarshal(encoded, &authority); err != nil {
		return nil, true, fmt.Errorf("%w: decode provenance: %w", ErrInvalidErrandLaunchAuthority, err)
	}
	if err := authority.Validate(); err != nil {
		return nil, true, err
	}
	return &authority, true, nil
}

func (a ErrandLaunchAuthority) Validate() error {
	if a.Version != ErrandLaunchAuthorityVersion || a.SessionID == a.ParentSessionID ||
		!completeErrandLaunchIdentity(a.TaskID, a.SessionID, a.ParentSessionID, a.FirstTurnID, a.SubmissionID, a.AgentProfileID) {
		return fmt.Errorf("%w: requires a supported version and complete initial identity", ErrInvalidErrandLaunchAuthority)
	}
	return nil
}

// ReadReservedLaunchIdentity distinguishes an absent context from a malformed
// reservation. The compatibility getter intentionally omits that distinction.
func ReadReservedLaunchIdentity(ctx context.Context) (ReservedLaunchIdentity, bool, error) {
	raw := ctx.Value(reservedLaunchIdentityKey{})
	if raw == nil {
		return ReservedLaunchIdentity{}, false, nil
	}
	identity, ok := raw.(ReservedLaunchIdentity)
	if !ok || !completeErrandLaunchIdentity(identity.TaskID, identity.SessionID, identity.TurnID, identity.SubmissionID) {
		return identity, true, fmt.Errorf("%w: reserved context is incomplete", ErrInvalidErrandLaunchAuthority)
	}
	return identity, true, nil
}

func completeErrandLaunchIdentity(values ...string) bool {
	for _, value := range values {
		if value == "" || strings.TrimSpace(value) != value {
			return false
		}
	}
	return true
}
