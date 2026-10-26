package errands

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// StopReasonErrandCancelled is the stop reason recorded when an errand's
// runtime is stopped because the errand was cancelled.
const StopReasonErrandCancelled = "errand_cancelled"

// CancelRequest describes one errand cancellation. TaskID is the verified
// caller task and always scopes the lookup. ParentSessionID, when set, also
// requires the errand to belong to that parent.
type CancelRequest struct {
	ErrandID        string
	TaskID          string
	ParentSessionID string
	Source          models.ErrandCancelSource
}

// Cancel cancels one errand of the caller's task. The row is marked cancelled
// before its runtime is stopped, and a repeated cancel changes no row but
// retries the stop.
func (s *Service) Cancel(ctx context.Context, req CancelRequest) (*Snapshot, error) {
	if req.TaskID == "" || req.ErrandID == "" {
		return nil, fmt.Errorf("%w: task and errand are required", ErrInvalidRequest)
	}
	errand, err := s.store.GetErrand(ctx, req.ErrandID)
	if errors.Is(err, models.ErrErrandNotFound) {
		return nil, notFound()
	}
	if err != nil {
		return nil, fmt.Errorf("load errand: %w", err)
	}
	if errand.TaskID != req.TaskID || (req.ParentSessionID != "" && errand.ParentSessionID != req.ParentSessionID) {
		return nil, notFound()
	}
	row, changed, err := s.cancelRow(ctx, errand.ID, req.Source)
	if err != nil {
		return nil, err
	}
	stopErr := s.stopRuntime(ctx, row)
	if changed && req.Source == models.ErrandCancelSourceUser {
		s.deliverPending(ctx, row.ParentSessionID)
	}
	if stopErr != nil {
		return nil, stopErr
	}
	return &Snapshot{Errand: row, Status: models.ErrandStatusCancelled}, nil
}

// CancelAll cancels every live errand of a task for the user, under the task
// mutex, and asks each affected parent to deliver the resulting notices once.
// It returns the number of errands it cancelled.
func (s *Service) CancelAll(ctx context.Context, taskID string) (int, error) {
	if taskID == "" {
		return 0, fmt.Errorf("%w: task is required", ErrInvalidRequest)
	}
	unlock := s.taskLocks.lock(taskID)
	defer unlock()

	snapshots, err := s.list(ctx, taskID)
	if err != nil {
		return 0, err
	}
	var failures []error
	cancelled := 0
	parents := map[string]struct{}{}
	for _, snapshot := range snapshots {
		if !IsLive(snapshot.Status) {
			continue
		}
		row, changed, err := s.cancelRow(ctx, snapshot.Errand.ID, models.ErrandCancelSourceUser)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if changed {
			cancelled++
			parents[row.ParentSessionID] = struct{}{}
		}
		if err := s.stopRuntime(ctx, row); err != nil {
			failures = append(failures, err)
		}
	}
	for _, parentID := range sortedKeys(parents) {
		s.deliverPending(ctx, parentID)
	}
	return cancelled, errors.Join(failures...)
}

// CancelForParent cancels every live errand of a parent session with the
// cascade source and discards the results it had not delivered. It never takes
// the task mutex, so session state hooks may call it.
func (s *Service) CancelForParent(ctx context.Context, parentSessionID string) (int, error) {
	errands, err := s.store.ListErrandsByParent(ctx, parentSessionID)
	if err != nil {
		return 0, fmt.Errorf("list errands of parent: %w", err)
	}
	return s.cascade(ctx, errands)
}

// CancelForTask cancels every live errand of a task with the cascade source,
// queued errands included, for task archive and delete.
func (s *Service) CancelForTask(ctx context.Context, taskID string) (int, error) {
	errands, err := s.store.ListErrandsByTask(ctx, taskID)
	if err != nil {
		return 0, fmt.Errorf("list errands of task: %w", err)
	}
	return s.cascade(ctx, errands)
}

func (s *Service) cascade(ctx context.Context, errands []*models.Errand) (int, error) {
	var failures []error
	cancelled := 0
	for _, errand := range errands {
		status, err := s.Project(ctx, errand)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !IsLive(status) {
			if err := s.discardPending(ctx, errand.ID); err != nil {
				failures = append(failures, err)
			}
			continue
		}
		row, changed, err := s.cancelRow(ctx, errand.ID, models.ErrandCancelSourceCascade)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if changed {
			cancelled++
		}
		if err := s.stopRuntime(ctx, row); err != nil {
			failures = append(failures, err)
		}
	}
	return cancelled, errors.Join(failures...)
}

// CancelOnSessionStop records a user stop of an errand session that the caller
// is already stopping. It does not stop the runtime again. A session that is
// not an errand, or an errand already cancelled, is left alone.
func (s *Service) CancelOnSessionStop(ctx context.Context, sessionID string) error {
	row, changed, err := s.cancelRow(ctx, sessionID, models.ErrandCancelSourceUser)
	if errors.Is(err, models.ErrErrandNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if changed {
		s.deliverPending(ctx, row.ParentSessionID)
	}
	return nil
}

// cancelRow marks an errand cancelled with a version-guarded write, publishes
// the change, and counts it. A row that is already cancelled is returned
// unchanged.
func (s *Service) cancelRow(
	ctx context.Context, id string, source models.ErrandCancelSource,
) (*models.Errand, bool, error) {
	row, changed, err := s.update(ctx, id, func(e *models.Errand) bool {
		return s.applyCancel(e, source)
	})
	if err != nil {
		if errors.Is(err, models.ErrErrandNotFound) {
			return nil, false, err
		}
		return nil, false, fmt.Errorf("cancel errand: %w", err)
	}
	if changed {
		s.events.PublishErrandUpdated(ctx, row, models.ErrandStatusCancelled)
		countSettled(models.ErrandStatusCancelled)
		s.logger.Info("errand.cancelled", s.errandFields(row, "source", string(source))...)
	}
	return row, changed, nil
}

// applyCancel moves an errand to cancelled. A user cancel of an outstanding
// errand leaves a cancellation notice as its result; a cascade discards an
// undelivered result; a parent cancel leaves no result.
func (s *Service) applyCancel(e *models.Errand, source models.ErrandCancelSource) bool {
	if e.Lifecycle == models.ErrandLifecycleCancelled {
		return false
	}
	outstanding := IsOutstanding(e)
	now := s.clock.Now()
	e.Lifecycle = models.ErrandLifecycleCancelled
	e.CancelSource = source
	e.AwaitingResult = false
	e.SettledAt = &now
	switch {
	case source == models.ErrandCancelSourceUser && outstanding:
		e.ResultKind = models.ErrandResultCancelled
		e.ResultState = models.ErrandResultPending
		e.ResultText = ""
		e.ResultTruncated = false
		e.ResultTurnID = ""
		e.DeliveredRoute, e.DeliveryToken, e.DeliveredAt = "", "", nil
	case source == models.ErrandCancelSourceCascade && e.ResultState == models.ErrandResultPending:
		e.ResultState = models.ErrandResultDiscarded
	}
	return true
}

// discardPending discards an undelivered result of an errand that is no longer
// live, because its parent can no longer receive it.
func (s *Service) discardPending(ctx context.Context, id string) error {
	row, changed, err := s.update(ctx, id, func(e *models.Errand) bool {
		if e.ResultState != models.ErrandResultPending {
			return false
		}
		e.ResultState = models.ErrandResultDiscarded
		return true
	})
	if err != nil {
		return fmt.Errorf("discard errand result: %w", err)
	}
	if changed {
		s.publish(ctx, row)
	}
	return nil
}

// stopRuntime stops the session of a launched errand. A queued errand has no
// runtime; a failure is logged and returned so a repeated cancel can retry.
func (s *Service) stopRuntime(ctx context.Context, errand *models.Errand) error {
	if errand == nil || errand.Lifecycle == models.ErrandLifecycleQueued {
		return nil
	}
	if err := s.stopper.StopErrandSession(ctx, errand.ID, StopReasonErrandCancelled); err != nil {
		s.logger.Warn("errand runtime stop failed",
			zap.String("errand_id", errand.ID), zap.String("task_id", errand.TaskID), zap.Error(err))
		return fmt.Errorf("stop errand session: %w", err)
	}
	return nil
}

func notFound() *Error {
	return newError(ReasonNotFound, "errand not found", nil)
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
