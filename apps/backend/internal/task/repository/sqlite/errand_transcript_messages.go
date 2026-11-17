package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
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
	decoder := json.NewDecoder(bytes.NewBufferString(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&message); err != nil {
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
	if message == nil || message.TaskSessionID != "" || message.PayloadDigest != "" {
		return fmt.Errorf("invalid errand message")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	query := `SELECT ` + errandMessageColumns + ` FROM errand_messages m JOIN task_session_errands e ON e.id=m.errand_id WHERE m.errand_id=? AND m.id=?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE OF m`
	}
	current, err := scanErrandMessage(tx.QueryRowContext(ctx, tx.Rebind(query), errandID, message.ID))
	if err != nil {
		return err
	}
	if current.TurnID != message.TurnID || (message.TaskID != "" && message.TaskID != current.TaskID) {
		return fmt.Errorf("errand message ownership mismatch")
	}
	row, err := prepareErrandMessageReplacement(current, message, r.nowUTC())
	if err != nil {
		return err
	}

	encoded, err := json.Marshal(&row)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_messages SET payload=?,updated_at=? WHERE errand_id=? AND id=? AND turn_id=?`), string(encoded), row.UpdatedAt, errandID, row.ID, row.TurnID)
	if err != nil {
		return err
	}
	if err = errandTranscriptUpdated(result); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	*message = row
	return nil
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

func prepareErrandMessageReplacement(current, message *models.Message, now time.Time) (models.Message, error) {
	oldMetadata, err := json.Marshal(current.Metadata)
	if err != nil {
		return models.Message{}, err
	}
	newMetadata, err := json.Marshal(message.Metadata)
	if err != nil {
		return models.Message{}, err
	}
	_, sanitized, err := mergeRetainedMessageMetadata(oldMetadata, newMetadata)
	if err != nil {
		return models.Message{}, err
	}
	row := *message
	row.TaskID = current.TaskID
	row.CreatedAt = current.CreatedAt
	row.UpdatedAt = errandTranscriptTime(now)
	if sanitized != nil {
		row.Metadata = sanitized
		row.Type = current.Type
		row.UpdatedAt = current.UpdatedAt
	}
	return row, nil
}
