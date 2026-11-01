package db

import (
	"context"

	"github.com/kandev/kandev/internal/db/dialect"
)

// LockErrandDelivery serializes a task's inventory, claims and prepared queue
// admission. Existing hierarchy/task admission locks precede this guard;
// session/stream locks follow it. No external I/O may run under this lock.
func LockErrandDelivery(ctx context.Context, tx ContextExecer, driver, taskID string) error {
	if dialect.IsPostgres(driver) {
		// Task FK ownership precedes the advisory guard and session/stream locks.
		if _, err := tx.ExecContext(ctx, tx.Rebind(`SELECT id FROM tasks WHERE id = ? FOR KEY SHARE`), taskID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, tx.Rebind(`SELECT pg_advisory_xact_lock(hashtextextended('errands:' || ?, 0))`), taskID)
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE tasks SET id = id WHERE 0`)
	return err
}
