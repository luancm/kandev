package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// CreateErrandMessage retains the complete tool payload in Errand-owned storage.
func (r *Repository) CreateErrandMessage(ctx context.Context, errandID string, message *models.Message) error {
	if message == nil || message.ID == "" || message.TurnID == "" {
		return fmt.Errorf("errand message and turn identity are required")
	}
	taskID, err := r.errandTranscriptOwner(ctx, errandID, message.TaskID, message.TaskSessionID)
	if err != nil {
		return err
	}
	row := *message
	row.TaskID = taskID
	if row.PayloadDigest != "" {
		return fmt.Errorf("errand message requires inline tool payload")
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = r.nowUTC()
	}
	row.CreatedAt = errandTranscriptTime(row.CreatedAt)
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = row.CreatedAt
	}
	row.UpdatedAt = errandTranscriptTime(row.UpdatedAt)
	if row.Type == "" {
		row.Type = models.MessageTypeMessage
	}
	encoded, err := json.Marshal(&row)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO errand_messages(id,errand_id,turn_id,payload,created_at,updated_at) VALUES(?,?,?,?,?,?)`), row.ID, errandID, row.TurnID, string(encoded), row.CreatedAt, row.UpdatedAt)
	if err == nil {
		*message = row
	}
	return err
}

const errandMessageColumns = `m.id, e.task_id, m.turn_id, m.payload, m.created_at, m.updated_at`

func scanErrandMessage(scanner interface{ Scan(...any) error }) (*models.Message, error) {
	var id, taskID, turnID, payload string
	var createdAt, updatedAt time.Time
	if err := scanner.Scan(&id, &taskID, &turnID, &payload, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var message models.Message
	if err := json.Unmarshal([]byte(payload), &message); err != nil {
		return nil, err
	}
	message.ID, message.TaskID, message.TaskSessionID, message.TurnID = id, taskID, "", turnID
	message.CreatedAt, message.UpdatedAt = createdAt, updatedAt
	return &message, nil
}

func (r *Repository) GetErrandMessage(ctx context.Context, errandID, messageID string) (*models.Message, error) {
	return scanErrandMessage(r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT `+errandMessageColumns+`
 FROM errand_messages m JOIN task_session_errands e ON e.id=m.errand_id WHERE m.errand_id=? AND m.id=?`), errandID, messageID))
}

func (r *Repository) UpdateErrandMessage(ctx context.Context, errandID string, message *models.Message) error {
	if message == nil {
		return fmt.Errorf("errand message is required")
	}
	if _, err := r.errandTranscriptOwner(ctx, errandID, message.TaskID, message.TaskSessionID); err != nil {
		return err
	}
	if message.PayloadDigest != "" {
		return fmt.Errorf("errand message requires inline tool payload")
	}
	current, err := r.GetErrandMessage(ctx, errandID, message.ID)
	if err != nil {
		return err
	}
	if current.TurnID != message.TurnID {
		return fmt.Errorf("errand message turn ownership is immutable")
	}
	row := *message
	row.TaskID = current.TaskID
	row.CreatedAt = current.CreatedAt
	row.UpdatedAt = errandTranscriptTime(r.nowUTC())
	encoded, err := json.Marshal(&row)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE errand_messages SET payload=?,updated_at=? WHERE errand_id=? AND id=? AND turn_id=?`), string(encoded), row.UpdatedAt, errandID, row.ID, row.TurnID)
	if err != nil {
		return err
	}
	if err = errandTranscriptUpdated(result); err == nil {
		*message = row
	}
	return err
}

// ListErrandMessages returns the latest page in chronological order. A scoped
// cursor selects messages strictly older than itself, including equal timestamps.
func (r *Repository) ListErrandMessages(ctx context.Context, errandID, beforeMessageID string, limit int) ([]*models.Message, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	query := `SELECT ` + errandMessageColumns + ` FROM errand_messages m JOIN task_session_errands e ON e.id=m.errand_id WHERE m.errand_id=?`
	args := []any{errandID}
	if beforeMessageID != "" {
		cursor, err := r.GetErrandMessage(ctx, errandID, beforeMessageID)
		if err != nil {
			return nil, false, err
		}
		query += ` AND (m.created_at < ? OR (m.created_at = ? AND m.id < ?))`
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}
	query += ` ORDER BY m.created_at DESC,m.id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	messages := make([]*models.Message, 0)
	for rows.Next() {
		message, scanErr := scanErrandMessage(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(messages) > limit
	if more {
		messages = messages[:limit]
	}
	slices.Reverse(messages)
	return messages, more, nil
}
