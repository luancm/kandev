package errands

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// List returns a task's errands in creation order with their projected status.
func (s *Service) List(ctx context.Context, taskID string) ([]Snapshot, error) {
	if taskID == "" {
		return nil, fmt.Errorf("%w: task is required", ErrInvalidRequest)
	}
	return s.list(ctx, taskID)
}

// Lookup returns one errand row by ID without scoping. Callers authorize the
// returned task themselves before acting on the errand.
func (s *Service) Lookup(ctx context.Context, errandID string) (*models.Errand, error) {
	errand, err := s.store.GetErrand(ctx, errandID)
	if errors.Is(err, models.ErrErrandNotFound) {
		return nil, notFound()
	}
	if err != nil {
		return nil, fmt.Errorf("load errand: %w", err)
	}
	return errand, nil
}

func (s *Service) list(ctx context.Context, taskID string) ([]Snapshot, error) {
	errands, err := s.store.ListErrandsByTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list errands: %w", err)
	}
	sessions := make(map[string]*models.TaskSession, len(errands))
	var working []string
	for _, errand := range errands {
		if errand.Lifecycle != models.ErrandLifecycleLaunched {
			continue
		}
		session, err := s.store.GetTaskSession(ctx, errand.ID)
		if err != nil && !isMissingSession(err) {
			return nil, fmt.Errorf("load errand session %s: %w", errand.ID, err)
		}
		if session == nil {
			continue
		}
		sessions[errand.ID] = session
		if isWorking(session) {
			working = append(working, session.ID)
		}
	}
	pending := map[string]models.TaskPendingAction{}
	if len(working) > 0 {
		pending, err = s.store.GetPendingActionsBySessionIDs(ctx, working)
		if err != nil {
			return nil, fmt.Errorf("load pending actions: %w", err)
		}
	}
	snapshots := make([]Snapshot, 0, len(errands))
	for _, errand := range errands {
		permission := pending[errand.ID] == models.TaskPendingActionPermission
		snapshots = append(snapshots, Snapshot{
			Errand: errand, Status: ProjectStatus(errand, sessions[errand.ID], permission),
		})
	}
	return snapshots, nil
}
