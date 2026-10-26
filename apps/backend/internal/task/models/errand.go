package models

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// SessionRole distinguishes errand sessions from ordinary task sessions. It is
// written once when the session row is inserted and never updated.
type SessionRole string

// SessionRoleErrand marks a session that a parent session started as an errand.
const SessionRoleErrand SessionRole = "errand"

// IsErrandSession reports whether session is an errand. An errand never owns
// the primary role, the generated title, or task runtime state, and is never
// a default target for task-level routing.
func IsErrandSession(session *TaskSession) bool {
	return session != nil && session.Role == SessionRoleErrand
}

// ErrandLifecycle is the errand-owned launch state, independent of the
// errand session's runtime state.
type ErrandLifecycle string

const (
	ErrandLifecycleQueued    ErrandLifecycle = "queued"
	ErrandLifecycleLaunched  ErrandLifecycle = "launched"
	ErrandLifecycleCancelled ErrandLifecycle = "cancelled"
)

// ErrandCancelSource records who cancelled an errand; empty while not cancelled.
type ErrandCancelSource string

const (
	ErrandCancelSourceParent  ErrandCancelSource = "parent"
	ErrandCancelSourceUser    ErrandCancelSource = "user"
	ErrandCancelSourceCascade ErrandCancelSource = "cascade"
)

// ErrandOutcome is the outcome of the latest parent-initiated errand turn;
// empty until one settles.
type ErrandOutcome string

const (
	ErrandOutcomeDone   ErrandOutcome = "done"
	ErrandOutcomeFailed ErrandOutcome = "failed"
)

// ErrandResultKind classifies the current result; empty when there is none.
type ErrandResultKind string

const (
	ErrandResultOutput      ErrandResultKind = "output"
	ErrandResultFailure     ErrandResultKind = "failure"
	ErrandResultInterrupted ErrandResultKind = "interrupted"
	ErrandResultCancelled   ErrandResultKind = "cancelled"
)

// ErrandResultState tracks delivery of the current result to the parent.
type ErrandResultState string

const (
	ErrandResultNone       ErrandResultState = "none"
	ErrandResultPending    ErrandResultState = "pending"
	ErrandResultDelivered  ErrandResultState = "delivered"
	ErrandResultSuperseded ErrandResultState = "superseded"
	ErrandResultDiscarded  ErrandResultState = "discarded"
)

// ErrandDeliveryRoute names how a delivered result reached the parent; empty
// until delivery.
type ErrandDeliveryRoute string

const (
	ErrandDeliveryAwait        ErrandDeliveryRoute = "await"
	ErrandDeliveryTurnContext  ErrandDeliveryRoute = "turn_context"
	ErrandDeliveryNotification ErrandDeliveryRoute = "notification"
)

// ErrandStatus is the single projected status reported for an errand.
type ErrandStatus string

const (
	ErrandStatusQueued         ErrandStatus = "queued"
	ErrandStatusRunning        ErrandStatus = "running"
	ErrandStatusNeedsAttention ErrandStatus = "needs_attention"
	ErrandStatusDone           ErrandStatus = "done"
	ErrandStatusFailed         ErrandStatus = "failed"
	ErrandStatusCancelled      ErrandStatus = "cancelled"
)

var (
	ErrErrandNotFound        = errors.New("errand not found")
	ErrErrandVersionConflict = errors.New("errand version changed")
	ErrInvalidErrand         = errors.New("invalid errand")
	ErrErrandSessionPrimary  = errors.New("an errand session cannot be primary")
)

// Errand is the bookkeeping row for one errand. ID equals the errand's
// reserved session ID; a queued errand has no session row yet. ID, TaskID,
// ParentSessionID, Title, Objective, AgentProfileID, MCPServerIDs,
// FirstTurnID, SubmissionID, and CreatedAt are fixed at insert. Every other
// change is a compare-and-set on Version.
type Errand struct {
	ID                  string
	TaskID              string
	ParentSessionID     string
	Title               string
	Objective           string
	AgentProfileID      string
	MCPServerIDs        []string
	FirstTurnID         string
	SubmissionID        string
	Lifecycle           ErrandLifecycle
	CancelSource        ErrandCancelSource
	LastOutcome         ErrandOutcome
	AwaitingResult      bool
	ResultText          string
	ResultTruncated     bool
	ResultTurnID        string
	ResultKind          ErrandResultKind
	ResultState         ErrandResultState
	DeliveredRoute      ErrandDeliveryRoute
	DeliveryToken       string
	DeliveredAt         *time.Time
	LatestSettledTurnID string
	Version             int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
	SettledAt           *time.Time
}

// Validate rejects an errand without its identity or with a value outside
// the closed state sets.
func (e *Errand) Validate() error {
	switch {
	case e.ID == "" || e.TaskID == "" || e.ParentSessionID == "":
		return fmt.Errorf("%w: id, task, and parent session are required", ErrInvalidErrand)
	case e.ParentSessionID == e.ID:
		return fmt.Errorf("%w: an errand cannot be its own parent", ErrInvalidErrand)
	case !slices.Contains([]ErrandLifecycle{ErrandLifecycleQueued, ErrandLifecycleLaunched, ErrandLifecycleCancelled}, e.Lifecycle):
		return fmt.Errorf("%w: lifecycle %q", ErrInvalidErrand, e.Lifecycle)
	case !slices.Contains([]ErrandCancelSource{"", ErrandCancelSourceParent, ErrandCancelSourceUser, ErrandCancelSourceCascade}, e.CancelSource):
		return fmt.Errorf("%w: cancel source %q", ErrInvalidErrand, e.CancelSource)
	case !slices.Contains([]ErrandOutcome{"", ErrandOutcomeDone, ErrandOutcomeFailed}, e.LastOutcome):
		return fmt.Errorf("%w: outcome %q", ErrInvalidErrand, e.LastOutcome)
	case !slices.Contains([]ErrandResultKind{"", ErrandResultOutput, ErrandResultFailure, ErrandResultInterrupted, ErrandResultCancelled}, e.ResultKind):
		return fmt.Errorf("%w: result kind %q", ErrInvalidErrand, e.ResultKind)
	case !slices.Contains([]ErrandResultState{ErrandResultNone, ErrandResultPending, ErrandResultDelivered, ErrandResultSuperseded, ErrandResultDiscarded}, e.ResultState):
		return fmt.Errorf("%w: result state %q", ErrInvalidErrand, e.ResultState)
	case !slices.Contains([]ErrandDeliveryRoute{"", ErrandDeliveryAwait, ErrandDeliveryTurnContext, ErrandDeliveryNotification}, e.DeliveredRoute):
		return fmt.Errorf("%w: delivery route %q", ErrInvalidErrand, e.DeliveredRoute)
	}
	return nil
}

func (s *TaskSession) addRoleAPI(result map[string]interface{}) {
	if s.Role != "" {
		result["role"] = string(s.Role)
	}
	if s.ParentSessionID != "" {
		result["parent_session_id"] = s.ParentSessionID
	}
}
