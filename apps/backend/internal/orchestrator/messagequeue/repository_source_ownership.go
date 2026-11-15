package messagequeue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
)

// LiveServerSourcesInTransaction reads only current queue authority. Admission
// receipts are deliberately absent: they survive removal of the accepted source.
// The caller holds the owning task delivery guard throughout its release decision.
func LiveServerSourcesInTransaction(ctx context.Context, tx *sqlx.Tx, taskID, sessionID string) ([]QueuedMessage, error) {
	present, err := internaldb.TableExistsContext(ctx, tx, "queued_messages")
	if err != nil || !present {
		return nil, err
	}
	// Refusal moves authority atomically between claim and source. Keep that
	// session locked across all reads, including on READ COMMITTED PostgreSQL.
	if err := lockSessionTxIn(ctx, tx, tx, sessionID); err != nil {
		return nil, err
	}
	sources, err := pendingServerSourcesTx(ctx, tx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	dispatches, err := fifoServerSourcesTx(ctx, tx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	claimed, err := sendNowServerSourcesTx(ctx, tx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	sources = append(sources, dispatches...)
	return append(sources, claimed...), nil
}

func pendingServerSourcesTx(ctx context.Context, tx *sqlx.Tx, taskID, sessionID string) ([]QueuedMessage, error) {
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`SELECT id, session_id, task_id, position, content, model, plan_mode,
        attachments_json, metadata_json, queued_at, queued_by FROM queued_messages
        WHERE task_id = ? AND session_id = ? AND queued_by = ? ORDER BY position`), taskID, sessionID, QueuedByServer)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var sources []QueuedMessage
	for rows.Next() {
		source, err := scanQueuedRow(rows)
		if err != nil {
			return nil, err
		}
		sources = append(sources, *source)
	}
	return sources, rows.Err()
}

func fifoServerSourcesTx(ctx context.Context, tx *sqlx.Tx, taskID, sessionID string) ([]QueuedMessage, error) {
	present, err := internaldb.TableExistsContext(ctx, tx, "queue_dispatch_claims")
	if err != nil || !present {
		return nil, err
	}
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`SELECT entry_id, attempt_id, message_json FROM queue_dispatch_claims WHERE session_id = ?`), sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var sources []QueuedMessage
	for rows.Next() {
		var id, attempt, encoded string
		if err := rows.Scan(&id, &attempt, &encoded); err != nil {
			return nil, err
		}
		var source QueuedMessage
		if err := json.Unmarshal([]byte(encoded), &source); err != nil {
			return nil, err
		}
		if id == source.ID && attempt != "" && source.TaskID == taskID && source.SessionID == sessionID && source.QueuedBy == QueuedByServer {
			sources = append(sources, source)
		}
	}
	return sources, rows.Err()
}

func sendNowServerSourcesTx(ctx context.Context, tx *sqlx.Tx, taskID, sessionID string) ([]QueuedMessage, error) {
	present, err := internaldb.TableExistsContext(ctx, tx, "queue_send_now_claims")
	if err != nil || !present {
		return nil, err
	}
	var id, encoded string
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT claim_id, claim_json FROM queue_send_now_claims WHERE session_id = ?`), sessionID).Scan(&id, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var claim SendNowClaim
	if err := unmarshalPersistedSendNowClaim([]byte(encoded), &claim); err != nil {
		return nil, err
	}
	taskGeneration, sessionGeneration, err := SourceGenerationsInTransaction(ctx, tx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	if invalidSendNowSourceClaim(&claim, id, taskID, sessionID, taskGeneration, sessionGeneration) {
		return nil, nil
	}
	var incarnation string
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT queue_incarnation_id FROM task_sessions WHERE id = ? AND task_id = ?`), sessionID, taskID).Scan(&incarnation)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if claim.Identity.SessionIncarnationID == "" || claim.Identity.SessionIncarnationID != incarnation {
		return nil, nil
	}
	return serverSendNowSources(&claim, taskID, sessionID)
}

func invalidSendNowSourceClaim(claim *SendNowClaim, id, taskID, sessionID string, taskGeneration, sessionGeneration int64) bool {
	generation, captured := claim.SourceGenerations[taskID]
	return !captured || claim.OperationGeneration <= 0 || id == "" || claim.ClaimID != id || claim.Identity.TaskID != taskID || claim.Identity.SessionID != sessionID ||
		claim.Dispatch.TaskID != taskID || claim.Dispatch.SessionID != sessionID || claim.OperationGeneration != sessionGeneration ||
		claim.SessionGeneration != sessionGeneration || generation != taskGeneration
}

func serverSendNowSources(claim *SendNowClaim, taskID, sessionID string) ([]QueuedMessage, error) {
	var sources []QueuedMessage
	for _, source := range claim.Sources {
		if source.TaskID != taskID || source.SessionID != sessionID {
			return nil, ErrSendNowClaimChanged
		}
		if source.QueuedBy == QueuedByServer {
			sources = append(sources, source)
		}
	}
	return sources, nil
}
