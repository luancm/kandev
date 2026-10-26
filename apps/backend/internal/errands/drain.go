package errands

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// DrainQueued launches queued errands that the session ceiling refused
// earlier. It walks the queue by task and creation order, reuses each row's
// reserved identity and MCP snapshot, and stops a task's walk at its first
// ceiling refusal or busy row so a later errand never overtakes an earlier
// one. Any other launch failure fails that errand and the walk continues.
func (s *Service) DrainQueued(ctx context.Context) {
	queued, err := s.store.ListQueuedErrands(ctx)
	if err != nil {
		s.logger.Warn("errand queue drain could not list queued errands", zap.Error(err))
		return
	}
	blocked := map[string]struct{}{}
	for _, errand := range queued {
		if ctx.Err() != nil {
			return
		}
		if _, stop := blocked[errand.TaskID]; stop {
			continue
		}
		if !s.drainOne(ctx, errand) {
			blocked[errand.TaskID] = struct{}{}
		}
	}
}

// drainOne reports whether the task's walk may continue past this errand.
func (s *Service) drainOne(ctx context.Context, errand *models.Errand) bool {
	if !s.launching.acquire(errand.ID) {
		return false
	}
	defer s.launching.release(errand.ID)
	current, err := s.store.GetErrand(ctx, errand.ID)
	if err != nil || current.Lifecycle != models.ErrandLifecycleQueued {
		return err == nil
	}
	outcome, err := s.launchQueued(ctx, current, false)
	if err != nil {
		s.logger.Warn("errand queue drain failed to record a launch",
			zap.String("errand_id", current.ID), zap.String("task_id", current.TaskID), zap.Error(err))
		return false
	}
	return !outcome.queued
}
