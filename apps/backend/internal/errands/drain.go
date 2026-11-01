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
// one. Unknown launch evidence stops only that task; a positively proved
// rejection may settle that errand and let the walk continue.
func (s *Service) DrainQueued(ctx context.Context) {
	s.drainQueued(ctx, nil)
}

func (s *Service) drainQueued(ctx context.Context, owners *lifecycleInventory) {
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
		if owners != nil && !owners.permits(errand) {
			blocked[errand.TaskID] = struct{}{}
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
	live, err := s.checkLaunchOwner(ctx, current)
	if err != nil {
		s.logger.Warn("errand queue ownership is unreadable", s.errandFields(current, "error", err.Error())...)
		return false
	}
	if !live {
		_, err := s.cascade(ctx, []*models.Errand{current})
		if err != nil {
			s.logger.Warn("errand queue cascade failed", s.errandFields(current, "error", err.Error())...)
		}
		fresh, readErr := s.store.GetErrand(ctx, current.ID)
		return readErr == nil && fresh.Lifecycle == models.ErrandLifecycleCancelled
	}
	outcome, err := s.launchQueued(ctx, current, false)
	if err != nil {
		s.logger.Warn("errand queue drain failed to record a launch",
			zap.String("errand_id", current.ID), zap.String("task_id", current.TaskID), zap.Error(err))
		return false
	}
	return !outcome.queued
}
