package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

// CreateErrandMessage stores large shell output separately from its summary.
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
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = insertErrandMessageTx(ctx, tx, errandID, &row); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	*message = row
	return nil
}

func insertErrandMessageTx(ctx context.Context, tx *sqlx.Tx, errandID string, row *models.Message) error {
	if err := externalizeErrandMessageTx(ctx, tx, row); err != nil {
		return err
	}
	encoded, err := json.Marshal(row)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO errand_messages(id,errand_id,turn_id,payload,payload_digest,payload_size,unread_candidate,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`), row.ID, errandID, row.TurnID, string(encoded), row.PayloadDigest, row.PayloadSize, errandMessageUnreadCandidate(row), row.CreatedAt, row.UpdatedAt)
	return err
}

func errandMessageUnreadCandidate(message *models.Message) bool {
	return statussummary.IsUnreadQualifyingMessage(string(message.AuthorType), string(message.Type), message.Content, message.Metadata)
}

const errandMessageColumns = `m.id, e.task_id, m.turn_id, m.payload, m.payload_digest, m.payload_size, m.created_at, m.updated_at`

func scanErrandMessage(scanner interface{ Scan(...any) error }) (*models.Message, error) {
	var id, taskID, turnID, payload string
	var digest string
	var size int64
	var createdAt, updatedAt time.Time
	if err := scanner.Scan(&id, &taskID, &turnID, &payload, &digest, &size, &createdAt, &updatedAt); err != nil {
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
	message.PayloadDigest, message.PayloadSize = digest, size
	return &message, nil
}

func (r *Repository) GetErrandMessage(ctx context.Context, errandID, messageID string) (*models.Message, error) {
	return scanErrandMessage(r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT `+errandMessageColumns+`
 FROM errand_messages m JOIN task_session_errands e ON e.id=m.errand_id WHERE m.errand_id=? AND m.id=?`), errandID, messageID))
}

func (r *Repository) UpdateErrandMessage(ctx context.Context, errandID string, message *models.Message) error {
	if message == nil || message.TaskSessionID != "" {
		return fmt.Errorf("invalid errand message")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	row, err := r.updateErrandMessageTx(ctx, tx, errandID, message)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	*message = row
	return nil
}

func (r *Repository) updateErrandMessageTx(ctx context.Context, tx *sqlx.Tx, errandID string, message *models.Message) (models.Message, error) {
	query := `SELECT ` + errandMessageColumns + ` FROM errand_messages m JOIN task_session_errands e ON e.id=m.errand_id WHERE m.errand_id=? AND m.id=?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE OF m`
	}
	current, err := scanErrandMessage(tx.QueryRowContext(ctx, tx.Rebind(query), errandID, message.ID))
	if err != nil {
		return models.Message{}, err
	}
	if current.TurnID != message.TurnID || (message.TaskID != "" && message.TaskID != current.TaskID) {
		return models.Message{}, fmt.Errorf("errand message ownership mismatch")
	}
	if message.PayloadDigest != "" && (message.PayloadDigest != current.PayloadDigest || message.PayloadSize != current.PayloadSize) {
		return models.Message{}, fmt.Errorf("errand message payload ownership mismatch")
	}
	row, err := prepareErrandMessageReplacement(current, message, r.nowUTC())
	if err != nil {
		return models.Message{}, err
	}
	if err = externalizeErrandMessageTx(ctx, tx, &row); err != nil {
		return models.Message{}, err
	}
	if err = reconcileErrandShellReplacementTx(ctx, tx, current, &row); err != nil {
		return models.Message{}, err
	}

	encoded, err := json.Marshal(&row)
	if err != nil {
		return models.Message{}, err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_messages SET payload=?,payload_digest=?,payload_size=?,unread_candidate=?,updated_at=? WHERE errand_id=? AND id=? AND turn_id=?`), string(encoded), row.PayloadDigest, row.PayloadSize, errandMessageUnreadCandidate(&row), row.UpdatedAt, errandID, row.ID, row.TurnID)
	if err != nil {
		return models.Message{}, err
	}
	if err = errandTranscriptUpdated(result); err != nil {
		return models.Message{}, err
	}
	return row, nil
}

func reconcileErrandShellReplacementTx(ctx context.Context, tx *sqlx.Tx, current, row *models.Message) error {
	stored, err := json.Marshal(current.Metadata)
	if err != nil {
		return err
	}
	incoming, err := json.Marshal(row.Metadata)
	if err != nil {
		return err
	}
	_, replacement, err := reconcileOwnedShellOutputStorageTx(ctx, tx, errandShellOutputQueries, row.ID, row.PayloadDigest, stored, incoming)
	if err != nil {
		return err
	}
	if replacement != nil {
		row.Metadata = replacement
	}
	if models.ToolPayloadRemoved(row.Metadata) {
		row.PayloadDigest, row.PayloadSize = "", 0
	}
	return nil
}

func externalizeErrandMessageTx(ctx context.Context, tx *sqlx.Tx, message *models.Message) error {
	if message.Metadata == nil {
		message.PayloadDigest, message.PayloadSize = "", 0
		return nil
	}
	metadata, err := externalizeMessagePayloadWith(ctx, message, func(ctx context.Context, digest string, payload []byte) error {
		compressed, err := gzipCompress(payload)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO task_message_payloads (digest,compressed_content,uncompressed_size,compressed_size,created_at) VALUES (?,?,?,?,?) ON CONFLICT (digest) DO NOTHING`), digest, compressed, len(payload), len(compressed), time.Now().UTC())
		return err
	})
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewBufferString(metadata))
	decoder.UseNumber()
	return decoder.Decode(&message.Metadata)
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
	orderedID := dialect.ByteOrderedText(r.ro.DriverName(), "m.id")
	args := []any{errandID}
	if beforeMessageID != "" {
		cursor, err := r.GetErrandMessage(ctx, errandID, beforeMessageID)
		if err != nil {
			return nil, false, err
		}
		query += ` AND (m.created_at < ? OR (m.created_at = ? AND ` + orderedID + ` < ?))`
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}
	query += ` ORDER BY m.created_at DESC,` + orderedID + ` DESC LIMIT ?`
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
