package models

type ErrandExecutionDescriptor struct {
	ErrandID    string
	TaskID      string
	Generation  uint64
	ExecutionID string
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
