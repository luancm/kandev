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
	BeginErrandAwait(context.Context, models.ErrandAwaitIdentity) error
	OfferErrandAwait(context.Context, models.ErrandAwaitIdentity, []models.ErrandAwaitDeliveryItem) error
	AbortErrandAwait(context.Context, models.ErrandAwaitIdentity) ([]*models.Errand, error)
	ReserveErrandParentAdmission(context.Context, models.ErrandParentAdmission) (models.ErrandAdmissionReservation, error)
	DecideErrandParentAdmission(context.Context, models.ErrandParentAdmission, models.ErrandAdmissionState) error
	ListUnresolvedErrandParentAdmissions(context.Context, string) ([]models.ErrandParentAdmission, error)
	ReleaseErrandResultClaims(context.Context, string, string, []models.ErrandResultClaim) ([]*models.Errand, error)
	ListErrandResultClaims(context.Context, string, string, string) ([]models.ErrandResultClaim, error)
	AbortAwait(context.Context, string, string, string) ([]*models.Errand, error)
	ClaimResults(ctx context.Context, taskID, parentID string, route models.ErrandDeliveryRoute, token string, candidates []models.ErrandClaimCandidate, at time.Time, inventory []models.ErrandClaimCandidate) ([]*models.Errand, error)
	CreateErrand(ctx context.Context, errand *models.Errand) error
	GetErrand(ctx context.Context, id string) (*models.Errand, error)
	ListErrandsByTask(ctx context.Context, taskID string) ([]*models.Errand, error)
	ListErrandsByParent(ctx context.Context, parentSessionID string) ([]*models.Errand, error)
	ListQueuedErrands(ctx context.Context) ([]*models.Errand, error)
	ListErrands(ctx context.Context) ([]*models.Errand, error)
	CountLiveErrandsByTask(ctx context.Context, taskID string) (int, error)
	DeleteErrand(ctx context.Context, id string) error
	UpdateErrand(ctx context.Context, errand *models.Errand) error
	GetTask(ctx context.Context, id string) (*models.Task, error)
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

type LaunchDisposition string

const (
	LaunchQueued   LaunchDisposition = "queued"
	LaunchAdmitted LaunchDisposition = "admitted"
	LaunchSettled  LaunchDisposition = "settled"
	LaunchBlocked  LaunchDisposition = "blocked"
)

// LaunchResult binds the decision to verified reserved identities. Admitted
// includes uncertain dispatch and never authorizes another prompt.
type LaunchResult struct {
	Disposition  LaunchDisposition
	SessionID    string
	TurnID       string
	SubmissionID string
}

// ErrLaunchRejected certifies a positively proved failure before admission.
// Inspection errors and unconfirmed dispatch must never use this type.
type ErrLaunchRejected struct{ Err error }

func (e *ErrLaunchRejected) Error() string { return "errand launch rejected: " + e.Err.Error() }
func (e *ErrLaunchRejected) Unwrap() error { return e.Err }

// Launcher inspects or launches the same reservation under admission ownership.
// Queued is a definite capacity refusal before effects. Blocked plus an error
// retains unknown evidence without manufacturing a failure result.
type Launcher interface {
	LaunchErrand(ctx context.Context, request LaunchRequest) (LaunchResult, error)
}

// The private teardown seam shares the storage model's immutable identity.
type ErrandTeardownTarget = models.ErrandTeardownTarget
type ErrandTeardownIntent = models.ErrandTeardownIntent

type cancellationTargetFenceKey struct{}

// WithCancellationTargetFence keeps a deletion handoff bound to the target
// captured before releasing admission. Only a trusted lifecycle caller sets it.
func WithCancellationTargetFence(ctx context.Context, target ErrandTeardownTarget) context.Context {
	return context.WithValue(ctx, cancellationTargetFenceKey{}, target)
}

type TeardownOutcome string

const (
	TeardownCompleted  TeardownOutcome = "completed"
	TeardownSuperseded TeardownOutcome = "superseded"
)

// ErrandTeardown serializes target capture and cancellation with prompt
// admission. commit performs only the durable cancellation/intent CAS. Callers
// publish and wake awaiters after this method releases admission ownership.
// A nil error certifies a durable terminal receipt, never a scheduled stop.
type ErrandTeardown interface {
	CancelErrandWithTarget(ctx context.Context, row *models.Errand, commit func(ErrandTeardownTarget) error) error
	RetryErrandTeardown(ctx context.Context, intent ErrandTeardownIntent) (TeardownOutcome, error)
}

// TeardownIntentStore is an additive capability until lifecycle callers migrate.
type TeardownIntentStore interface {
	GetErrand(context.Context, string) (*models.Errand, error)
	TransitionErrandTeardownIntent(context.Context, string, string, int64, models.ErrandTeardownIntentState) (*models.Errand, error)
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
