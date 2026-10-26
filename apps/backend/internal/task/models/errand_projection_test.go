package models

import "testing"

// @covers AC-TASKS-ERRANDS-003.3
func TestErrandCanSendResult(t *testing.T) {
	tests := []struct {
		name   string
		status ErrandStatus
		errand Errand
		want   bool
	}{
		{"done with an unreported later turn", ErrandStatusDone,
			Errand{LatestSettledTurnID: "turn-2", ResultTurnID: "turn-1", ResultState: ErrandResultPending}, true},
		{"done and the latest turn is already a pending result", ErrandStatusDone,
			Errand{LatestSettledTurnID: "turn-1", ResultTurnID: "turn-1", ResultState: ErrandResultPending}, false},
		{"done and the latest turn was already delivered", ErrandStatusDone,
			Errand{LatestSettledTurnID: "turn-1", ResultTurnID: "turn-1", ResultState: ErrandResultDelivered}, false},
		{"done and the latest turn result was superseded", ErrandStatusDone,
			Errand{LatestSettledTurnID: "turn-1", ResultTurnID: "turn-1", ResultState: ErrandResultSuperseded}, true},
		{"done and the latest turn result was discarded", ErrandStatusFailed,
			Errand{LatestSettledTurnID: "turn-1", ResultTurnID: "turn-1", ResultState: ErrandResultDiscarded}, true},
		{"done with no result recorded", ErrandStatusDone,
			Errand{LatestSettledTurnID: "turn-1", ResultState: ErrandResultNone}, true},
		{"no settled turn to capture", ErrandStatusFailed, Errand{ResultState: ErrandResultNone}, false},
		{"running", ErrandStatusRunning,
			Errand{LatestSettledTurnID: "turn-2", ResultTurnID: "turn-1", ResultState: ErrandResultDelivered}, false},
		{"needs attention", ErrandStatusNeedsAttention, Errand{LatestSettledTurnID: "turn-2"}, false},
		{"queued", ErrandStatusQueued, Errand{LatestSettledTurnID: "turn-2"}, false},
		{"cancelled", ErrandStatusCancelled, Errand{LatestSettledTurnID: "turn-2"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errand := tt.errand
			if got := errand.CanSendResult(tt.status); got != tt.want {
				t.Fatalf("CanSendResult(%s) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestAddSessionRoleFieldsOnlyForErrandSessions(t *testing.T) {
	errand := map[string]interface{}{"session_id": "errand-1"}
	AddSessionRoleFields(errand, &TaskSession{ID: "errand-1", Role: SessionRoleErrand, ParentSessionID: "parent-1"})
	if errand["role"] != "errand" || errand["parent_session_id"] != "parent-1" {
		t.Fatalf("errand fields = %v, want role=errand and parent_session_id=parent-1", errand)
	}

	ordinary := map[string]interface{}{"session_id": "session-1"}
	AddSessionRoleFields(ordinary, &TaskSession{ID: "session-1"})
	AddSessionRoleFields(ordinary, nil)
	if len(ordinary) != 1 {
		t.Fatalf("ordinary event payload = %v, want it unchanged", ordinary)
	}
}
