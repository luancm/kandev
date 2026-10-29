package errands

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

type memoryStore struct {
	mu       sync.Mutex
	errands  map[string]*models.Errand
	sessions map[string]*models.TaskSession
	pending  map[string]models.TaskPendingAction
	updates  int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		errands:  map[string]*models.Errand{},
		sessions: map[string]*models.TaskSession{},
		pending:  map[string]models.TaskPendingAction{},
	}
}

func (s *memoryStore) addSession(session *models.TaskSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
}

func (s *memoryStore) setSessionState(id string, state models.TaskSessionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id].State = state
}

func (s *memoryStore) CreateErrand(_ context.Context, errand *models.Errand) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.errands[errand.ID]; exists {
		return errors.New("duplicate errand")
	}
	row := *errand
	row.Version = 1
	s.errands[row.ID] = &row
	errand.Version = 1
	return nil
}

func (s *memoryStore) GetErrand(_ context.Context, id string) (*models.Errand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.errands[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", models.ErrErrandNotFound, id)
	}
	copied := *row
	return &copied, nil
}

func (s *memoryStore) sorted(match func(*models.Errand) bool) []*models.Errand {
	var rows []*models.Errand
	for _, row := range s.errands {
		if match(row) {
			copied := *row
			rows = append(rows, &copied)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TaskID != rows[j].TaskID {
			return rows[i].TaskID < rows[j].TaskID
		}
		if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].CreatedAt.Before(rows[j].CreatedAt)
		}
		return rows[i].ID < rows[j].ID
	})
	return rows
}

func (s *memoryStore) ListErrandsByTask(_ context.Context, taskID string) ([]*models.Errand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sorted(func(e *models.Errand) bool { return e.TaskID == taskID }), nil
}

func (s *memoryStore) ListErrandsByParent(_ context.Context, parentID string) ([]*models.Errand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sorted(func(e *models.Errand) bool { return e.ParentSessionID == parentID }), nil
}

func (s *memoryStore) ListQueuedErrands(_ context.Context) ([]*models.Errand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sorted(func(e *models.Errand) bool { return e.Lifecycle == models.ErrandLifecycleQueued }), nil
}

func (s *memoryStore) ListErrands(context.Context) ([]*models.Errand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sorted(func(*models.Errand) bool { return true }), nil
}

// CountLiveErrandsByTask counts errands whose projected status is live.
func (s *memoryStore) CountLiveErrandsByTask(_ context.Context, taskID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	live := 0
	for _, row := range s.errands {
		if row.TaskID == taskID && IsLive(ProjectStatus(row, s.sessions[row.ID], false)) {
			live++
		}
	}
	return live, nil
}

func (s *memoryStore) DeleteErrand(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.errands, id)
	return nil
}

func (s *memoryStore) UpdateErrand(_ context.Context, errand *models.Errand) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.errands[errand.ID]
	if !ok {
		return fmt.Errorf("%w: %s", models.ErrErrandNotFound, errand.ID)
	}
	if row.Version != errand.Version {
		return fmt.Errorf("%w: %s", models.ErrErrandVersionConflict, errand.ID)
	}
	next := *errand
	next.Version++
	s.errands[errand.ID] = &next
	errand.Version = next.Version
	s.updates++
	return nil
}

func (s *memoryStore) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", models.ErrTaskSessionNotFound, id)
	}
	copied := *session
	return &copied, nil
}

func (s *memoryStore) GetPendingActionsBySessionIDs(_ context.Context, ids []string) (map[string]models.TaskPendingAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := map[string]models.TaskPendingAction{}
	for _, id := range ids {
		if action, ok := s.pending[id]; ok {
			result[id] = action
		}
	}
	return result, nil
}

type fakeLauncher struct {
	mu       sync.Mutex
	store    *memoryStore
	requests []LaunchRequest
	identity []models.ReservedLaunchIdentity
	// behave decides the result per call; nil launches successfully.
	behave func(call int, request LaunchRequest) error
	// onCall runs inside the launch, before it returns.
	onCall func(request LaunchRequest)
}

func (l *fakeLauncher) LaunchErrand(ctx context.Context, request LaunchRequest) error {
	l.mu.Lock()
	call := len(l.requests)
	l.requests = append(l.requests, request)
	identity, _ := models.ReservedLaunchIdentityFromContext(ctx)
	l.identity = append(l.identity, identity)
	l.mu.Unlock()
	if l.onCall != nil {
		l.onCall(request)
	}
	if l.behave != nil {
		if err := l.behave(call, request); err != nil {
			return err
		}
	}
	l.store.addSession(&models.TaskSession{
		ID: request.ErrandID, TaskID: request.TaskID, Role: models.SessionRoleErrand,
		ParentSessionID: request.ParentSessionID, State: models.TaskSessionStateRunning,
	})
	return nil
}

func (l *fakeLauncher) calls() []LaunchRequest {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]LaunchRequest(nil), l.requests...)
}

type fakeStopper struct {
	mu        sync.Mutex
	store     *memoryStore
	stopped   []string
	reasons   []string
	rowAtStop map[string]models.ErrandLifecycle
	err       error
}

func (s *fakeStopper) StopErrandSession(ctx context.Context, sessionID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = append(s.stopped, sessionID)
	s.reasons = append(s.reasons, reason)
	if s.rowAtStop == nil {
		s.rowAtStop = map[string]models.ErrandLifecycle{}
	}
	if row, err := s.store.GetErrand(ctx, sessionID); err == nil {
		s.rowAtStop[sessionID] = row.Lifecycle
	}
	return s.err
}

func (s *fakeStopper) stops() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.stopped...)
}

type fakeProfiles struct {
	mu       sync.Mutex
	resolved ResolvedProfile
	err      error
	selector string
	defaultP string
}

func (p *fakeProfiles) CompatibleProfiles(context.Context, string) ([]ProfileInfo, error) {
	return nil, nil
}

func (p *fakeProfiles) ResolveProfile(_ context.Context, _ string, selector, defaultProfileID string) (ResolvedProfile, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.selector, p.defaultP = selector, defaultProfileID
	return p.resolved, p.err
}

type fakeMCP struct {
	ids []string
	err error
}

func (m *fakeMCP) SessionServerIDs(context.Context, string, string) ([]string, error) {
	return m.ids, m.err
}

type publishedEvent struct {
	errand models.Errand
	status models.ErrandStatus
}

type recordingPublisher struct {
	mu     sync.Mutex
	events []publishedEvent
	log    *[]string
}

func (p *recordingPublisher) PublishErrandUpdated(_ context.Context, errand *models.Errand, status models.ErrandStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, publishedEvent{errand: *errand, status: status})
	if p.log != nil {
		*p.log = append(*p.log, "publish:"+string(status))
	}
}

func (p *recordingPublisher) statuses() []models.ErrandStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	statuses := make([]models.ErrandStatus, 0, len(p.events))
	for _, event := range p.events {
		statuses = append(statuses, event.status)
	}
	return statuses
}

type recordingDeliverer struct {
	mu               sync.Mutex
	parents          []string
	prepared         []string
	preparedMetadata []map[string]interface{}
	blocked          bool
	admitErr         error
}

func (d *recordingDeliverer) NotificationBlocked(context.Context, string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.blocked
}

func (d *recordingDeliverer) DeliverPending(_ context.Context, parentSessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.parents = append(d.parents, parentSessionID)
}

func (d *recordingDeliverer) AdmitPrepared(_ context.Context, parentSessionID, content string, metadata map[string]interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.prepared = append(d.prepared, parentSessionID+"\n"+content)
	d.preparedMetadata = append(d.preparedMetadata, metadata)
	return d.admitErr
}

func (d *recordingDeliverer) delivered() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.parents...)
}

type steppingClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *steppingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(time.Second)
	return c.now
}

type harness struct {
	t         *testing.T
	store     *memoryStore
	launcher  *fakeLauncher
	stopper   *fakeStopper
	profiles  *fakeProfiles
	mcp       *fakeMCP
	publisher *recordingPublisher
	deliverer *recordingDeliverer
	service   *Service
}

const (
	testTask   = "task-1"
	testParent = "parent-1"
)

func newHarness(t *testing.T) *harness {
	t.Helper()
	store := newMemoryStore()
	h := &harness{
		t: t, store: store,
		launcher:  &fakeLauncher{store: store},
		stopper:   &fakeStopper{store: store},
		profiles:  &fakeProfiles{resolved: ResolvedProfile{ID: "profile-1", Name: "Reviewer"}},
		mcp:       &fakeMCP{ids: []string{"mcp-1", "mcp-2"}},
		publisher: &recordingPublisher{},
		deliverer: &recordingDeliverer{},
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	service, err := New(Dependencies{
		Store: store, Launcher: h.launcher, Stopper: h.stopper, Profiles: h.profiles, MCP: h.mcp,
		Events: h.publisher, Deliverer: h.deliverer,
		Clock:  &steppingClock{now: time.Date(2026, time.October, 26, 9, 0, 0, 0, time.UTC)},
		Logger: log,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h.service = service
	store.addSession(&models.TaskSession{
		ID: testParent, TaskID: testTask, AgentProfileID: "parent-profile", State: models.TaskSessionStateWaitingForInput,
	})
	return h
}

func (h *harness) start(title string) (*StartResult, error) {
	return h.service.Start(context.Background(), StartRequest{
		TaskID: testTask, ParentSessionID: testParent, Title: title, Objective: "Do " + title,
	})
}

func (h *harness) mustStart(title string) *StartResult {
	h.t.Helper()
	result, err := h.start(title)
	if err != nil {
		h.t.Fatalf("Start(%s): %v", title, err)
	}
	return result
}
