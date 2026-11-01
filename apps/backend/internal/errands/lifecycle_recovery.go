package errands

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type lifecycleTask struct {
	live bool
	err  error
}

type lifecycleParent struct {
	session *models.TaskSession
	err     error
}

type lifecycleInventory struct {
	live     map[string]bool
	terminal map[string]bool
	blocked  map[string]bool
	err      error
}

func (v *lifecycleInventory) permits(row *models.Errand) bool {
	return v.live[row.ID] && !v.blocked[row.TaskID]
}

func (s *Service) readTaskLifecycle(ctx context.Context, taskID string) lifecycleTask {
	task, err := s.store.GetTask(ctx, taskID)
	if errors.Is(err, repoerrors.ErrTaskNotFound) || errors.Is(err, sql.ErrNoRows) {
		return lifecycleTask{}
	}
	if err != nil {
		return lifecycleTask{err: fmt.Errorf("task %s lifecycle: %w", taskID, err)}
	}
	return lifecycleTask{live: task != nil && task.ID == taskID && task.ArchivedAt == nil}
}

func (s *Service) readParentLifecycle(ctx context.Context, parentID string) lifecycleParent {
	parent, err := s.store.GetTaskSession(ctx, parentID)
	if isMissingSession(err) {
		return lifecycleParent{}
	}
	if err != nil {
		return lifecycleParent{err: fmt.Errorf("parent %s lifecycle: %w", parentID, err)}
	}
	return lifecycleParent{session: parent}
}

func liveLifecycleParent(parent *models.TaskSession, row *models.Errand) bool {
	return parent != nil && parent.ID == row.ParentSessionID && parent.TaskID == row.TaskID &&
		parent.Role == "" && parent.ParentSessionID == "" && !parent.IsPassthrough && !isTerminalParent(parent.State)
}

// Each durable task and parent is read at most once per inventory, independent
// of retained history size. Unknown ownership blocks that task for this pass.
func (s *Service) inspectLifecycle(ctx context.Context, rows []*models.Errand) *lifecycleInventory {
	v := &lifecycleInventory{live: map[string]bool{}, terminal: map[string]bool{}, blocked: map[string]bool{}}
	tasks, parents := map[string]lifecycleTask{}, map[string]lifecycleParent{}
	for _, row := range rows {
		task, found := tasks[row.TaskID]
		if !found {
			task = s.readTaskLifecycle(ctx, row.TaskID)
			tasks[row.TaskID] = task
			v.err = errors.Join(v.err, task.err)
		}
		if task.err != nil {
			v.blocked[row.TaskID] = true
			continue
		}
		if !task.live {
			v.terminal[row.ID] = true
			continue
		}
		parent, found := parents[row.ParentSessionID]
		if !found {
			parent = s.readParentLifecycle(ctx, row.ParentSessionID)
			parents[row.ParentSessionID] = parent
			v.err = errors.Join(v.err, parent.err)
		}
		if parent.err != nil {
			v.blocked[row.TaskID] = true
			continue
		}
		v.live[row.ID] = liveLifecycleParent(parent.session, row)
		v.terminal[row.ID] = !v.live[row.ID]
	}
	return v
}

// ReconcileLifecycle reconstructs cascades lost between a durable owner
// transition and its in-memory hint. Startup and the existing ceiling sweep
// call it after authenticated runtime recovery, including with the flag off.
func (s *Service) ReconcileLifecycle(ctx context.Context) error {
	_, err := s.reconcileLifecycle(ctx)
	return err
}

func (s *Service) reconcileLifecycle(ctx context.Context) (*lifecycleInventory, error) {
	rows, err := s.store.ListErrands(ctx)
	if err != nil {
		return nil, fmt.Errorf("list errands for lifecycle recovery: %w", err)
	}
	v := s.inspectLifecycle(ctx, rows)
	for _, row := range rows {
		if !v.terminal[row.ID] {
			continue
		}
		var cancelErr error
		if row.Lifecycle != models.ErrandLifecycleCancelled && !pendingTeardown(row) {
			_, _, cancelErr = s.cancelObserved(ctx, row, models.ErrandCancelSourceCascade)
		}
		// A prior user cancellation may retain a notice while its independent
		// teardown is still pending. A dead owner cannot receive that notice.
		discardErr := s.discardPending(ctx, row.ID)
		if err := errors.Join(cancelErr, discardErr); err != nil {
			v.blocked[row.TaskID] = true
			v.err = errors.Join(v.err, fmt.Errorf("errand %s lifecycle recovery: %w", row.ID, err))
		}
	}
	if v.err != nil {
		s.logger.Warn("errand lifecycle recovery has unresolved ownership or cancellation", zap.Error(v.err))
	}
	return v, v.err
}

// checkLaunchOwner runs inside the existing per-errand launch reservation.
// The production launcher repeats this check inside its admission ownership.
func (s *Service) checkLaunchOwner(ctx context.Context, row *models.Errand) (bool, error) {
	task := s.readTaskLifecycle(ctx, row.TaskID)
	if task.err != nil || !task.live {
		return false, task.err
	}
	parent := s.readParentLifecycle(ctx, row.ParentSessionID)
	return liveLifecycleParent(parent.session, row), parent.err
}
