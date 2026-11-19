package errands

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

func observeErrand(row *models.Errand, observation models.ErrandBrowserObservation) Snapshot {
	return Snapshot{
		Errand: row, Status: observation.Status, AttentionReason: observation.AttentionReason,
		ContextMetadata: observation.ContextMetadata,
	}
}

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
	rows, err := s.store.ListErrandsByTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list errands: %w", err)
	}
	statuses, err := s.store.ReadErrandStatuses(ctx, taskID, "")
	if err != nil {
		return nil, err
	}
	snapshots := make([]Snapshot, 0, len(rows))
	for _, row := range rows {
		if observation, ok := statuses[row.ID]; ok {
			snapshots = append(snapshots, observeErrand(row, observation))
		}
	}
	return snapshots, nil
}
