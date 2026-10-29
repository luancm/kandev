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
	ListErrands(ctx context.Context) ([]*models.Errand, error)
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
	MCPServerIDs         []string
	MCPExcludedServerIDs []string
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

// ParentDeliverer admits errand notifications onto a parent's queue. It is
// optional: a Service without one leaves results pending. DeliverPending and
// AdmitPrepared must not call back into the service.
type ParentDeliverer interface {
	// NotificationBlocked reports that a parent turn is running or any queue
	// entry, a notification included, is already waiting.
	NotificationBlocked(ctx context.Context, parentSessionID string) bool
	// DeliverPending admits one empty server notification. A refusal, including
	// a full queue, leaves results pending.
	DeliverPending(ctx context.Context, parentSessionID string)
	// AdmitPrepared admits one notification whose content is already the
	// status block, used when the errand row will not survive until dispatch.
	AdmitPrepared(ctx context.Context, parentSessionID, content string, metadata map[string]interface{}) error
}

// MCPSnapshot captures session additions and exclusions at errand admission.
type MCPSnapshot struct {
	DefinitionIDs         []string
	ExcludedDefinitionIDs []string
}

// MCPSelections reads the session-level MCP selections of a parent session.
type MCPSelections interface {
	SessionSnapshot(ctx context.Context, taskID, sessionID string) (MCPSnapshot, error)
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
	// CompatibleProfiles lists the profiles a parent may offer for errands.
	CompatibleProfiles(ctx context.Context, taskID string) ([]ProfileInfo, error)
}

// EventPublisher publishes one errand.updated event for a durable change.
type EventPublisher interface {
	PublishErrandUpdated(ctx context.Context, errand *models.Errand, status models.ErrandStatus)
}

// TurnView is the transcript evidence of one errand turn.
type TurnView struct {
	LastAgentText      string
	EarliestUserSender string
	Completed          bool
}

// TurnReader reads errand transcript evidence. The orchestrator satisfies it
// from task messages. A nil reader treats only the reserved first turn as
// parent-initiated and captures no assistant text.
type TurnReader interface {
	ReadTurn(ctx context.Context, sessionID, turnID string) (TurnView, error)
	LatestTurn(ctx context.Context, sessionID string) (turnID string, view TurnView, err error)
}

// DeliveryLog reports whether a parent message contains a delivery token.
// Reconciliation uses it to return claims whose token never landed.
type DeliveryLog interface {
	HasDeliveryToken(ctx context.Context, parentSessionID, token string) (bool, error)
}

// Clock supplies the current time.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }
