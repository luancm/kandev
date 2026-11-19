package models

import (
	"time"

	"github.com/kandev/kandev/internal/common/processidentity"
)

const ErrandTurnRecoveryUnresolvedKey = "errand_recovery_unresolved"

// ErrandRecoveryObservation identifies the active prompt behind a stream recovery transition.
type ErrandRecoveryObservation struct {
	TaskID           string
	Binding          ErrandHarnessBinding
	TurnID           string
	SubmissionID     string
	PromptGeneration uint64
	Unresolved       bool
}

type ErrandExecutionDescriptor struct {
	ErrandID           string
	TaskID             string
	Generation         uint64
	ExecutionID        string
	ActiveSubmissionID string
	ActiveTurnID       string
	PromptGeneration   uint64
}

// ErrandPermissionOwner identifies one Errand execution generation.
type ErrandPermissionOwner struct {
	ErrandID    string
	TaskID      string
	Generation  uint64
	ExecutionID string
}

// ErrandPermissionSource is the immutable admitted origin of one request.
type ErrandPermissionSource struct {
	Owner             ErrandPermissionOwner
	IncarnationID     string
	HarnessGeneration int64
	StreamID          string
	SubmissionID      string
	TurnID            string
	PromptGeneration  uint64
	RequestID         string
	PendingID         string
}

type ErrandPermissionFinalization struct {
	ClaimID     string
	Result      PermissionResolutionResult
	Status      PermissionStatus
	FinalizedAt time.Time
}

type ErrandDeliveryAdmission struct {
	ErrandID          string
	Generation        uint64
	ExecutionID       string
	IncarnationID     string
	HarnessGeneration int64
	StreamID          string
	NativeSessionID   string
	PromptGeneration  uint64
	TurnID            string
	OriginalRuntime   processidentity.Identity
	Submission        *AgentDeliverySubmission
}

// ErrandInterruptedAdmission identifies an admitted prompt for retained journal recovery.
type ErrandInterruptedAdmission struct {
	Binding          ErrandHarnessBinding
	TaskID           string
	TurnID           string
	SubmissionID     string
	PromptGeneration uint64
	PayloadHash      string
	OriginalRuntime  processidentity.Identity
}

type ErrandHarnessBinding struct {
	ErrandID          string
	Generation        uint64
	Sequence          uint64
	ExecutionID       string
	IncarnationID     string
	HarnessGeneration int64
	StreamID          string
	NativeSessionID   string
}

// ErrandToolOrigin identifies the admitted prompt that created a tool call.
type ErrandToolOrigin struct {
	SubmissionID     string
	PromptGeneration uint64
	TurnID           string
}
