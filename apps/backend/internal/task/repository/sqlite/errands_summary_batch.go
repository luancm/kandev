package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

// BatchErrandsSummariesByTaskIDs returns a bounded errand summary for each
// requested task ID.
func (r *Repository) BatchErrandsSummariesByTaskIDs(ctx context.Context, taskIDs []string) (map[string]statussummary.ErrandsSummary, error) {
	result := make(map[string]statussummary.ErrandsSummary, len(taskIDs))
	uniqueTaskIDs := make([]string, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		if taskID == "" {
			continue
		}
		if _, exists := result[taskID]; exists {
			continue
		}
		result[taskID] = statussummary.ErrandsSummary{}
		uniqueTaskIDs = append(uniqueTaskIDs, taskID)
	}
	if len(uniqueTaskIDs) == 0 {
		return result, nil
	}

	queryTemplate := `SELECT e.task_id,
		COALESCE(SUM(CASE WHEN ` + liveErrandPredicate(r.ro.DriverName()) + ` THEN 1 ELSE 0 END), 0),
		COALESCE(MAX(CASE WHEN ps.state = ? AND ` + outstandingErrandPredicate(r.ro.DriverName()) + ` THEN 1 ELSE 0 END), 0)
		FROM task_session_errands e
		LEFT JOIN task_sessions s ON s.id = e.id
		LEFT JOIN task_sessions ps ON ps.id = e.parent_session_id
		WHERE e.task_id IN (%s)
		GROUP BY e.task_id`
	for _, chunk := range chunkIDs(uniqueTaskIDs, sqliteMaxHostParams-1) {
		placeholders, taskIDArgs := buildInPlaceholders(chunk)
		query := fmt.Sprintf(queryTemplate, placeholders)
		args := make([]interface{}, 0, len(taskIDArgs)+1)
		args = append(args, string(models.TaskSessionStateWaitingForInput))
		args = append(args, taskIDArgs...)

		rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
		if err != nil {
			return nil, fmt.Errorf("summarize errands by task IDs: %w", err)
		}
		for rows.Next() {
			var taskID string
			var live, waiting int64
			if err := rows.Scan(&taskID, &live, &waiting); err != nil {
				return nil, fmt.Errorf("scan batched errand summary: %w", closeErrandSummaryRows(rows, err))
			}
			if _, requested := result[taskID]; requested {
				result[taskID] = statussummary.ErrandsSummary{Live: int(live), Waiting: waiting > 0}
			}
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate batched errand summaries: %w", closeErrandSummaryRows(rows, err))
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close batched errand summaries: %w", err)
		}
	}
	return result, nil
}

func closeErrandSummaryRows(rows *sql.Rows, cause error) error {
	if closeErr := rows.Close(); closeErr != nil {
		return errors.Join(cause, closeErr)
	}
	return cause
}
