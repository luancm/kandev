package models

import (
	"fmt"
	"strings"
	"time"
)

type ErrandTeardownIntentState string

const ErrandTeardownIntentVersion = 1

const (
	ErrandTeardownIntentPending    ErrandTeardownIntentState = "pending"
	ErrandTeardownIntentCompleted  ErrandTeardownIntentState = "completed"
	ErrandTeardownIntentSuperseded ErrandTeardownIntentState = "superseded"
)

type ErrandTeardownTarget struct {
	TaskID               string `json:"task_id"`
	SessionID            string `json:"session_id"`
	SessionIncarnation   string `json:"session_incarnation,omitempty"`
	ExecutionID          string `json:"execution_id,omitempty"`
	TurnID               string `json:"turn_id,omitempty"`
	NoTurn               bool   `json:"no_turn,omitempty"`
	SubmissionID         string `json:"submission_id,omitempty"`
	FirstTurnID          string `json:"first_turn_id,omitempty"`
	ReservedSubmissionID string `json:"reserved_submission_id,omitempty"`
	PromptGeneration     uint64 `json:"prompt_generation,omitempty"`
}

type ErrandTeardownIntent struct {
	Version     int                       `json:"version"`
	ID          string                    `json:"id"`
	State       ErrandTeardownIntentState `json:"state"`
	RequestedAt time.Time                 `json:"requested_at"`
	Target      ErrandTeardownTarget      `json:"target"`
}

func (i *ErrandTeardownIntent) validateFor(errand *Errand) error {
	if i == nil {
		return nil
	}
	invalid := func(reason string) error {
		return fmt.Errorf("%w: teardown intent %s", ErrInvalidErrand, reason)
	}
	switch {
	case i.Version != ErrandTeardownIntentVersion:
		return invalid("version is unsupported")
	case strings.TrimSpace(i.ID) == "":
		return invalid("id is required")
	case i.RequestedAt.IsZero():
		return invalid("request time is required")
	case i.State != ErrandTeardownIntentPending && i.State != ErrandTeardownIntentCompleted && i.State != ErrandTeardownIntentSuperseded:
		return invalid("state is invalid")
	case errand.Lifecycle != ErrandLifecycleCancelled || errand.CancelSource == "":
		return invalid("requires a cancelled errand with a cancellation source")
	}
	return i.Target.validateFor(errand)
}

// ValidateFor checks the private intent against the errand identity and lifecycle.
func (i *ErrandTeardownIntent) ValidateFor(errand *Errand) error {
	return i.validateFor(errand)
}

func (target ErrandTeardownTarget) validateFor(errand *Errand) error {
	if strings.TrimSpace(target.TaskID) == "" || target.TaskID != errand.TaskID ||
		strings.TrimSpace(target.SessionID) == "" || target.SessionID != errand.ID {
		return invalidErrandTeardownTarget("does not match errand task and session")
	}
	if target.hasBlankIdentity() {
		return invalidErrandTeardownTarget("contains a blank identity value")
	}
	if target.hasReservedLaunchIdentity() {
		return target.validateReservedLaunch(errand)
	}
	return target.validateSessionTarget()
}

func (target ErrandTeardownTarget) hasReservedLaunchIdentity() bool {
	return target.FirstTurnID != "" || target.ReservedSubmissionID != ""
}

func (target ErrandTeardownTarget) hasBlankIdentity() bool {
	for _, value := range []string{
		target.ExecutionID, target.TurnID, target.SubmissionID,
		target.FirstTurnID, target.ReservedSubmissionID,
	} {
		if value != "" && strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}

func (target ErrandTeardownTarget) validateReservedLaunch(errand *Errand) error {
	if target.FirstTurnID == "" || target.ReservedSubmissionID == "" {
		return invalidErrandTeardownTarget("reserved launch identity must be a complete pre-session tuple")
	}
	if target.FirstTurnID != errand.FirstTurnID || target.ReservedSubmissionID != errand.SubmissionID {
		return invalidErrandTeardownTarget("reserved launch identity does not match errand")
	}
	if target.SessionIncarnation != "" || target.ExecutionID != "" || target.TurnID != "" ||
		target.NoTurn || target.SubmissionID != "" || target.PromptGeneration != 0 {
		return invalidErrandTeardownTarget("reserved launch identity must be a complete pre-session tuple")
	}
	return nil
}

func (target ErrandTeardownTarget) validateSessionTarget() error {
	if strings.TrimSpace(target.SessionIncarnation) == "" {
		return invalidErrandTeardownTarget("session incarnation is required")
	}
	if (target.TurnID == "") == !target.NoTurn {
		return invalidErrandTeardownTarget("requires a turn id or an explicit no-turn witness")
	}
	if target.PromptGeneration != 0 && target.ExecutionID == "" {
		return invalidErrandTeardownTarget("prompt generation requires an execution id")
	}
	return nil
}

func invalidErrandTeardownTarget(reason string) error {
	return fmt.Errorf("%w: teardown target %s", ErrInvalidErrand, reason)
}
