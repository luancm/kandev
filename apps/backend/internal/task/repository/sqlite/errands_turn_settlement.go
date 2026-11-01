package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const errandTerminalParentOrigin = "parent"

// ReserveErrandTurnSettlement binds origin and exact identity before provider
// I/O. A terminal observation may seal that same pending intent for retry.
func (r *Repository) ReserveErrandTurnSettlement(ctx context.Context, taskID string, effect *models.AgentDeliveryEffect) error {
	if !validErrandTerminalIntent(effect) {
		return models.ErrInvalidErrand
	}

	return r.withErrandTask(ctx, taskID, func(tx *sqlx.Tx) error {
		if err := r.validateErrandSettlementOwner(ctx, tx, taskID, effect); err != nil {
			return err
		}
		if err := db.RequireErrandTurnSettlements(ctx, tx, effect.SessionID, effect.TurnID); err != nil {
			return err
		}
		stored, err := errandSettlementEffectTx(ctx, tx, effect.SessionID, effect.IncarnationID, effect.TurnID)
		if err != nil {
			return err
		}
		if stored == nil {
			_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO agent_delivery_effects
				(effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome, created_at)
				VALUES (?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?, ?, ?)`), effect.EffectKey, effect.StreamID, effect.Sequence,
				db.ErrandTurnSettlementEffect, effect.SessionID, effect.IncarnationID, effect.HarnessGeneration,
				effect.SubmissionID, effect.TurnID, effect.Outcome, r.nowUTC())
			return err
		}
		if !sameErrandSettlementIdentity(stored, effect) || settlementOrigin(stored.Outcome) != settlementOrigin(effect.Outcome) {
			return models.ErrErrandVersionConflict
		}
		if stored.State == models.DeliveryEffectCompleted || stored.Outcome == effect.Outcome || !strings.Contains(effect.Outcome, ":") {
			return nil
		}
		if strings.Contains(stored.Outcome, ":") {
			return models.ErrErrandVersionConflict
		}
		_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE agent_delivery_effects SET outcome = ? WHERE effect_key = ? AND state = 'pending'`), effect.Outcome, stored.EffectKey)
		return err
	})
}

func settlementOrigin(outcome string) string {
	origin, _, _ := strings.Cut(outcome, ":")
	return origin
}

func (r *Repository) guardErrandTurnReservation(ctx context.Context, tx *sqlx.Tx, sessionID, prospectiveTurnID string) error {
	if !dialect.IsPostgres(tx.DriverName()) {
		if err := db.LockErrandDelivery(ctx, tx, tx.DriverName(), ""); err != nil {
			return err
		}
	}
	var taskID, role string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT task_id, role FROM task_sessions WHERE id = ?`), sessionID).Scan(&taskID, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if role != string(models.SessionRoleErrand) {
		return nil
	}
	// Step-stamped turn creation later takes the task row for update. Take it
	// before the errand advisory guard, matching queue/task admission order.
	if dialect.IsPostgres(tx.DriverName()) {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`SELECT id FROM tasks WHERE id = ? FOR UPDATE`), taskID); err != nil {
			return err
		}
	}
	if err := lockErrandTask(ctx, tx, taskID); err != nil {
		return err
	}
	return db.RequireErrandTurnSettlements(ctx, tx, sessionID, prospectiveTurnID)
}

func sameErrandSettlementIdentity(a, b *models.AgentDeliveryEffect) bool {
	return a.EffectKey == b.EffectKey && a.SessionID == b.SessionID && a.IncarnationID == b.IncarnationID &&
		a.TurnID == b.TurnID && a.SubmissionID == b.SubmissionID && a.StreamID == b.StreamID &&
		a.HarnessGeneration == b.HarnessGeneration && a.Sequence == b.Sequence
}

func (r *Repository) validateErrandSettlementOwner(ctx context.Context, tx *sqlx.Tx, taskID string, e *models.AgentDeliveryEffect) error {
	var valid bool
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM task_sessions s
		JOIN task_session_turns t ON t.task_session_id = s.id WHERE s.id = ? AND s.task_id = ?
		AND s.queue_incarnation_id = ? AND s.role = 'errand' AND t.id = ? AND t.task_id = ?)`),
		e.SessionID, taskID, e.IncarnationID, e.TurnID, taskID).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return models.ErrInvalidErrand
	}
	return nil
}

// CommitErrandTurnSettlement writes the row and decision in one task-guarded
// transaction. Replay observes the receipt before checking the old row version.
func (r *Repository) CommitErrandTurnSettlement(ctx context.Context, row *models.Errand, effect *models.AgentDeliveryEffect) (bool, error) {
	if !validErrandTerminalCommit(row, effect) {
		return false, models.ErrInvalidErrand
	}

	var committed *models.Errand
	applied := false
	err := r.withErrandTask(ctx, row.TaskID, func(tx *sqlx.Tx) error {
		if err := r.validateErrandSettlementOwner(ctx, tx, row.TaskID, effect); err != nil {
			return err
		}
		stored, err := errandSettlementEffectTx(ctx, tx, effect.SessionID, effect.IncarnationID, effect.TurnID)
		if err != nil {
			return err
		}
		if !matchingErrandTerminalReceipt(stored, effect) {
			return db.ErrErrandSettlementPending
		}
		if stored.State == models.DeliveryEffectCompleted {
			committed, err = scanErrand(tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+errandColumns+` FROM task_session_errands WHERE id = ?`), row.ID))
			if err != nil {
				return err
			}
			return readWrittenErrandDeliveryState(ctx, tx, committed)
		}
		if stored.Outcome != effect.Outcome && stored.Outcome != settlementOrigin(effect.Outcome) {
			return db.ErrErrandSettlementPending
		}
		next, outcome, err := r.writeErrandTerminalRowTx(ctx, tx, row, effect)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE agent_delivery_effects SET state = 'completed', outcome = ?, completed_at = ? WHERE effect_key = ? AND state = 'pending'`), outcome, *effect.CompletedAt, effect.EffectKey); err != nil {
			return err
		}
		committed = next
		applied = true
		return nil
	})
	if err == nil && committed != nil {
		*row = *committed
	}
	return err == nil && applied, err
}

func (r *Repository) writeErrandTerminalRowTx(ctx context.Context, tx *sqlx.Tx, row *models.Errand, effect *models.AgentDeliveryEffect) (*models.Errand, string, error) {
	current, err := currentErrandForUpdate(ctx, tx, row)
	if err != nil {
		return nil, "", err
	}
	if err := r.guardObservedErrandTerminalTx(ctx, tx, effect); err != nil {
		return nil, "", err
	}
	if current.Lifecycle == models.ErrandLifecycleCancelled {
		return current, settlementOrigin(effect.Outcome) + ":cancelled", nil
	}
	next := *row
	preserveErrandIdentity(&next, current)
	next.ResultRevision, next.UpdatedAt = errandUpdateRevision(current, &next), r.nowUTC()
	if err := writeErrandUpdate(ctx, tx, &next); err != nil {
		return nil, "", err
	}
	if err := readWrittenErrandDeliveryState(ctx, tx, &next); err != nil {
		return nil, "", err
	}
	next.Version++
	return &next, effect.Outcome, nil
}

func (r *Repository) guardObservedErrandTerminalTx(ctx context.Context, tx *sqlx.Tx, effect *models.AgentDeliveryEffect) error {
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), effect.SessionID); err != nil {
		return err
	}
	predicate, order := currentTurnAuthority(r.db.DriverName(), "t")
	var turnID string
	var completed sql.NullTime
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT t.id, t.completed_at FROM task_session_turns t
  WHERE t.task_session_id = ? AND `+predicate+` ORDER BY `+order+` LIMIT 1`), effect.SessionID).Scan(&turnID, &completed)
	if err != nil {
		return err
	}
	if turnID != effect.TurnID || !completed.Valid {
		return db.ErrErrandSettlementPending
	}
	return nil
}

func errandSettlementEffectTx(ctx context.Context, tx *sqlx.Tx, sessionID, incarnationID, turnID string) (*models.AgentDeliveryEffect, error) {
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`SELECT effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id,
		harness_generation, submission_id, turn_id, outcome, created_at, completed_at FROM agent_delivery_effects
		WHERE effect_type = ? AND session_id = ? AND incarnation_id = ? AND turn_id = ?`), db.ErrandTurnSettlementEffect, sessionID, incarnationID, turnID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var found *models.AgentDeliveryEffect
	for rows.Next() {
		if found != nil {
			return nil, db.ErrErrandSettlementPending
		}
		var e models.AgentDeliveryEffect
		if err := rows.Scan(&e.EffectKey, &e.StreamID, &e.Sequence, &e.EffectType, &e.State, &e.SessionID,
			&e.IncarnationID, &e.HarnessGeneration, &e.SubmissionID, &e.TurnID, &e.Outcome, &e.CreatedAt, &e.CompletedAt); err != nil {
			return nil, err
		}
		found = &e
	}
	return found, rows.Err()
}

// PendingErrandTurnSettlements enumerates exact completed turns, never the
// mutable latest turn. The reserved first identity is also durable admission
// evidence for pre-existing legacy errands. Other missing evidence fails closed.
func (r *Repository) PendingErrandTurnSettlements(ctx context.Context, sessionID string) ([]*models.AgentDeliveryEffect, error) {
	session, err := r.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if !models.IsErrandSession(session) {
		return nil, nil
	}
	row, err := r.GetErrand(ctx, sessionID)
	if errors.Is(err, models.ErrErrandNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	turns, err := r.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	var pending []*models.AgentDeliveryEffect
	for _, turn := range turns {
		if turn.CompletedAt == nil || turn.Metadata[models.TurnMetaKeyLifecycleOnly] == true {
			continue
		}
		effect, err := r.readErrandSettlementEffect(ctx, sessionID, session.QueueIncarnationID, turn.ID)
		if err != nil {
			return nil, err
		}
		if effect == nil {
			if turn.ID != row.FirstTurnID {
				return nil, db.ErrErrandSettlementPending
			}
			effect = &models.AgentDeliveryEffect{EffectKey: fmt.Sprintf("errand.turn:%s:%s:%s:%s", row.TaskID, session.QueueIncarnationID, turn.ID, row.SubmissionID),
				EffectType: db.ErrandTurnSettlementEffect, State: models.DeliveryEffectPending, SessionID: sessionID,
				IncarnationID: session.QueueIncarnationID, TurnID: turn.ID, SubmissionID: row.SubmissionID, Outcome: errandTerminalParentOrigin}
		}
		if err := r.resolveErrandTerminalEvidence(ctx, effect); err != nil {
			return nil, err
		}
		if effect.State != models.DeliveryEffectCompleted {
			pending = append(pending, effect)
		}
		effect.TaskID = row.TaskID
	}
	return pending, nil
}

func (r *Repository) readErrandSettlementEffect(ctx context.Context, sessionID, incarnationID, turnID string) (*models.AgentDeliveryEffect, error) {
	tx, err := r.ro.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	return errandSettlementEffectTx(ctx, tx, sessionID, incarnationID, turnID)
}

func validErrandTerminalIntent(effect *models.AgentDeliveryEffect) bool {
	return effect != nil && effect.EffectKey != "" && effect.TurnID != "" && effect.SubmissionID != "" && effect.IncarnationID != "" &&
		(effect.Outcome == errandTerminalParentOrigin || effect.Outcome == authorKindUser || strings.Contains(effect.Outcome, ":"))
}

func validErrandTerminalCommit(row *models.Errand, effect *models.AgentDeliveryEffect) bool {
	return row != nil && effect != nil && row.ID == effect.SessionID && effect.State == models.DeliveryEffectCompleted && effect.CompletedAt != nil
}

func matchingErrandTerminalReceipt(stored, observed *models.AgentDeliveryEffect) bool {
	return stored != nil && sameErrandSettlementIdentity(stored, observed) && settlementOrigin(stored.Outcome) == settlementOrigin(observed.Outcome)
}
