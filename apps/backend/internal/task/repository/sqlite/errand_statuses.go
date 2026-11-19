package sqlite

import (
	"context"
	"database/sql"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) ReadErrandStatuses(ctx context.Context, taskID, errandID string) (map[string]models.ErrandBrowserObservation, error) {
	driver := r.ro.DriverName()
	contextColumn := "NULL"
	if errandID != "" {
		contextColumn = errandContextMetadataColumn
	}
	query := `SELECT e.id,` + errandBrowserStatusExpression(driver) + `,` + errandBrowserAttentionExpression(driver) + `,` + contextColumn + `
		FROM task_session_errands e
		WHERE e.task_id=?`
	args := []any{taskID}
	if errandID != "" {
		query += ` AND e.id=?`
		args = append(args, errandID)
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	statuses := make(map[string]models.ErrandBrowserObservation)
	for rows.Next() {
		var item models.ErrandBrowserObservation
		var contextMetadata sql.NullString
		if err := rows.Scan(&item.ID, &item.Status, &item.AttentionReason, &contextMetadata); err != nil {
			return nil, err
		}
		if errandID != "" {
			item.ContextMetadata, err = decodeErrandContextMetadata(contextMetadata.String)
			if err != nil {
				return nil, err
			}
			if item.ContextMetadata == nil {
				item.ContextMetadata = map[string]any{models.SessionMetaKeyContextWindow: nil}
			}
		}
		item.AttentionReason = models.ProjectErrandAttentionReason(item.Status, item.AttentionReason)
		statuses[item.ID] = item
	}
	return statuses, rows.Err()
}
