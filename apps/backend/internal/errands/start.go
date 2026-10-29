package errands

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// StartRequest describes one errand start. ParentSessionID is the verified
// caller session, never a value taken from tool arguments.
type StartRequest struct {
	TaskID          string
	ParentSessionID string
	Title           string
	Objective       string
	// Profile is an agent profile ID or exact name. Empty selects the
	// parent session's profile.
	Profile string
}

// StartResult is the outcome of a start that created an errand.
type StartResult struct {
	Snapshot
	ProfileName string
}

// Start creates an errand and launches its session. It returns without
// waiting for the errand's first turn. A ceiling refusal leaves the errand
// queued; a synchronous launch failure that created no session removes the
// errand and returns the launch error unchanged for the caller to classify.
func (s *Service) Start(ctx context.Context, req StartRequest) (*StartResult, error) {
	req.TaskID = strings.TrimSpace(req.TaskID)
	req.ParentSessionID = strings.TrimSpace(req.ParentSessionID)
	req.Title = strings.TrimSpace(req.Title)
	req.Objective = strings.TrimSpace(req.Objective)
	if req.TaskID == "" || req.ParentSessionID == "" || req.Title == "" || req.Objective == "" {
		startErr := fmt.Errorf("%w: task, parent session, title, and prompt are required", ErrInvalidRequest)
		countStarted(StartOutcomeRejected)
		s.logStartRequest(req, StartOutcomeRejected, startErr)
		return nil, startErr
	}
	prepared, err := s.prepareStart(ctx, req)
	if err != nil {
		return nil, err
	}
	errand, err := s.admit(ctx, prepared)
	if err != nil {
		return nil, err
	}
	defer s.launching.release(errand.ID)

	s.publish(ctx, errand)
	outcome, launchErr := s.launchQueued(ctx, errand, true)
	if launchErr != nil {
		return nil, launchErr
	}
	return &StartResult{Snapshot: outcome.snapshot, ProfileName: prepared.profile.Name}, nil
}

type preparedStart struct {
	req     StartRequest
	parent  *models.TaskSession
	profile ResolvedProfile
	mcp     []string
}

func (s *Service) prepareStart(ctx context.Context, req StartRequest) (*preparedStart, error) {
	parent, err := s.loadParent(ctx, req)
	if err != nil {
		countStarted(StartOutcomeRejected)
		s.logStartRequest(req, StartOutcomeRejected, err)
		return nil, err
	}
	profile, err := s.profiles.ResolveProfile(ctx, req.TaskID, req.Profile, parent.AgentProfileID)
	if err != nil {
		countStarted(StartOutcomeRejected)
		s.logStartRequest(req, StartOutcomeRejected, err)
		return nil, err
	}
	mcp, err := s.mcp.SessionServerIDs(ctx, req.TaskID, parent.ID)
	if err != nil {
		countStarted(StartOutcomeFailed)
		startErr := fmt.Errorf("read parent MCP selections: %w", err)
		s.logStartRequest(req, StartOutcomeFailed, startErr)
		return nil, startErr
	}
	return &preparedStart{req: req, parent: parent, profile: profile, mcp: mcp}, nil
}

// loadParent verifies the caller session: it must exist on the task and be an
// ordinary, non-passthrough session. Errands never start errands.
func (s *Service) loadParent(ctx context.Context, req StartRequest) (*models.TaskSession, error) {
	parent, err := s.store.GetTaskSession(ctx, req.ParentSessionID)
	if err != nil && !isMissingSession(err) {
		return nil, fmt.Errorf("load parent session: %w", err)
	}
	if parent == nil || parent.TaskID != req.TaskID || models.IsErrandSession(parent) || parent.IsPassthrough {
		return nil, newError(ReasonNotSupported, "this session cannot start errands", nil)
	}
	return parent, nil
}

// admit enforces the live limit and inserts the queued row under the task
// mutex, so concurrent starts cannot exceed the limit. The returned errand is
// held by the launch guard until the caller releases it.
func (s *Service) admit(ctx context.Context, prepared *preparedStart) (*models.Errand, error) {
	unlock := s.taskLocks.lock(prepared.req.TaskID)
	defer unlock()

	live, err := s.store.CountLiveErrandsByTask(ctx, prepared.req.TaskID)
	if err != nil {
		countStarted(StartOutcomeFailed)
		startErr := fmt.Errorf("count live errands: %w", err)
		s.logStartRequest(prepared.req, StartOutcomeFailed, startErr)
		return nil, startErr
	}
	if live >= MaxLiveErrands {
		countStarted(StartOutcomeLimitReached)
		s.logger.Info("errand.started",
			zap.String("task_id", prepared.req.TaskID), zap.String("parent_session_id", prepared.req.ParentSessionID),
			zap.String("outcome", string(StartOutcomeLimitReached)), zap.Int("live", live))
		return nil, newError(ReasonLimitReached, fmt.Sprintf("a task can hold at most %d live errands", MaxLiveErrands),
			map[string]interface{}{"limit": MaxLiveErrands, "live": live})
	}
	errand := newQueuedErrand(prepared, s.clock.Now())
	s.launching.acquire(errand.ID)
	if err := s.store.CreateErrand(ctx, errand); err != nil {
		s.launching.release(errand.ID)
		countStarted(StartOutcomeFailed)
		s.logger.Warn("errand.started", s.errandFields(errand,
			"outcome", string(StartOutcomeFailed), "error", oneLine(err.Error(), 300))...)
		return nil, fmt.Errorf("create errand: %w", err)
	}
	return errand, nil
}

func newQueuedErrand(prepared *preparedStart, now time.Time) *models.Errand {
	errandID, turnID, submissionID := newErrandIdentity()
	return &models.Errand{
		ID: errandID, TaskID: prepared.req.TaskID, ParentSessionID: prepared.parent.ID,
		Title: prepared.req.Title, Objective: prepared.req.Objective,
		AgentProfileID: prepared.profile.ID, MCPServerIDs: prepared.mcp,
		FirstTurnID: turnID, SubmissionID: submissionID,
		Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone,
		AwaitingResult: true, CreatedAt: now, UpdatedAt: now,
	}
}

// launchOutcome is the durable result of one launch attempt.
type launchOutcome struct {
	snapshot Snapshot
	// queued is true when the ceiling refused the launch.
	queued bool
}

// launchQueued launches one queued errand and records the result. With
// removeOnFailure a launch failure that created no session deletes the row and
// returns the launch error; otherwise the errand becomes failed.
func (s *Service) launchQueued(ctx context.Context, errand *models.Errand, removeOnFailure bool) (launchOutcome, error) {
	launchCtx := models.WithReservedLaunchIdentity(ctx, models.ReservedLaunchIdentity{
		TaskID: errand.TaskID, SessionID: errand.ID, TurnID: errand.FirstTurnID, SubmissionID: errand.SubmissionID,
	})
	err := s.launcher.LaunchErrand(launchCtx, LaunchRequest{
		ErrandID: errand.ID, TaskID: errand.TaskID, ParentSessionID: errand.ParentSessionID,
		Title: errand.Title, Objective: errand.Objective, AgentProfileID: errand.AgentProfileID,
		TurnID: errand.FirstTurnID, SubmissionID: errand.SubmissionID, MCPServerIDs: errand.MCPServerIDs,
	})
	switch {
	case err == nil:
		return s.recordLaunched(ctx, errand)
	case errors.Is(err, ErrCeilingRefused):
		return s.recordQueued(ctx, errand)
	default:
		return s.recordLaunchFailure(ctx, errand, err, removeOnFailure)
	}
}

func (s *Service) recordQueued(ctx context.Context, errand *models.Errand) (launchOutcome, error) {
	current, err := s.store.GetErrand(ctx, errand.ID)
	if err != nil {
		return launchOutcome{}, fmt.Errorf("reload queued errand: %w", err)
	}
	status, err := s.Project(ctx, current)
	if err != nil {
		return launchOutcome{}, err
	}
	countStarted(StartOutcomeQueued)
	s.logger.Info("errand.started", s.errandFields(current, "outcome", string(StartOutcomeQueued))...)
	return launchOutcome{snapshot: Snapshot{Errand: current, Status: status}, queued: true}, nil
}

func (s *Service) recordLaunched(ctx context.Context, errand *models.Errand) (launchOutcome, error) {
	row, changed, err := s.update(ctx, errand.ID, func(e *models.Errand) bool {
		if e.Lifecycle != models.ErrandLifecycleQueued {
			return false
		}
		e.Lifecycle = models.ErrandLifecycleLaunched
		return true
	})
	if err != nil {
		return launchOutcome{}, fmt.Errorf("record launched errand: %w", err)
	}
	if row.Lifecycle == models.ErrandLifecycleCancelled {
		// A cancel won while the launch ran; stop the session it created. A
		// failure is logged by stopRuntime and a repeated cancel retries it.
		_ = s.stopRuntime(ctx, row)
	}
	status, err := s.Project(ctx, row)
	if err != nil {
		return launchOutcome{}, err
	}
	if changed {
		s.events.PublishErrandUpdated(ctx, row, status)
		countStarted(StartOutcomeStarted)
		s.logger.Info("errand.started", s.errandFields(row, "outcome", string(StartOutcomeStarted))...)
		s.logger.Info("errand.admitted", s.errandFields(row)...)
	}
	return launchOutcome{snapshot: Snapshot{Errand: row, Status: status}}, nil
}

// recordLaunchFailure settles an errand whose launch failed. A failure that
// left no session row removes the errand when the caller asked for removal.
func (s *Service) recordLaunchFailure(
	ctx context.Context, errand *models.Errand, launchErr error, removeOnFailure bool,
) (launchOutcome, error) {
	session, err := s.store.GetTaskSession(ctx, errand.ID)
	if err != nil && !isMissingSession(err) {
		return launchOutcome{}, errors.Join(launchErr, fmt.Errorf("inspect failed errand launch: %w", err))
	}
	if session == nil && removeOnFailure {
		return launchOutcome{}, s.removeFailedStart(ctx, errand, launchErr)
	}
	row, changed, err := s.update(ctx, errand.ID, func(e *models.Errand) bool {
		if e.Lifecycle == models.ErrandLifecycleCancelled {
			return false
		}
		s.applyLaunchFailure(e, launchErr)
		return true
	})
	if err != nil {
		return launchOutcome{}, errors.Join(launchErr, fmt.Errorf("record errand launch failure: %w", err))
	}
	status, err := s.Project(ctx, row)
	if err != nil {
		return launchOutcome{}, err
	}
	if changed {
		s.events.PublishErrandUpdated(ctx, row, status)
		countSettled(status)
		countStarted(StartOutcomeFailed)
		s.logger.Warn("errand.started", s.errandFields(row,
			"outcome", string(StartOutcomeFailed), "error", oneLine(launchErr.Error(), 300))...)
		s.logger.Warn("errand.settled", s.errandFields(row, "status", string(status), "error", oneLine(launchErr.Error(), 300))...)
		s.deliverPending(ctx, row.ParentSessionID)
	}
	return launchOutcome{snapshot: Snapshot{Errand: row, Status: status}}, nil
}

// removeFailedStart deletes an errand whose start failed before any session
// existed and tells clients it never ran.
func (s *Service) removeFailedStart(ctx context.Context, errand *models.Errand, launchErr error) error {
	if err := s.store.DeleteErrand(ctx, errand.ID); err != nil {
		return errors.Join(launchErr, fmt.Errorf("remove errand after failed launch: %w", err))
	}
	removed := *errand
	removed.Lifecycle = models.ErrandLifecycleLaunched
	removed.LastOutcome = models.ErrandOutcomeFailed
	removed.AwaitingResult = false
	s.events.PublishErrandUpdated(ctx, &removed, models.ErrandStatusFailed)
	countStarted(StartOutcomeFailed)
	s.logger.Warn("errand.started", s.errandFields(errand,
		"outcome", string(StartOutcomeFailed), "error", oneLine(launchErr.Error(), 300))...)
	return fmt.Errorf("launch errand: %w", launchErr)
}

// applyLaunchFailure turns a queued errand into a failed one with a failure
// result for the parent.
func (s *Service) applyLaunchFailure(e *models.Errand, launchErr error) {
	now := s.clock.Now()
	e.Lifecycle = models.ErrandLifecycleLaunched
	e.LastOutcome = models.ErrandOutcomeFailed
	e.AwaitingResult = false
	e.ResultKind = models.ErrandResultFailure
	e.ResultState = models.ErrandResultPending
	e.ResultText = "The errand could not be started: " + oneLine(launchErr.Error(), 300)
	e.ResultTruncated = false
	e.ResultTurnID = e.FirstTurnID
	e.LatestSettledTurnID = e.FirstTurnID
	e.SettledAt = &now
}

func (s *Service) errandFields(errand *models.Errand, extra ...string) []zap.Field {
	fields := []zap.Field{
		zap.String("task_id", errand.TaskID), zap.String("session_id", errand.ID),
		zap.String("errand_id", errand.ID), zap.String("parent_session_id", errand.ParentSessionID),
	}
	for i := 0; i+1 < len(extra); i += 2 {
		fields = append(fields, zap.String(extra[i], extra[i+1]))
	}
	return fields
}

func (s *Service) logStartRequest(req StartRequest, outcome StartOutcome, err error) {
	fields := []zap.Field{
		zap.String("task_id", req.TaskID), zap.String("parent_session_id", req.ParentSessionID),
		zap.String("outcome", string(outcome)),
	}
	if err != nil {
		fields = append(fields, zap.String("error", oneLine(err.Error(), 300)))
	}
	s.logger.Warn("errand.started", fields...)
}

func newErrandIdentity() (errandID, turnID, submissionID string) {
	return uuid.NewString(), uuid.NewString(), "prompt:errand:" + uuid.NewString()
}
