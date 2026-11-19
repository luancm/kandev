package models

// ErrandPermissionOrigin identifies the admitted prompt owning a permission.
type ErrandPermissionOrigin struct {
	SubmissionID     string
	PromptGeneration uint64
	TurnID           string
}
