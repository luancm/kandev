package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

// SealErrandTurnSettlement seals only an existing exact intent. Missing legacy
// intents must use reservation, including its strict admission fence.
func (r *Repository) SealErrandTurnSettlement(ctx context.Context, taskID string, effect *models.AgentDeliveryEffect) error {
	if !validErrandTerminalIntent(effect) || !sealedErrandOutcome(effect.Outcome) {
		return models.ErrInvalidErrand
	}
	return r.withErrandTask(ctx, taskID, func(tx *sqlx.Tx) error {
		if err := r.validateErrandSettlementOwner(ctx, tx, taskID, effect); err != nil {
			return err
		}
		if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), effect.SessionID); err != nil {
			return err
		}
		stored, err := errandSettlementEffectTx(ctx, tx, effect.SessionID, effect.IncarnationID, effect.TurnID)
		if err != nil {
			return err
		}
		if stored == nil {
			return sql.ErrNoRows
		}
		if !matchingErrandTerminalReceipt(stored, effect) {
			return models.ErrErrandVersionConflict
		}
		if stored.State == models.DeliveryEffectCompleted || stored.Outcome == effect.Outcome {
			return nil
		}
		if strings.Contains(stored.Outcome, ":") {
			return models.ErrErrandVersionConflict
		}
		_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE agent_delivery_effects SET outcome = ? WHERE effect_key = ? AND state = 'pending'`), effect.Outcome, effect.EffectKey)
		return err
	})
}

// CommitHistoricalErrandTurnReceipt completes a sealed exact decision without
// changing the Errand row or publishing a result for a historical turn.
func (r *Repository) CommitHistoricalErrandTurnReceipt(ctx context.Context, taskID string, effect *models.AgentDeliveryEffect) error {
	if !validErrandTerminalIntent(effect) || !sealedErrandOutcome(effect.Outcome) || effect.State != models.DeliveryEffectCompleted || effect.CompletedAt == nil {
		return models.ErrInvalidErrand
	}
	return r.withErrandTask(ctx, taskID, func(tx *sqlx.Tx) error {
		if err := r.validateErrandSettlementOwner(ctx, tx, taskID, effect); err != nil {
			return err
		}
		if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), effect.SessionID); err != nil {
			return err
		}
		stored, err := errandSettlementEffectTx(ctx, tx, effect.SessionID, effect.IncarnationID, effect.TurnID)
		if err != nil {
			return err
		}
		if !matchingErrandTerminalReceipt(stored, effect) || stored.Outcome != effect.Outcome {
			return db.ErrErrandSettlementPending
		}
		var completed sql.NullTime
		if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT completed_at FROM task_session_turns WHERE id = ? AND task_session_id = ? AND task_id = ?`), effect.TurnID, effect.SessionID, taskID).Scan(&completed); err != nil {
			return err
		}
		if !completed.Valid {
			return db.ErrErrandSettlementPending
		}
		if stored.State == models.DeliveryEffectCompleted {
			return supersedeInterruptedAssessmentTx(ctx, tx, taskID, effect)
		}
		if stored.State != models.DeliveryEffectPending {
			return db.ErrErrandSettlementPending
		}
		_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE agent_delivery_effects SET state = 'completed', completed_at = ? WHERE effect_key = ? AND state = 'pending'`), *effect.CompletedAt, effect.EffectKey)
		if err != nil {
			return err
		}
		return supersedeInterruptedAssessmentTx(ctx, tx, taskID, effect)
	})
}

func sealedErrandOutcome(outcome string) bool {
	origin, cause, sealed := strings.Cut(outcome, ":")
	if !sealed || (origin != errandTerminalParentOrigin && origin != authorKindUser) {
		return false
	}
	switch cause {
	case string(models.ErrandOutcomeDone), string(models.ErrandOutcomeFailed), "interrupted", string(models.DeliverySubmissionCancelled):
		return true
	default:
		return false
	}
}
