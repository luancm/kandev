package sqlite

import (
	"context"
	"database/sql"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// LookupErrandPermissionSource resolves unique retained attribution, not liveness.
func (r *Repository) LookupErrandPermissionSource(ctx context.Context, owner models.ErrandPermissionOwner, requestID, pendingID string) (*models.ErrandPermissionSource, error) {
	if !validErrandPermissionOwner(owner) || owner.TaskID == "" || requestID == "" || pendingID == "" {
		return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	tx, err := r.ro.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var taskID string
	if err = tx.GetContext(ctx, &taskID, tx.Rebind(`SELECT task_id FROM task_session_errands WHERE id=?`), owner.ErrandID); err != nil {
		return nil, err
	}
	if taskID != owner.TaskID {
		return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	if err = checkCurrentErrandPermissionOwnerTx(ctx, tx, owner); err != nil {
		return nil, err
	}
	_, source, err := r.loadErrandPermissionSourceTx(ctx, tx, owner, requestID, pendingID, nil, false)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &source, nil
}
