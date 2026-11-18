package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) ReadErrandStatuses(ctx context.Context, taskID, errandID string) (map[string]models.ErrandBrowserObservation, error) {
	driver := r.ro.DriverName()
	query := `SELECT e.id,` + errandBrowserStatusExpression(driver) + `,` + errandBrowserAttentionExpression(driver) + `
		FROM task_session_errands e LEFT JOIN task_sessions s ON s.id=e.id
		WHERE e.task_id=? AND ` + validErrandBrowserIdentity()
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
		if err := rows.Scan(&item.ID, &item.Status, &item.AttentionReason); err != nil {
			return nil, err
		}
		item.AttentionReason = models.ProjectErrandAttentionReason(item.Status, item.AttentionReason)
		statuses[item.ID] = item
	}
	return statuses, rows.Err()
}
