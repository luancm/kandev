package models

import (
	"errors"
	"testing"
)

// @covers AC-TASKS-ERRANDS-001.1
func TestSessionRoleProjectsThroughToAPIAndSummary(t *testing.T) {
	errand := &TaskSession{ID: "errand-1", TaskID: "task-1", Role: SessionRoleErrand, ParentSessionID: "parent-1"}
	api := errand.ToAPI()
	if api["role"] != "errand" || api["parent_session_id"] != "parent-1" {
		t.Fatalf("errand ToAPI role fields = (%v, %v), want (errand, parent-1)", api["role"], api["parent_session_id"])
	}

	ordinary := (&TaskSession{ID: "session-1", TaskID: "task-1"}).ToAPI()
	for _, key := range []string{"role", "parent_session_id"} {
		if _, ok := ordinary[key]; ok {
			t.Fatalf("ordinary session ToAPI contains %q; ordinary payloads must stay unchanged", key)
		}
	}

	observation := &TaskSessionSummaryObservation{
		ID: "errand-1", TaskID: "task-1", Role: SessionRoleErrand, ParentSessionID: "parent-1",
	}
	session := observation.ToTaskSession()
	if session.Role != SessionRoleErrand || session.ParentSessionID != "parent-1" {
		t.Fatalf("summary observation role fields = (%q, %q), want (errand, parent-1)", session.Role, session.ParentSessionID)
	}
}

func TestErrandValidateRequiresIdentityAndKnownStates(t *testing.T) {
	valid := func() *Errand {
		return &Errand{
			ID: "errand-1", TaskID: "task-1", ParentSessionID: "parent-1",
			Lifecycle: ErrandLifecycleQueued, ResultState: ErrandResultNone,
		}
	}
	if err := valid().Validate(); err != nil {
		t.Fatalf("valid errand rejected: %v", err)
	}
	cases := map[string]func(*Errand){
		"missing id":             func(e *Errand) { e.ID = "" },
		"missing task":           func(e *Errand) { e.TaskID = "" },
		"missing parent":         func(e *Errand) { e.ParentSessionID = "" },
		"self parent":            func(e *Errand) { e.ParentSessionID = e.ID },
		"unknown lifecycle":      func(e *Errand) { e.Lifecycle = "paused" },
		"empty lifecycle":        func(e *Errand) { e.Lifecycle = "" },
		"unknown cancel source":  func(e *Errand) { e.CancelSource = "system" },
		"unknown outcome":        func(e *Errand) { e.LastOutcome = "partial" },
		"unknown result kind":    func(e *Errand) { e.ResultKind = "summary" },
		"unknown result state":   func(e *Errand) { e.ResultState = "lost" },
		"empty result state":     func(e *Errand) { e.ResultState = "" },
		"unknown delivery route": func(e *Errand) { e.DeliveredRoute = "email" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			errand := valid()
			mutate(errand)
			if err := errand.Validate(); !errors.Is(err, ErrInvalidErrand) {
				t.Fatalf("Validate() = %v, want ErrInvalidErrand", err)
			}
		})
	}
}
