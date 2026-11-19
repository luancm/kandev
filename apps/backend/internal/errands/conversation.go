package errands

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

// ReadErrandMessages reads one page from the Errand's conversation.
func (s *Service) ReadErrandMessages(ctx context.Context, taskID, errandID, before string, limit int) ([]*models.Message, bool, error) {
	row, err := s.Lookup(ctx, errandID)
	if err != nil {
		return nil, false, err
	}
	if row.TaskID != taskID {
		return nil, false, notFound()
	}
	reader, ok := s.store.(interface {
		ListErrandMessages(context.Context, string, string, int) ([]*models.Message, bool, error)
	})
	if !ok {
		return nil, false, errors.New("errand conversation storage is unavailable")
	}
	messages, more, err := reader.ListErrandMessages(ctx, row.ID, before, limit)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, notFound()
	}
	return messages, more, err
}

// MarkErrandRead acknowledges a message the browser has observed in the conversation.
func (s *Service) MarkErrandRead(ctx context.Context, taskID, errandID, messageID string) error {
	writer, ok := s.store.(interface {
		AdvanceErrandReadCursor(context.Context, string, string, string) (bool, error)
	})
	if !ok {
		return errors.New("errand conversation storage is unavailable")
	}
	changed, err := writer.AdvanceErrandReadCursor(ctx, taskID, errandID, messageID)
	if err != nil {
		return err
	}
	if changed {
		s.publishOwnedSnapshot(ctx, taskID, errandID, false)
	}
	return nil
}
