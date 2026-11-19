package models

import (
	"errors"
	"testing"
)

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
