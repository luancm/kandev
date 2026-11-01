package errandawait

// Identity is host-owned invocation authority, never tool arguments.
// Turn/submission are optional when the authenticated bridge cannot bind them.
type Identity struct {
	TaskID              string `json:"task_id"`
	ParentSessionID     string `json:"parent_session_id"`
	Token               string `json:"token"`
	ParentIncarnationID string `json:"parent_incarnation_id"`
	HarnessGeneration   int64  `json:"harness_generation"`
	TurnID              string `json:"turn_id,omitempty"`
	SubmissionID        string `json:"submission_id,omitempty"`
}

func (i Identity) Valid() bool {
	return i.TaskID != "" && i.ParentSessionID != "" && i.Token != "" && i.ParentIncarnationID != "" && i.HarnessGeneration > 0 &&
		len(i.TaskID) <= 256 && len(i.ParentSessionID) <= 256 && len(i.Token) <= 256 && len(i.ParentIncarnationID) <= 256 && len(i.TurnID) <= 256 && len(i.SubmissionID) <= 256
}
