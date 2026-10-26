// Package errands owns the lifecycle of errands: visible child sessions that
// a task's agent starts to delegate work. It depends only on the narrow
// interfaces below; the task repository, the orchestrator, and the settings
// stores satisfy them in the composition root.
package errands

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// Store is the errand persistence plus the read-only session lookups that
// status projection needs. *sqlite.Repository satisfies it.
type Store interface {
	CreateErrand(ctx context.Context, errand *models.Errand) error
	GetErrand(ctx context.Context, id string) (*models.Errand, error)
	ListErrandsByTask(ctx context.Context, taskID string) ([]*models.Errand, error)
	ListErrandsByParent(ctx context.Context, parentSessionID string) ([]*models.Errand, error)
	ListQueuedErrands(ctx context.Context) ([]*models.Errand, error)
	CountLiveErrandsByTask(ctx context.Context, taskID string) (int, error)
	DeleteErrand(ctx context.Context, id string) error
	UpdateErrand(ctx context.Context, errand *models.Errand) error
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	GetPendingActionsBySessionIDs(ctx context.Context, sessionIDs []string) (map[string]models.TaskPendingAction, error)
}

// LaunchRequest carries everything a launcher needs to start one errand
// session. The identity fields are the errand's reserved identity.
type LaunchRequest struct {
	ErrandID        string
	TaskID          string
	ParentSessionID string
	Title           string
	Objective       string
	AgentProfileID  string
	TurnID          string
	SubmissionID    string
	// MCPServerIDs is the parent's session MCP snapshot taken at start. The
	// launcher drops definitions that are no longer enabled.
	MCPServerIDs []string
}

// Launcher starts an errand session on the task's shared workspace. It returns
// ErrCeilingRefused when the session ceiling refused the start, in which case
// it created nothing. The context carries models.ReservedLaunchIdentity.
type Launcher interface {
	LaunchErrand(ctx context.Context, request LaunchRequest) error
}

// SessionStopper stops the runtime of an errand session. It must succeed for
// a session whose runtime is already gone.
type SessionStopper interface {
	StopErrandSession(ctx context.Context, sessionID, reason string) error
}

// ParentDeliverer delivers pending errand results to a parent session. It is
// optional: a Service without one leaves results pending.
type ParentDeliverer interface {
	DeliverPending(ctx context.Context, parentSessionID string)
}

// MCPSelections reads the session-level MCP selections of a parent session.
type MCPSelections interface {
	SessionServerIDs(ctx context.Context, taskID, sessionID string) ([]string, error)
}

// ResolvedProfile is the effective agent profile of an errand.
type ResolvedProfile struct {
	ID   string
	Name string
}

// ProfileResolver resolves a profile selector (an ID or an exact name) for an
// errand on a task. An empty selector means defaultProfileID. Failures are
// *Error values with a profile reason.
type ProfileResolver interface {
	ResolveProfile(ctx context.Context, taskID, selector, defaultProfileID string) (ResolvedProfile, error)
}

// EventPublisher publishes one errand.updated event for a durable change.
type EventPublisher interface {
	PublishErrandUpdated(ctx context.Context, errand *models.Errand, status models.ErrandStatus)
}

// Clock supplies the current time.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }
