package db

import (
	"context"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
)

const ErrandTurnSettlementEffect = "errand.turn_settlement"

var ErrErrandSettlementPending = errors.New("preceding errand turn has no committed exact settlement decision")

// RequireErrandTurnSettlements runs under LockErrandDelivery in the same
// transaction as the successor reservation. Empty or ambiguous receipts never
// authorize admission. The prospective turn can already have been reserved.
func RequireErrandTurnSettlements(ctx context.Context, tx *sqlx.Tx, sessionID, prospectiveTurnID string) error {
	lifecycle := dialect.JSONExtract(tx.DriverName(), "t.metadata", "lifecycle_only")
	var pending bool
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (
		SELECT 1 FROM task_sessions s JOIN task_session_turns t ON t.task_session_id = s.id
		WHERE s.id = ? AND s.role = 'errand' AND t.completed_at IS NOT NULL AND t.id <> ?
		AND CAST(COALESCE(`+lifecycle+`, '') AS TEXT) NOT IN ('true', '1')
		AND ((SELECT COUNT(*) FROM agent_delivery_effects e WHERE e.effect_type = ?
			AND e.session_id = s.id AND e.incarnation_id = s.queue_incarnation_id AND e.turn_id = t.id) <> 1
		OR (SELECT COUNT(*) FROM agent_delivery_effects e WHERE e.effect_type = ?
			AND e.session_id = s.id AND e.incarnation_id = s.queue_incarnation_id AND e.turn_id = t.id
			AND e.submission_id <> '' AND e.state = 'completed' AND e.completed_at IS NOT NULL
			AND e.outcome IN ('parent:done', 'parent:failed', 'parent:interrupted', 'user:done', 'user:failed', 'user:interrupted', 'parent:cancelled', 'user:cancelled')) <> 1)
	)`), sessionID, prospectiveTurnID, ErrandTurnSettlementEffect, ErrandTurnSettlementEffect).Scan(&pending)
	if err != nil {
		return err
	}
	if pending {
		return ErrErrandSettlementPending
	}
	return nil
}
