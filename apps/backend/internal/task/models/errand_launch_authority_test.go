package models

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func launchAuthorityFixture() (*TaskSession, ReservedLaunchIdentity) {
	return &TaskSession{ID: "child", TaskID: "task", Role: SessionRoleErrand, ParentSessionID: "parent", AgentProfileID: "initial-profile"},
		ReservedLaunchIdentity{TaskID: "task", SessionID: "child", TurnID: "reserved-turn", SubmissionID: "reserved-submission"}
}

// @covers AC-TASKS-ERRANDS-002.1 AC-TASKS-ERRANDS-008.2
func TestErrandLaunchAuthorityImmutableRoundTrip(t *testing.T) {
	session, identity := launchAuthorityFixture()
	session.Metadata = map[string]interface{}{"independent": "retained"}
	require.NoError(t, BindErrandLaunchAuthority(session, identity, "parent"))
	before, exists, err := LoadErrandLaunchAuthority(session.Metadata)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, ErrandLaunchAuthority{Version: 1, TaskID: "task", SessionID: "child", ParentSessionID: "parent", FirstTurnID: "reserved-turn", SubmissionID: "reserved-submission", AgentProfileID: "initial-profile"}, *before)
	require.NoError(t, BindErrandLaunchAuthority(session, identity, "parent"))
	identity.SubmissionID = "fresh-request-submission"
	require.Error(t, BindErrandLaunchAuthority(session, identity, "parent"))
	encoded, err := json.Marshal(session.Metadata)
	require.NoError(t, err)
	var reloaded map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &reloaded))
	after, exists, err := LoadErrandLaunchAuthority(reloaded)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, before, after)
	require.Equal(t, "retained", reloaded["independent"])
	after.FirstTurnID = "mutated-reader"
	again, _, err := LoadErrandLaunchAuthority(reloaded)
	require.NoError(t, err)
	require.Equal(t, "reserved-turn", again.FirstTurnID)
}

func TestErrandLaunchAuthorityRejectsMalformedRecord(t *testing.T) {
	valid := map[string]interface{}{"version": 1, "task_id": "task", "session_id": "child", "parent_session_id": "parent", "first_turn_id": "turn", "submission_id": "submission", "agent_profile_id": "profile"}
	for _, key := range []string{"version", "task_id", "session_id", "parent_session_id", "first_turn_id", "submission_id", "agent_profile_id"} {
		t.Run(key, func(t *testing.T) {
			candidate := make(map[string]interface{})
			for k, v := range valid {
				candidate[k] = v
			}
			delete(candidate, key)
			_, exists, err := LoadErrandLaunchAuthority(map[string]interface{}{SessionMetaKeyErrandLaunchAuthority: candidate})
			require.True(t, exists)
			require.Error(t, err)
		})
	}
	for _, raw := range []interface{}{nil, "invalid", 7, map[string]interface{}{"version": 2}} {
		_, exists, err := LoadErrandLaunchAuthority(map[string]interface{}{SessionMetaKeyErrandLaunchAuthority: raw})
		require.True(t, exists)
		require.Error(t, err)
	}
	_, exists, err := LoadErrandLaunchAuthority(nil)
	require.False(t, exists)
	require.NoError(t, err)
}

func TestErrandLaunchAuthorityRequiresCanonicalIdentity(t *testing.T) {
	for _, field := range []string{"task", "session", "role", "parent", "primary", "profile", "self parent"} {
		t.Run(field, func(t *testing.T) {
			session, identity := launchAuthorityFixture()
			parent := "parent"
			switch field {
			case "task":
				session.TaskID = "other-task"
			case "session":
				session.ID = "other-session"
			case "role":
				session.Role = ""
			case "parent":
				session.ParentSessionID = "other-parent"
			case "primary":
				session.IsPrimary = true
			case "profile":
				session.AgentProfileID = " "
			case "self parent":
				parent = session.ID
				session.ParentSessionID = parent
			}
			require.Error(t, BindErrandLaunchAuthority(session, identity, parent))
			require.NotContains(t, session.Metadata, SessionMetaKeyErrandLaunchAuthority)
		})
	}
}

func TestReservedLaunchIdentityReadDistinguishesMalformedFromMissing(t *testing.T) {
	_, exists, err := ReadReservedLaunchIdentity(context.Background())
	require.NoError(t, err)
	require.False(t, exists)
	_, valid := launchAuthorityFixture()
	for _, field := range []string{"task", "session", "turn", "submission", "blank", "padded"} {
		t.Run(field, func(t *testing.T) {
			identity := valid
			switch field {
			case "task":
				identity.TaskID = ""
			case "session":
				identity.SessionID = ""
			case "turn":
				identity.TurnID = ""
			case "submission":
				identity.SubmissionID = ""
			case "blank":
				identity.TurnID = " "
			case "padded":
				identity.SubmissionID = " submission "
			}
			_, exists, err := ReadReservedLaunchIdentity(WithReservedLaunchIdentity(context.Background(), identity))
			require.True(t, exists)
			require.Error(t, err)
		})
	}
	identity, exists, err := ReadReservedLaunchIdentity(WithReservedLaunchIdentity(context.Background(), valid))
	require.True(t, exists)
	require.NoError(t, err)
	require.Equal(t, valid, identity)
}
