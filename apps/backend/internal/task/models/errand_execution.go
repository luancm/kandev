package models

import "time"

type ErrandExecutionDescriptor struct {
	ErrandID           string
	TaskID             string
	Generation         uint64
	ExecutionID        string
	ActiveSubmissionID string
	ActiveTurnID       string
	PromptGeneration   uint64
}

type ErrandPermissionFinalization struct {
	RequestID   string
	PendingID   string
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
	Submission        *AgentDeliverySubmission
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
