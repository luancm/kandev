package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// RecordErrandRecoveryObservation updates only the open turn of the current authenticated binding.
func (r *Repository) RecordErrandRecoveryObservation(ctx context.Context, observation models.ErrandRecoveryObservation) (bool, error) {
	binding := observation.Binding
	if !validErrandHarnessBinding(&binding, 0) || observation.TaskID == "" || observation.TurnID == "" ||
		observation.SubmissionID == "" || observation.PromptGeneration == 0 || observation.PromptGeneration > math.MaxInt64 {
		return false, models.ErrInvalidErrand
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockTaskRowInTx(ctx, tx, observation.TaskID); err != nil {
		return false, err
	}
	transportID := models.ErrandExecutionIdentity(binding.ErrandID)
	if err := lockAgentDeliveryWrites(ctx, tx, tx.DriverName(), []string{transportID}, []string{binding.StreamID}); err != nil {
		return false, err
	}
	metadata, err := readErrandRecoveryTurnMetadata(ctx, tx, observation)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	changed, err := writeErrandRecoveryTurnMetadata(ctx, tx, observation, metadata)
	if err != nil || !changed {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit Errand recovery observation: %w", err)
	}
	return true, nil
}

func readErrandRecoveryTurnMetadata(ctx context.Context, tx *sqlx.Tx, observation models.ErrandRecoveryObservation) (string, error) {
	const query = `SELECT t.metadata FROM task_session_errands e
	JOIN errand_execution_generations g ON g.errand_id=e.id
	JOIN errand_turns t ON t.errand_id=g.errand_id AND t.id=g.active_turn_id
	JOIN errand_harness_bindings b ON b.errand_id=g.errand_id AND b.owner_generation=g.generation AND b.execution_id=g.execution_id
	JOIN agent_delivery_effects a ON a.effect_key=? AND a.effect_type=? AND a.session_id=?
	 AND a.submission_id=g.active_submission_id AND a.turn_id=t.id AND a.stream_id=b.stream_id
	 AND a.incarnation_id=b.incarnation_id AND a.harness_generation=b.harness_generation
	WHERE e.id=? AND e.task_id=? AND e.lifecycle<>'cancelled' AND t.completed_at IS NULL
	 AND g.generation=? AND g.execution_id=? AND g.active_submission_id=? AND t.id=? AND g.prompt_generation=?
	 AND g.generation=(SELECT MAX(generation) FROM errand_execution_generations WHERE errand_id=e.id)
	 AND b.incarnation_id=? AND b.harness_generation=? AND b.stream_id=? AND b.native_session_id=?
	 AND b.binding_sequence=(SELECT MAX(binding_sequence) FROM errand_harness_bindings WHERE errand_id=e.id AND owner_generation=g.generation)`
	binding := observation.Binding
	var metadata string
	err := tx.GetContext(ctx, &metadata, tx.Rebind(query),
		errandPromptAdmissionEffect+":"+observation.SubmissionID, errandPromptAdmissionEffect, models.ErrandExecutionIdentity(binding.ErrandID),
		binding.ErrandID, observation.TaskID,
		int64(binding.Generation), binding.ExecutionID, observation.SubmissionID, observation.TurnID, int64(observation.PromptGeneration),
		binding.IncarnationID, binding.HarnessGeneration, binding.StreamID, binding.NativeSessionID)
	return metadata, err
}

func writeErrandRecoveryTurnMetadata(ctx context.Context, tx *sqlx.Tx, observation models.ErrandRecoveryObservation, raw string) (bool, error) {
	metadata := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return false, err
	}
	key := models.ErrandTurnRecoveryUnresolvedKey
	stream, err := json.Marshal(observation.Binding.StreamID)
	if err != nil {
		return false, err
	}
	if (string(metadata[key]) == string(stream)) == observation.Unresolved {
		return false, nil
	}
	if observation.Unresolved {
		if metadata == nil {
			metadata = make(map[string]json.RawMessage)
		}
		metadata[key] = stream
	} else {
		delete(metadata, key)
	}
	updated, err := json.Marshal(metadata)
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_turns SET metadata=? WHERE errand_id=? AND id=? AND metadata=? AND completed_at IS NULL`),
		string(updated), observation.Binding.ErrandID, observation.TurnID, raw)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
