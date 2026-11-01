package sqlite

import (
	"context"
	"encoding/json"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// ListUnresolvedErrandAwaits captures retained invocation identities at a
// recovery boundary. Empty parent selects the startup inventory.
func (r *Repository) ListUnresolvedErrandAwaits(ctx context.Context, parentID string) ([]models.ErrandAwaitIdentity, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`SELECT token,task_id,parent_session_id,
 parent_incarnation_id,harness_generation,turn_id,submission_id FROM errand_await_attempt
 WHERE state IN ('open','claimed','offered') AND (?='' OR parent_session_id=?) ORDER BY task_id,token`), parentID, parentID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var result []models.ErrandAwaitIdentity
	for rows.Next() {
		var id models.ErrandAwaitIdentity
		if err := rows.Scan(&id.Token, &id.TaskID, &id.ParentSessionID, &id.ParentIncarnationID, &id.HarnessGeneration, &id.TurnID, &id.SubmissionID); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

// RecoverErrandAwait classifies an invocation whose owner was lost. Receipt
// acceptance already commits its positive decision with the inbox, so a
// concurrently committed receipt wins in either task-guard ordering.
func (r *Repository) RecoverErrandAwait(ctx context.Context, id models.ErrandAwaitIdentity) (bool, error) {
	if id.TaskID == "" || id.ParentSessionID == "" || id.Token == "" {
		return false, models.ErrInvalidErrand
	}
	changed := false
	err := r.withErrandTask(ctx, id.TaskID, func(tx *sqlx.Tx) error {
		if _, err := checkErrandAwaitOwner(ctx, tx, id); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_await_attempt SET state='uncertain',updated_at=?
 WHERE token=? AND state IN ('open','claimed','offered')`), r.nowUTC(), id.Token)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		changed = count > 0
		return err
	})
	return changed && err == nil, err
}

// ClassifyErrandAwaitTerminal runs after ordered inbox projection. The
// accepted event and captured origin, never the latest turn, supply authority.
// This records unconfirmed reception, not non-reception or an abort.
func (r *Repository) ClassifyErrandAwaitTerminal(ctx context.Context, event *models.AgentDeliveryEvent) error {
	if event == nil || !event.Terminal || !hasValidAgentDeliveryEventOwnerIdentity(event) || deliveryTerminalOutcome(event.EventType) == "" {
		return nil
	}
	return r.withErrandTask(ctx, event.TaskID, func(tx *sqlx.Tx) error {
		stored, projectedAt, err := r.loadAgentDeliveryEventTx(ctx, tx, event.StreamID, event.Sequence)
		if err != nil {
			return err
		}
		matches, err := sameAgentDeliveryEvent(&stored, event)
		if err != nil {
			return err
		}
		if !matches {
			return repoerrors.ErrAgentDeliveryEventConflict
		}
		projected, exists, err := r.projectedDeliverySequenceTx(ctx, tx, event.StreamID)
		if err != nil || !exists || projected < event.Sequence || !projectedAt.Valid {
			return err
		}
		var taskID string
		if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT task_id FROM task_sessions WHERE id=?`), event.SessionID).Scan(&taskID); err != nil {
			return err
		}
		if taskID != event.TaskID {
			return models.ErrErrandVersionConflict
		}
		return r.classifyErrandAwaitOriginTx(ctx, tx, event)
	})
}

func (r *Repository) classifyErrandAwaitOriginTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent) error {
	var payload streams.AgentEvent
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_await_attempt SET state='uncertain',updated_at=?
 WHERE task_id=? AND parent_session_id=? AND parent_incarnation_id=? AND harness_generation=?
 AND state IN ('open','claimed','offered') AND (turn_id<>'' OR submission_id<>'')
 AND (turn_id='' OR turn_id=?) AND (submission_id='' OR submission_id=?)`),
		r.nowUTC(), event.TaskID, event.SessionID, event.IncarnationID, event.HarnessGeneration, payload.TurnID, event.SubmissionID)
	return err
}
