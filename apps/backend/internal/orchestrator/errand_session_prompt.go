package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// errandRowReader reads one errand row by ID. An errand's ID is its session
// ID, so a prompt recovers the trusted errand block from the durable row rather
// than from caller-supplied text. The task repository implements it.
type errandRowReader interface {
	GetErrand(ctx context.Context, id string) (*models.Errand, error)
}

// errandLaunchForSession rebuilds the ErrandLaunch of an errand session from
// its durable row. Other sessions return nil without touching the reader, so
// the feature-off path never reads the errand table.
func (s *Service) errandLaunchForSession(ctx context.Context, session *models.TaskSession) (*ErrandLaunch, error) {
	if !models.IsErrandSession(session) {
		return nil, nil
	}
	reader, ok := s.repo.(errandRowReader)
	if !ok {
		return nil, errors.New("errand session prompt requires an errand row reader")
	}
	row, err := reader.GetErrand(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("load errand row for session prompt: %w", err)
	}
	if row == nil || row.ID != session.ID || row.TaskID != session.TaskID ||
		row.ParentSessionID == "" || row.ParentSessionID != session.ParentSessionID {
		return nil, errors.New("errand row does not match its session")
	}
	return &ErrandLaunch{ErrandID: row.ID, ParentSessionID: row.ParentSessionID, Title: row.Title}, nil
}
