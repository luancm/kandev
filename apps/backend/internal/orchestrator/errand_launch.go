package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

func validErrandLaunchRequest(req errands.LaunchRequest) bool {
	return req.ErrandID != "" && req.TaskID != "" && req.ParentSessionID != "" && req.AgentProfileID != "" &&
		req.TurnID != "" && req.SubmissionID != "" && strings.TrimSpace(req.Objective) != ""
}

func (s *Service) validateErrandLaunchReservation(ctx context.Context, req errands.LaunchRequest, queuedOnly bool) error {
	store, ok := s.repo.(interface {
		GetErrand(context.Context, string) (*models.Errand, error)
	})
	if !ok {
		return errors.New("errand launch reservation store is unavailable")
	}
	row, err := store.GetErrand(ctx, req.ErrandID)
	if err != nil {
		return fmt.Errorf("read errand launch reservation: %w", err)
	}
	if row == nil || row.TaskID != req.TaskID || row.ParentSessionID != req.ParentSessionID ||
		row.FirstTurnID != req.TurnID || row.SubmissionID != req.SubmissionID ||
		row.AgentProfileID != req.AgentProfileID {
		return models.ErrInvalidErrand
	}
	if row.Lifecycle == models.ErrandLifecycleCancelled || (queuedOnly && row.Lifecycle != models.ErrandLifecycleQueued) {
		return errors.New("errand launch reservation is no longer runnable")
	}
	return s.validateErrandLaunchOwner(ctx, req)
}

func (s *Service) validateErrandLaunchOwner(ctx context.Context, req errands.LaunchRequest) error {
	task, err := s.repo.GetTask(ctx, req.TaskID)
	if err != nil {
		return fmt.Errorf("read errand task lifecycle: %w", err)
	}
	if task == nil || task.ID != req.TaskID || task.ArchivedAt != nil {
		return errors.New("errand task is no longer runnable")
	}
	parent, err := s.repo.GetTaskSession(ctx, req.ParentSessionID)
	if err != nil {
		return fmt.Errorf("read errand parent lifecycle: %w", err)
	}
	if parent == nil || parent.ID != req.ParentSessionID || parent.TaskID != req.TaskID ||
		parent.IsPassthrough || isTerminalSessionState(parent.State) {
		return errors.New("errand parent is no longer runnable")
	}
	return nil
}
