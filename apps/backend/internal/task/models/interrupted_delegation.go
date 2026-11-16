package models

import "github.com/kandev/kandev/internal/common/processidentity"

const InterruptedDelegationEffectType = "errand.interrupted_delegation.v1"

// InterruptedDelegationAssessment retains original admission independently of a
// replacement execution. Recovery must authenticate inventory before consumption.
type InterruptedDelegationAssessment struct {
	Version           int                      `json:"version"`
	Provenance        string                   `json:"provenance"`
	OriginalRuntime   processidentity.Identity `json:"original_runtime"`
	PromptGeneration  uint64                   `json:"prompt_generation"`
	TaskID            string                   `json:"task_id"`
	SessionID         string                   `json:"session_id"`
	ExecutionID       string                   `json:"execution_id"`
	IncarnationID     string                   `json:"incarnation_id"`
	HarnessGeneration uint64                   `json:"harness_generation"`
	StreamID          string                   `json:"stream_id"`
	SubmissionID      string                   `json:"submission_id"`
	TurnID            string                   `json:"turn_id"`
	NativeSessionID   string                   `json:"native_session_id"`
	PayloadHash       string                   `json:"payload_hash"`
}
