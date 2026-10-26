package errands

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// Dependencies are the collaborators of a Service. Store, Launcher,
// SessionStopper, ProfileResolver, MCPSelections, and EventPublisher are
// required; Deliverer and Clock are optional.
type Dependencies struct {
	Store     Store
	Launcher  Launcher
	Stopper   SessionStopper
	Profiles  ProfileResolver
	MCP       MCPSelections
	Events    EventPublisher
	Deliverer ParentDeliverer
	Clock     Clock
	Logger    *logger.Logger
}

// Service starts, projects, and cancels errands.
type Service struct {
	store     Store
	launcher  Launcher
	stopper   SessionStopper
	profiles  ProfileResolver
	mcp       MCPSelections
	events    EventPublisher
	deliverer ParentDeliverer
	clock     Clock
	logger    *logger.Logger

	taskLocks   taskLockSet
	parentLocks taskLockSet
	launching   launchGuard
	cascades    cascadeWorker
	awaits      awaitRegistry

	turns    TurnReader
	receipts DeliveryLog
	enabled  func() bool
}

// Snapshot is an errand row with its projected status.
type Snapshot struct {
	Errand *models.Errand
	Status models.ErrandStatus
}

// New builds a Service. It does not start its cascade worker; call
// StartWorkers.
func New(deps Dependencies) (*Service, error) {
	switch {
	case deps.Store == nil:
		return nil, errors.New("errands: store is required")
	case deps.Launcher == nil:
		return nil, errors.New("errands: launcher is required")
	case deps.Stopper == nil:
		return nil, errors.New("errands: session stopper is required")
	case deps.Profiles == nil:
		return nil, errors.New("errands: profile resolver is required")
	case deps.MCP == nil:
		return nil, errors.New("errands: MCP selection reader is required")
	case deps.Events == nil:
		return nil, errors.New("errands: event publisher is required")
	}
	if deps.Clock == nil {
		deps.Clock = systemClock{}
	}
	if deps.Logger == nil {
		deps.Logger = logger.Default()
	}
	return &Service{
		store: deps.Store, launcher: deps.Launcher, stopper: deps.Stopper, profiles: deps.Profiles,
		mcp: deps.MCP, events: deps.Events, deliverer: deps.Deliverer, clock: deps.Clock,
		logger: deps.Logger.WithFields(zap.String("component", "errands")),
	}, nil
}

// SetDeliverer wires the parent delivery seam after construction.
func (s *Service) SetDeliverer(deliverer ParentDeliverer) {
	s.deliverer = deliverer
}

// SetEnabled reports whether features.errands is on. A nil gate leaves the
// service enabled, which unit tests rely on; production sets the flag.
func (s *Service) SetEnabled(enabled func() bool) {
	s.enabled = enabled
}

// SetTurnReader wires transcript reads for result capture and reconciliation.
func (s *Service) SetTurnReader(reader TurnReader) {
	s.turns = reader
}

// SetDeliveryLog wires the read that proves a delivery token reached a parent
// message. Without it, reconciliation leaves delivered rows in place.
func (s *Service) SetDeliveryLog(log DeliveryLog) {
	s.receipts = log
}

func (s *Service) featureEnabled() bool {
	if s == nil || s.enabled == nil {
		return true
	}
	return s.enabled()
}

// StartWorkers launches the worker that stops runtimes for cascades triggered
// from session state transitions. It is idempotent.
func (s *Service) StartWorkers(ctx context.Context) {
	s.cascades.start(ctx, s.runCascade)
}

// StopWorkers ends the cascade worker and waits for it to drain. It is
// idempotent.
func (s *Service) StopWorkers() {
	s.cascades.stop()
}

// Project returns an errand's projected status.
func (s *Service) Project(ctx context.Context, errand *models.Errand) (models.ErrandStatus, error) {
	var session *models.TaskSession
	if errand.Lifecycle == models.ErrandLifecycleLaunched {
		loaded, err := s.store.GetTaskSession(ctx, errand.ID)
		if err != nil && !isMissingSession(err) {
			return "", fmt.Errorf("load errand session %s: %w", errand.ID, err)
		}
		session = loaded
	}
	permission := false
	if session != nil && isWorking(session) {
		actions, err := s.store.GetPendingActionsBySessionIDs(ctx, []string{session.ID})
		if err != nil {
			return "", fmt.Errorf("load pending action for errand %s: %w", errand.ID, err)
		}
		permission = actions[session.ID] == models.TaskPendingActionPermission
	}
	return ProjectStatus(errand, session, permission), nil
}

// publish emits errand.updated for a durable change. The status is projected
// from current state; a projection failure is logged and publishes nothing.
func (s *Service) publish(ctx context.Context, errand *models.Errand) {
	status, err := s.Project(ctx, errand)
	if err != nil {
		s.logger.Warn("errand status projection failed; errand.updated not published",
			zap.String("errand_id", errand.ID), zap.Error(err))
		return
	}
	s.events.PublishErrandUpdated(ctx, errand, status)
}

type taskLockSet struct {
	mu    sync.Mutex
	locks map[string]*taskLock
}

type taskLock struct {
	mu   sync.Mutex
	refs int
}

// lock serializes cap checks and bulk cancellation per task. The returned
// function releases it.
func (t *taskLockSet) lock(taskID string) func() {
	t.mu.Lock()
	if t.locks == nil {
		t.locks = make(map[string]*taskLock)
	}
	entry := t.locks[taskID]
	if entry == nil {
		entry = &taskLock{}
		t.locks[taskID] = entry
	}
	entry.refs++
	t.mu.Unlock()
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		t.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(t.locks, taskID)
		}
		t.mu.Unlock()
	}
}

// launchGuard prevents a queued errand from being launched twice at once, by
// its Start call and by a concurrent queue drain.
type launchGuard struct {
	mu       sync.Mutex
	inflight map[string]struct{}
}

func (g *launchGuard) acquire(errandID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inflight == nil {
		g.inflight = make(map[string]struct{})
	}
	if _, busy := g.inflight[errandID]; busy {
		return false
	}
	g.inflight[errandID] = struct{}{}
	return true
}

func (g *launchGuard) release(errandID string) {
	g.mu.Lock()
	delete(g.inflight, errandID)
	g.mu.Unlock()
}
