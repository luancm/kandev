package models

import (
	"fmt"
	"strings"
	"time"
)

type ErrandTeardownIntentState string
type ErrandTeardownPurpose string

const ErrandTeardownIntentVersion = 1

const (
	ErrandTeardownIntentPending    ErrandTeardownIntentState = "pending"
	ErrandTeardownIntentClosed     ErrandTeardownIntentState = "closed"
	ErrandTeardownIntentCompleted  ErrandTeardownIntentState = "completed"
	ErrandTeardownIntentSuperseded ErrandTeardownIntentState = "superseded"
)

const (
	ErrandTeardownPurposeCancellation ErrandTeardownPurpose = "cancellation"
	ErrandTeardownPurposeCompletion   ErrandTeardownPurpose = "completion"
)

type ErrandTeardownTarget struct {
	TaskID          string `json:"task_id"`
	ErrandID        string `json:"errand_id,omitempty"`
	OwnerGeneration uint64 `json:"owner_generation,omitempty"`

	ExecutionID  string `json:"execution_id,omitempty"`
	TurnID       string `json:"turn_id,omitempty"`
	NoTurn       bool   `json:"no_turn,omitempty"`
	SubmissionID string `json:"submission_id,omitempty"`

	PromptGeneration uint64 `json:"prompt_generation,omitempty"`
}

func (t ErrandTeardownTarget) ParentAdmission(parentSessionID string) ErrandParentAdmission {
	return ErrandParentAdmission{
		TaskID: t.TaskID, ErrandID: t.ErrandID, TurnID: t.TurnID, SubmissionID: t.SubmissionID,
		ParentSessionID: parentSessionID, SenderSessionID: parentSessionID,
	}
}

type ErrandTeardownIntent struct {
	Version     int                       `json:"version"`
	ID          string                    `json:"id"`
	State       ErrandTeardownIntentState `json:"state"`
	Purpose     ErrandTeardownPurpose     `json:"purpose,omitempty"`
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
	case i.State != ErrandTeardownIntentPending && i.State != ErrandTeardownIntentClosed &&
		i.State != ErrandTeardownIntentCompleted && i.State != ErrandTeardownIntentSuperseded:
		return invalid("state is invalid")
	}
	if err := i.validatePurpose(errand); err != nil {
		return err
	}
	return i.Target.validateFor(errand)
}

// ValidateFor checks the private intent against the errand identity and lifecycle.
func (i *ErrandTeardownIntent) ValidateFor(errand *Errand) error {
	return i.validateFor(errand)
}

func (target ErrandTeardownTarget) validateFor(errand *Errand) error {
	if target.ErrandID != errand.ID || target.TaskID != errand.TaskID || target.hasBlankIdentity() {
		return invalidErrandTeardownTarget("does not match the errand owner")
	}
	if target.OwnerGeneration == 0 && target.ExecutionID != "" {
		return invalidErrandTeardownTarget("execution requires a generation")
	}
	if target.NoTurn {
		if target.TurnID != "" || target.SubmissionID != "" {
			return invalidErrandTeardownTarget("no-turn witness carries a submission")
		}
		return nil
	}
	if target.TurnID == "" || target.SubmissionID == "" || target.ExecutionID == "" || target.PromptGeneration == 0 {
		return invalidErrandTeardownTarget("active owned turn requires exact dispatch identity")
	}
	return nil
}

func (target ErrandTeardownTarget) hasBlankIdentity() bool {
	for _, value := range []string{
		target.ExecutionID, target.TurnID, target.SubmissionID,
	} {
		if value != "" && strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}

func invalidErrandTeardownTarget(reason string) error {
	return fmt.Errorf("%w: teardown target %s", ErrInvalidErrand, reason)
}

func (i *ErrandTeardownIntent) validatePurpose(errand *Errand) error {
	invalid := func(reason string) error { return fmt.Errorf("%w: teardown intent %s", ErrInvalidErrand, reason) }
	switch i.Purpose {
	case ErrandTeardownPurposeCancellation:
		if errand.Lifecycle != ErrandLifecycleCancelled || errand.CancelSource == "" || i.State == ErrandTeardownIntentClosed {
			return invalid("cancellation requires a cancelled errand with a cancellation source")
		}
	case ErrandTeardownPurposeCompletion:
		if errand.Lifecycle == ErrandLifecycleCancelled || errand.CancelSource != "" || i.Target.TurnID == "" || i.Target.NoTurn {
			return invalid("completion requires an uncancelled errand with an exact turn target")
		}
	default:
		return invalid("purpose is invalid")
	}
	return nil
}
