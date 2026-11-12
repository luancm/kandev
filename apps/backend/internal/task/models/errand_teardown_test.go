package models

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestErrandValidateRejectsPartialTeardownTarget(t *testing.T) {
	errand := &Errand{
		ID: "errand-1", TaskID: "task-1", ParentSessionID: "parent-1",
		Lifecycle: ErrandLifecycleCancelled, CancelSource: ErrandCancelSourceUser,
		ResultState: ErrandResultNone,
		TeardownIntent: &ErrandTeardownIntent{
			Version: 1, ID: "intent-1", State: ErrandTeardownIntentPending,
			RequestedAt: time.Date(2026, time.November, 1, 12, 0, 0, 0, time.UTC),
			Target:      ErrandTeardownTarget{TaskID: "task-1", SessionID: "errand-1", TurnID: "turn-1"},
		},
	}
	if err := errand.Validate(); !errors.Is(err, ErrInvalidErrand) {
		t.Fatalf("Validate() = %v, want ErrInvalidErrand for session target without incarnation", err)
	}
}

func TestErrandValidateRejectsBlankTeardownIdentities(t *testing.T) {
	base := Errand{
		ID: "errand-1", TaskID: "task-1", ParentSessionID: "parent-1",
		Lifecycle: ErrandLifecycleCancelled, CancelSource: ErrandCancelSourceUser,
		ResultState: ErrandResultNone,
	}
	intent := func(target ErrandTeardownTarget) *ErrandTeardownIntent {
		return &ErrandTeardownIntent{
			Version: ErrandTeardownIntentVersion, ID: "intent-1", State: ErrandTeardownIntentPending,
			RequestedAt: time.Date(2026, time.November, 1, 12, 0, 0, 0, time.UTC), Target: target,
		}
	}
	cases := []struct {
		name   string
		errand Errand
		target ErrandTeardownTarget
	}{
		{
			name:   "session turn",
			target: ErrandTeardownTarget{TaskID: "task-1", SessionID: "errand-1", SessionIncarnation: "queue-1", TurnID: " ", ExecutionID: "execution-1"},
		},
		{
			name:   "execution",
			target: ErrandTeardownTarget{TaskID: "task-1", SessionID: "errand-1", SessionIncarnation: "queue-1", TurnID: "turn-1", ExecutionID: " ", SubmissionID: "submission-1"},
		},
		{
			name:   "submission",
			target: ErrandTeardownTarget{TaskID: "task-1", SessionID: "errand-1", SessionIncarnation: "queue-1", TurnID: "turn-1", SubmissionID: "\t"},
		},
		{
			name:   "reserved launch tuple",
			errand: Errand{FirstTurnID: " ", SubmissionID: "\t"},
			target: ErrandTeardownTarget{TaskID: "task-1", SessionID: "errand-1", FirstTurnID: " ", ReservedSubmissionID: "\t"},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			row := base
			row.FirstTurnID, row.SubmissionID = test.errand.FirstTurnID, test.errand.SubmissionID
			target := test.target
			target.TaskID, target.SessionID = row.TaskID, row.ID
			row.TeardownIntent = intent(target)
			if err := row.Validate(); !errors.Is(err, ErrInvalidErrand) {
				t.Fatalf("Validate() = %v, want ErrInvalidErrand for blank teardown identity", err)
			}
		})
	}
}

func TestErrandValidateAcceptsSessionAndReservedTeardownTargets(t *testing.T) {
	base := Errand{
		ID: "errand-1", TaskID: "task-1", ParentSessionID: "parent-1",
		Lifecycle: ErrandLifecycleCancelled, CancelSource: ErrandCancelSourceUser,
		FirstTurnID: "first-turn-1", SubmissionID: "first-submission-1",
		ResultState: ErrandResultNone,
	}
	intent := func(target ErrandTeardownTarget) *ErrandTeardownIntent {
		return &ErrandTeardownIntent{
			Version: ErrandTeardownIntentVersion, ID: "intent-1", State: ErrandTeardownIntentPending,
			RequestedAt: time.Date(2026, time.November, 1, 12, 0, 0, 0, time.UTC), Target: target,
		}
	}
	cases := []struct {
		name   string
		target ErrandTeardownTarget
	}{
		{
			name: "session turn",
			target: ErrandTeardownTarget{
				TaskID: "task-1", SessionID: "errand-1", SessionIncarnation: "queue-1",
				ExecutionID: "execution-1", TurnID: "turn-1", SubmissionID: "submission-1",
			},
		},
		{
			name:   "session with explicit no-turn witness",
			target: ErrandTeardownTarget{TaskID: "task-1", SessionID: "errand-1", SessionIncarnation: "queue-1", NoTurn: true},
		},
		{
			name: "reserved initial launch",
			target: ErrandTeardownTarget{
				TaskID: "task-1", SessionID: "errand-1",
				FirstTurnID: "first-turn-1", ReservedSubmissionID: "first-submission-1",
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			row := base
			row.TeardownIntent = intent(test.target)
			if err := row.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestErrandCompletionCleanupAcceptsCompletionIntentForLaunchedErrand(t *testing.T) {
	var intent ErrandTeardownIntent
	encoded := `{"version":1,"id":"intent-1","state":"pending","requested_at":"2026-11-01T12:00:00Z","purpose":"completion","target":{"task_id":"task-1","session_id":"errand-1","session_incarnation":"incarnation-1","execution_id":"execution-1","turn_id":"turn-1"}}`
	if err := json.Unmarshal([]byte(encoded), &intent); err != nil {
		t.Fatalf("Unmarshal completion intent: %v", err)
	}
	err := Errand{
		ID: "errand-1", TaskID: "task-1", ParentSessionID: "parent-1",
		Lifecycle: ErrandLifecycleLaunched, ResultState: ErrandResultNone,
		TeardownIntent: &intent,
	}
	if err := err.Validate(); err != nil {
		t.Fatalf("launched errand with completion intent Validate() = %v", err)
	}
}

func TestErrandCompletionCleanupRejectsMissingPurposeOnLaunchedErrand(t *testing.T) {
	var intent ErrandTeardownIntent
	encoded := `{"version":1,"id":"intent-1","state":"pending","requested_at":"2026-11-01T12:00:00Z","target":{"task_id":"task-1","session_id":"errand-1","session_incarnation":"incarnation-1","execution_id":"execution-1","turn_id":"turn-1"}}`
	if err := json.Unmarshal([]byte(encoded), &intent); err != nil {
		t.Fatalf("Unmarshal legacy intent: %v", err)
	}
	err := Errand{
		ID: "errand-1", TaskID: "task-1", ParentSessionID: "parent-1",
		Lifecycle: ErrandLifecycleLaunched, ResultState: ErrandResultNone,
		TeardownIntent: &intent,
	}
	if err := err.Validate(); !errors.Is(err, ErrInvalidErrand) {
		t.Fatalf("missing-purpose launched intent Validate() = %v, want ErrInvalidErrand", err)
	}
}

func TestErrandJSONOmitsPrivateTeardownIntent(t *testing.T) {
	errand := Errand{TeardownIntent: &ErrandTeardownIntent{ID: "intent-private"}}
	encoded, err := json.Marshal(errand)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), "TeardownIntent") || strings.Contains(string(encoded), "intent-private") {
		t.Fatalf("private teardown intent was serialized: %s", encoded)
	}
}
