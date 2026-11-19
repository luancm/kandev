package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

const errandContinuationMetadata = "errand_continuation"

type errandContinuationRecord struct {
	Generation   uint64 `json:"generation"`
	NativeID     string `json:"native_id"`
	SubmissionID string `json:"submission_id"`
}

// PrepareErrandContinuation reserves the new generation and its explicit
// instruction together. No work is created before the prior close is proven.
func (r *Repository) PrepareErrandContinuation(ctx context.Context, request models.ErrandContinuation) (*models.PreparedErrandContinuation, error) {
	if request.TaskID == "" || request.ErrandID == "" || request.TurnID == "" || request.SubmissionID == "" || request.Content == "" {
		return nil, models.ErrInvalidErrand
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = r.lockTaskRowInTx(ctx, tx, request.TaskID); err != nil {
		return nil, err
	}
	row, err := readErrandContinuationRow(ctx, tx, request)
	if err != nil {
		return nil, err
	}
	if prepared, err := readPreparedErrandContinuation(ctx, tx, row, request); !errors.Is(err, sql.ErrNoRows) {
		if err != nil || prepared.ExecutionID != "" {
			return prepared, err
		}
		if err := checkTaskCleanupBarrierTx(ctx, tx, request.TaskID); err != nil {
			return nil, err
		}
		return prepared, nil
	}
	prepared, err := r.reserveErrandContinuation(ctx, tx, row, request)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (r *Repository) reserveErrandContinuation(ctx context.Context, tx *sqlx.Tx, row *models.Errand, request models.ErrandContinuation) (*models.PreparedErrandContinuation, error) {
	if err := checkTaskCleanupBarrierTx(ctx, tx, request.TaskID); err != nil {
		return nil, err
	}
	prior, err := readErrandContinuationPredecessor(ctx, tx, row)
	if err != nil {
		return nil, err
	}
	if err = checkLiveErrandCapacity(ctx, tx, request.TaskID); err != nil {
		return nil, err
	}
	if err = r.insertErrandContinuation(ctx, tx, request, prior); err != nil {
		return nil, err
	}
	if err = r.reserveOwnedErrandParentInstruction(ctx, tx, row, request); err != nil {
		return nil, err
	}
	return &models.PreparedErrandContinuation{
		Generation: prior.Generation, NativeID: prior.NativeID, Predecessor: row.TeardownIntent.Target,
	}, nil
}

func (r *Repository) reserveOwnedErrandParentInstruction(ctx context.Context, tx *sqlx.Tx, row *models.Errand, request models.ErrandContinuation) error {
	if request.ParentSessionID == "" {
		return nil
	}
	if request.ParentSessionID != row.ParentSessionID || request.AuthorID != row.ParentSessionID {
		return models.ErrInvalidErrand
	}
	reservation, err := r.reserveErrandParentAdmissionTx(ctx, tx, row, request.ParentAdmission())
	if err != nil {
		return err
	}
	if !reservation.Created {
		return db.ErrErrandSettlementPending
	}
	return nil
}

func readErrandContinuationRow(ctx context.Context, tx *sqlx.Tx, request models.ErrandContinuation) (*models.Errand, error) {
	row, err := scanErrand(tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+errandColumns+` FROM task_session_errands WHERE task_id=? AND id=?`), request.TaskID, request.ErrandID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.ErrErrandNotFound
	}
	return row, err
}

// DiscardUnallocatedErrandContinuation releases a refused instruction after
// launch returns. Binding and release share the task lock.
func (r *Repository) DiscardUnallocatedErrandContinuation(ctx context.Context, request models.ErrandContinuation, generation uint64) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockTaskRowInTx(ctx, tx, request.TaskID); err != nil {
		return err
	}
	row, err := readErrandContinuationRow(ctx, tx, request)
	if err != nil {
		return err
	}
	if !errandHasConfirmedClose(row) || row.TeardownIntent.Target.OwnerGeneration+1 != generation {
		return nil
	}
	prepared, err := readPreparedErrandContinuation(ctx, tx, row, request)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if prepared.Generation != generation || prepared.ExecutionID != "" {
		return nil
	}
	if err := deleteUnboundErrandContinuation(ctx, tx, request, generation); err != nil {
		return err
	}
	if err := r.releaseUnallocatedParentInstruction(ctx, tx, request); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) releaseUnallocatedParentInstruction(ctx context.Context, tx *sqlx.Tx, request models.ErrandContinuation) error {
	if request.ParentSessionID == "" {
		return nil
	}
	state, err := readErrandAdmission(ctx, tx, request.ParentAdmission())
	if err != nil {
		return err
	}
	if state != string(models.ErrandAdmissionReserved) {
		return models.ErrErrandVersionConflict
	}
	if _, err = tx.ExecContext(ctx, tx.Rebind(`DELETE FROM errand_parent_admission WHERE errand_id=? AND submission_id=? AND state=?`),
		request.ErrandID, request.SubmissionID, models.ErrandAdmissionReserved); err != nil {
		return err
	}
	return retryErrandDeferredReleases(ctx, tx, request.TaskID, request.ParentSessionID, request.ErrandID, r.nowUTC())
}

func deleteUnboundErrandContinuation(ctx context.Context, tx *sqlx.Tx, request models.ErrandContinuation, generation uint64) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM errand_execution_generations
		WHERE errand_id=? AND generation=? AND execution_id='' AND active_submission_id=''
		 AND generation=(SELECT MAX(generation) FROM errand_execution_generations WHERE errand_id=?)`), request.ErrandID, int64(generation), request.ErrandID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return models.ErrExecutionRotated
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`DELETE FROM errand_turns WHERE errand_id=? AND id=?`), request.ErrandID, request.TurnID)
	return err
}

func readPreparedErrandContinuation(ctx context.Context, tx *sqlx.Tx, row *models.Errand, request models.ErrandContinuation) (*models.PreparedErrandContinuation, error) {
	turn, err := scanTurn(tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+errandTurnColumns+`
		FROM errand_turns t JOIN task_session_errands e ON e.id=t.errand_id WHERE t.errand_id=? AND t.id=?`), request.ErrandID, request.TurnID))
	if err != nil {
		return nil, err
	}
	receipt, err := decodeErrandContinuationRecord(turn, request.SubmissionID)
	if err != nil {
		return nil, err
	}
	message, err := scanErrandMessage(tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+errandMessageColumns+`
		FROM errand_messages m JOIN task_session_errands e ON e.id=m.errand_id WHERE m.errand_id=? AND m.id=?`), request.ErrandID, request.TurnID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.ErrInvalidErrand
	}
	if err != nil {
		return nil, err
	}
	if !matchesErrandContinuationMessage(message, request) {
		return nil, models.ErrInvalidErrand
	}
	prepared := &models.PreparedErrandContinuation{Generation: receipt.Generation, NativeID: receipt.NativeID}
	err = tx.GetContext(ctx, &prepared.ExecutionID, tx.Rebind(`SELECT execution_id FROM errand_execution_generations WHERE errand_id=? AND generation=?`), request.ErrandID, int64(receipt.Generation))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, db.ErrErrandSettlementPending
	}
	if err != nil {
		return nil, err
	}
	if prepared.ExecutionID == "" {
		if !errandHasConfirmedClose(row) || row.TeardownIntent.Target.OwnerGeneration+1 != prepared.Generation {
			return nil, db.ErrErrandSettlementPending
		}
		prepared.Predecessor = row.TeardownIntent.Target
		return prepared, nil
	}
	prepared.Accepted, err = hasAcceptedErrandContinuation(ctx, tx, request, prepared.ExecutionID)
	return prepared, err
}

func matchesErrandContinuationMessage(message *models.Message, request models.ErrandContinuation) bool {
	parent, _ := message.Metadata["sender_session_id"].(string)
	return message.TurnID == request.TurnID && message.AuthorType == models.MessageAuthorUser &&
		message.AuthorID == request.AuthorID && message.Content == request.Content && parent == request.ParentSessionID
}

func decodeErrandContinuationRecord(turn *models.Turn, submissionID string) (errandContinuationRecord, error) {
	var receipt errandContinuationRecord
	encoded, ok := turn.Metadata[errandContinuationMetadata].(string)
	if !ok || json.Unmarshal([]byte(encoded), &receipt) != nil || receipt.Generation == 0 || receipt.Generation > math.MaxInt64 || receipt.NativeID == "" || receipt.SubmissionID != submissionID {
		return receipt, models.ErrInvalidErrand
	}
	return receipt, nil
}

func hasAcceptedErrandContinuation(ctx context.Context, reader errandAdmissionReader, request models.ErrandContinuation, executionID string) (bool, error) {
	var accepted bool
	err := reader.QueryRowContext(ctx, reader.Rebind(`SELECT EXISTS (
		SELECT 1 FROM agent_delivery_submissions s JOIN agent_delivery_effects a
		 ON a.effect_key=? AND a.effect_type=? AND a.session_id=s.session_id AND a.incarnation_id=s.incarnation_id
		 AND a.harness_generation=s.harness_generation AND a.submission_id=s.id AND a.turn_id=?
		WHERE s.id=? AND s.session_id=? AND s.dispatch_attempt_id=? AND (
		 s.state='accepted' OR (s.state IN ('completed','failed','cancelled') AND EXISTS (
		  SELECT 1 FROM agent_delivery_effects terminal WHERE terminal.effect_type=? AND terminal.state=?
		  AND terminal.sequence>0 AND terminal.session_id=a.session_id AND terminal.incarnation_id=a.incarnation_id
		  AND terminal.stream_id=a.stream_id AND terminal.harness_generation=a.harness_generation
		  AND terminal.submission_id=s.id AND terminal.turn_id=a.turn_id AND terminal.outcome=s.state))))`),
		errandPromptAdmissionEffect+":"+request.SubmissionID, errandPromptAdmissionEffect, request.TurnID,
		request.SubmissionID, models.ErrandExecutionIdentity(request.ErrandID), executionID,
		models.DeliveryTerminalEffectType, models.DeliveryEffectBlockResolved).Scan(&accepted)
	return accepted, err
}

// ReadOwnedErrandContinuationAcceptance recovers only a durable dispatch
// decision for the exact parent continuation.
func (r *Repository) ReadOwnedErrandContinuationAcceptance(ctx context.Context, admission models.ErrandParentAdmission) (models.ErrandAdmissionState, bool, error) {
	unknown := models.ErrandAdmissionUnknown
	turn, err := r.GetErrandTurn(ctx, admission.ErrandID, admission.TurnID)
	if errors.Is(err, sql.ErrNoRows) {
		return unknown, false, nil
	}
	if err != nil {
		return unknown, false, err
	}
	if turn.TaskID != admission.TaskID || admission.SenderSessionID != admission.ParentSessionID {
		return unknown, true, models.ErrInvalidErrand
	}
	if state, err := readErrandAdmission(ctx, r.ro, admission); err != nil || state == "" {
		return unknown, true, err
	}
	receipt, err := decodeErrandContinuationRecord(turn, admission.SubmissionID)
	if err != nil {
		return unknown, true, err
	}
	var executionID string
	err = r.ro.GetContext(ctx, &executionID, r.ro.Rebind(`SELECT execution_id FROM errand_execution_generations
		WHERE errand_id=? AND generation=?`), admission.ErrandID, int64(receipt.Generation))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && executionID == "") {
		return unknown, true, nil
	}
	if err != nil {
		return unknown, true, err
	}
	accepted, err := hasAcceptedErrandContinuation(ctx, r.ro, models.ErrandContinuation{
		ErrandID: admission.ErrandID, TurnID: admission.TurnID, SubmissionID: admission.SubmissionID,
	}, executionID)
	if err == nil && accepted {
		return models.ErrandAdmissionAccepted, true, nil
	}
	return unknown, true, err
}

func readErrandContinuationPredecessor(ctx context.Context, tx *sqlx.Tx, row *models.Errand) (*errandContinuationRecord, error) {
	if !errandHasConfirmedClose(row) {
		return nil, db.ErrErrandSettlementPending
	}
	target := row.TeardownIntent.Target
	var nativeID string
	err := tx.GetContext(ctx, &nativeID, tx.Rebind(`SELECT b.native_session_id
		FROM errand_execution_generations g
		JOIN agent_delivery_effects a ON a.effect_key=? AND a.effect_type=? AND a.session_id=? AND a.turn_id=?
		JOIN errand_harness_bindings b ON b.errand_id=g.errand_id AND b.owner_generation=g.generation
		 AND b.execution_id=g.execution_id AND b.stream_id=a.stream_id AND b.incarnation_id=a.incarnation_id
		 AND b.harness_generation=a.harness_generation
		WHERE g.errand_id=? AND g.generation=? AND g.execution_id=? AND g.active_submission_id=''
		 AND g.generation=(SELECT MAX(latest.generation) FROM errand_execution_generations latest WHERE latest.errand_id=g.errand_id)`),
		errandPromptAdmissionEffect+":"+target.SubmissionID, errandPromptAdmissionEffect, models.ErrandExecutionIdentity(row.ID),
		target.TurnID, row.ID, int64(target.OwnerGeneration), target.ExecutionID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && nativeID == "") {
		return nil, db.ErrErrandSettlementPending
	}
	if err != nil {
		return nil, err
	}
	return &errandContinuationRecord{Generation: target.OwnerGeneration + 1, NativeID: nativeID}, nil
}

func errandHasConfirmedClose(row *models.Errand) bool {
	intent := row.TeardownIntent
	return row.Lifecycle == models.ErrandLifecycleLaunched && intent != nil &&
		intent.Purpose == models.ErrandTeardownPurposeCompletion && intent.State == models.ErrandTeardownIntentCompleted &&
		intent.Target.TaskID == row.TaskID && intent.Target.ErrandID == row.ID &&
		intent.Target.TurnID == row.LatestSettledTurnID && intent.Target.ExecutionID != "" &&
		intent.Target.OwnerGeneration > 0 && intent.Target.OwnerGeneration < math.MaxInt64
}

func (r *Repository) insertErrandContinuation(ctx context.Context, tx *sqlx.Tx, request models.ErrandContinuation, receipt *errandContinuationRecord) error {
	receipt.SubmissionID = request.SubmissionID
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO errand_execution_generations (errand_id,generation,execution_id) VALUES (?,?,'')`), request.ErrandID, int64(receipt.Generation)); err != nil {
		return err
	}
	now := errandTranscriptTime(r.nowUTC())
	turn := &models.Turn{ID: request.TurnID, TaskID: request.TaskID, StartedAt: now, CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]any{errandContinuationMetadata: string(encoded)}}
	if err = insertErrandTurn(ctx, tx, request.ErrandID, turn); err != nil {
		return err
	}
	message := &models.Message{ID: request.TurnID, TaskID: request.TaskID, TurnID: request.TurnID,
		AuthorID: request.AuthorID, AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage,
		Content: request.Content, CreatedAt: now, UpdatedAt: now}
	if request.ParentSessionID != "" {
		message.Metadata = map[string]any{"sender_session_id": request.ParentSessionID}
	}
	return insertErrandMessageTx(ctx, tx, request.ErrandID, message)
}
