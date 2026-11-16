package messagequeue

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
)

func (r *sqliteRepository) guardErrandSettlementTx(ctx context.Context, tx *sqlx.Tx, sessionID string) error {
	if !r.errandsSchemaPresent {
		return nil
	}
	var taskID, role string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT task_id, role FROM task_sessions WHERE id = ?`), sessionID).Scan(&taskID, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if role != "errand" {
		return nil
	}
	if err := db.LockErrandDelivery(ctx, tx, tx.DriverName(), taskID); err != nil {
		return err
	}
	return db.RequireErrandTurnSettlements(ctx, tx, sessionID, "")
}
