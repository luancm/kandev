package models

import "testing"

// @covers AC-TASKS-ERRANDS-005.1
// @covers AC-TASKS-ERRANDS-012.3
func TestIsWorkflowDetachedSessionCoversErrandsAndCompletionFollowUps(t *testing.T) {
	followUp := map[string]interface{}{SessionMetaKeyCompletionFollowUp: true}
	cases := []struct {
		name     string
		session  *TaskSession
		detached bool
		errand   bool
	}{
		{name: "nil session", session: nil},
		{name: "ordinary session", session: &TaskSession{ID: "s"}},
		{name: "ordinary session with false follow-up marker", session: &TaskSession{
			ID: "s", Metadata: map[string]interface{}{SessionMetaKeyCompletionFollowUp: false},
		}},
		{name: "ordinary session with non-boolean follow-up marker", session: &TaskSession{
			ID: "s", Metadata: map[string]interface{}{SessionMetaKeyCompletionFollowUp: "true"},
		}},
		{name: "completion follow-up", session: &TaskSession{ID: "s", Metadata: followUp}, detached: true},
		{name: "errand", session: &TaskSession{ID: "s", Role: SessionRoleErrand, ParentSessionID: "p"}, detached: true, errand: true},
		{name: "errand with follow-up marker", session: &TaskSession{
			ID: "s", Role: SessionRoleErrand, ParentSessionID: "p", Metadata: followUp,
		}, detached: true, errand: true},
		{name: "unknown role is not an errand", session: &TaskSession{ID: "s", Role: SessionRole("helper")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsWorkflowDetachedSession(tc.session); got != tc.detached {
				t.Fatalf("IsWorkflowDetachedSession() = %v, want %v", got, tc.detached)
			}
			if got := IsErrandSession(tc.session); got != tc.errand {
				t.Fatalf("IsErrandSession() = %v, want %v", got, tc.errand)
			}
		})
	}
}

// @covers AC-TASKS-ERRANDS-005.1
func TestIsWorkflowDetachedSessionKeepsCompletionFollowUpMarkerSemantics(t *testing.T) {
	for _, metadata := range []map[string]interface{}{
		nil,
		{SessionMetaKeyCompletionFollowUp: true},
		{SessionMetaKeyCompletionFollowUp: false},
		{SessionMetaKeyCompletionFollowUp: 1},
	} {
		session := &TaskSession{ID: "s", Metadata: metadata}
		if got, want := IsWorkflowDetachedSession(session), IsCompletionFollowUpSession(metadata); got != want {
			t.Fatalf("ordinary session with metadata %v: detached = %v, follow-up marker = %v", metadata, got, want)
		}
	}
}
