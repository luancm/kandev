package errands

import (
	"context"
	"errors"
	"fmt"
	"sort"

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
// before its runtime is stopped. Repeated calls retry only a pending intent.
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
	if changed && req.Source == models.ErrandCancelSourceUser {
		s.deliverPending(ctx, row.ParentSessionID)
	}
	if err != nil {
		return nil, err
	}
	status, err := s.Project(ctx, row)
	if err != nil {
		return nil, err
	}
	return &Snapshot{Errand: row, Status: status}, nil
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

	rows, err := s.store.ListErrandsByTask(ctx, taskID)
	if err != nil {
		return 0, fmt.Errorf("list errands for cancellation: %w", err)
	}
	return s.cancelRows(ctx, rows, models.ErrandCancelSourceUser)
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
	return s.cancelRows(ctx, errands, models.ErrandCancelSourceCascade)
}

// cancelRows inspects private teardown obligations before public projection.
// A failed row leaves siblings eligible and never erases a committed decision.
func (s *Service) cancelRows(ctx context.Context, rows []*models.Errand, source models.ErrandCancelSource) (int, error) {
	var failures []error
	cancelled := 0
	parents := map[string]struct{}{}
	for _, observed := range rows {
		row, changed, err := s.cancelObserved(ctx, observed, source)
		if changed {
			cancelled++
			if source == models.ErrandCancelSourceUser {
				parents[row.ParentSessionID] = struct{}{}
			}
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("errand %s: %w", observed.ID, err))
		}
	}
	for _, parentID := range sortedKeys(parents) {
		s.deliverPending(ctx, parentID)
	}
	return cancelled, errors.Join(failures...)
}

func (s *Service) cancelObserved(ctx context.Context, row *models.Errand, source models.ErrandCancelSource) (*models.Errand, bool, error) {
	if pendingCancellation(row) {
		return row, false, s.retryTeardown(ctx, row)
	}
	if row.Lifecycle == models.ErrandLifecycleCancelled {
		return row, false, nil
	}
	if !pendingTeardown(row) {
		status, err := s.Project(ctx, row)
		if err != nil {
			return row, false, err
		}
		if !IsLive(status) {
			if source == models.ErrandCancelSourceCascade {
				err = s.discardPending(ctx, row.ID)
			}
			return row, false, err
		}
	}
	return s.cancelRow(ctx, row.ID, source)
}

// CancelOnSessionStop owns the complete errand stop. The generic stop caller
// must return its result rather than stopping the same session again.
func (s *Service) CancelOnSessionStop(ctx context.Context, sessionID string) error {
	row, changed, err := s.cancelRow(ctx, sessionID, models.ErrandCancelSourceUser)
	if errors.Is(err, models.ErrErrandNotFound) {
		return nil
	}
	if changed {
		s.deliverPending(ctx, row.ParentSessionID)
	}
	return err
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
	case source == models.ErrandCancelSourceUser && outstanding && s.featureEnabled():
		e.ResultKind = models.ErrandResultCancelled
		e.ResultState = models.ErrandResultPending
		e.ResultText = cancellationNotice
		e.ResultTruncated = false
		e.ResultTurnID = ""
		e.DeliveredRoute, e.DeliveryToken, e.DeliveredAt = "", "", nil
	case source == models.ErrandCancelSourceUser && outstanding:
		e.ResultKind = models.ErrandResultCancelled
		e.ResultState = models.ErrandResultDiscarded
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
