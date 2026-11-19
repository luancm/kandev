package sqlite

import (
	"context"
	"fmt"
)

// GetParentErrandState reads both booleans from one snapshot, without loading
// child session details or result payloads. Recovery remains observation only.
func (r *Repository) GetParentErrandState(ctx context.Context, parentID string) (hasErrands, outstanding bool, err error) {
	err = r.ro.QueryRowContext(ctx, r.ro.Rebind(parentErrandStateQuery(r.ro.DriverName())), parentID, parentID).Scan(&hasErrands, &outstanding)
	if err != nil {
		return false, false, fmt.Errorf("read parent errand state: %w", err)
	}
	return hasErrands, outstanding, nil
}

func parentErrandStateQuery(driver string) string {
	return `SELECT
 EXISTS (SELECT 1 FROM task_session_errands WHERE parent_session_id = ?),
 EXISTS (SELECT 1 FROM task_session_errands e
 WHERE e.parent_session_id = ? AND ` + outstandingErrandPredicate(driver) + `)`
}
