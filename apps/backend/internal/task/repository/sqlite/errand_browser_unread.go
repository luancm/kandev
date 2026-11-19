package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// AdvanceErrandReadCursor acknowledges only the observed message in the owned transcript.
func (r *Repository) AdvanceErrandReadCursor(ctx context.Context, taskID, errandID, messageID string) (bool, error) {
	joinedMessage := `FROM errand_messages m
		LEFT JOIN errand_messages cursor_message ON cursor_message.id=e.last_read_message_id AND cursor_message.errand_id=e.id
		WHERE m.errand_id=e.id AND m.id=?`
	query := `UPDATE task_session_errands AS e SET last_read_message_id=?
		WHERE e.task_id=? AND e.id=? AND EXISTS (SELECT 1 ` + joinedMessage + `
		AND (cursor_message.id IS NULL OR ` + errandMessageAfter(r.db.DriverName()) + `))`
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), messageID, taskID, errandID, messageID)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 1 {
		return changed == 1, err
	}
	var exists int
	query = `SELECT 1 FROM task_session_errands e JOIN errand_messages m ON m.errand_id=e.id
		WHERE e.task_id=? AND e.id=? AND m.id=?`
	err = r.db.GetContext(ctx, &exists, r.db.Rebind(query), taskID, errandID, messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, models.ErrErrandNotFound
	}
	return false, err
}

func errandBrowserUnreadPredicate(driver string) string {
	return `COALESCE((
		SELECT cursor_message.id IS NULL OR ` + errandMessageAfter(driver) + `
		FROM errand_messages m
		LEFT JOIN errand_messages cursor_message ON cursor_message.id=e.last_read_message_id AND cursor_message.errand_id=e.id
		WHERE m.errand_id=e.id AND m.unread_candidate
		ORDER BY m.created_at DESC, ` + dialect.ByteOrderedText(driver, "m.id") + ` DESC LIMIT 1
	), FALSE)`
}

func errandMessageAfter(driver string) string {
	id := dialect.ByteOrderedText(driver, "m.id")
	cursorID := dialect.ByteOrderedText(driver, "cursor_message.id")
	return `(m.created_at, ` + id + `) > (cursor_message.created_at, ` + cursorID + `)`
}
