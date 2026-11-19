package models

import "fmt"

const ErrandLiveLimit = 5

type ErrandCapacityError struct {
	Live int
}

func (e *ErrandCapacityError) Error() string {
	return fmt.Sprintf("a task can hold at most %d live errands", ErrandLiveLimit)
}

// ErrandContinuation identifies one new user instruction in an owned transcript.
type ErrandContinuation struct {
	TaskID          string
	ErrandID        string
	TurnID          string
	SubmissionID    string
	AuthorID        string
	Content         string
	ParentSessionID string
}

func (c ErrandContinuation) ParentAdmission() ErrandParentAdmission {
	return ErrandParentAdmission{
		TaskID: c.TaskID, ErrandID: c.ErrandID, TurnID: c.TurnID, SubmissionID: c.SubmissionID,
		ParentSessionID: c.ParentSessionID, SenderSessionID: c.ParentSessionID,
	}
}

// PreparedErrandContinuation records the new generation and exact retained conversation.
type PreparedErrandContinuation struct {
	Generation  uint64
	ExecutionID string
	NativeID    string
	Accepted    bool
	Predecessor ErrandTeardownTarget
}
