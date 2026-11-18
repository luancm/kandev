package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) AppendErrandHarnessBinding(ctx context.Context, binding *models.ErrandHarnessBinding, expected uint64) error {
	if !validErrandHarnessBinding(binding, expected) {
		return fmt.Errorf("invalid errand harness binding")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var taskID string
	if err = tx.GetContext(ctx, &taskID, tx.Rebind(`SELECT task_id FROM task_session_errands WHERE id=?`), binding.ErrandID); err != nil {
		return err
	}
	if err = r.lockTaskRowInTx(ctx, tx, taskID); err != nil {
		return err
	}
	key := models.ErrandExecutionIdentity(binding.ErrandID)
	if err = lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{key}, nil); err != nil {
		return err
	}
	if err = r.ensureAgentDeliveryNoRetirementJob(ctx, tx, key); err != nil {
		return err
	}
	var existing models.ErrandHarnessBinding
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT binding_sequence,execution_id,incarnation_id,harness_generation,stream_id,native_session_id FROM errand_harness_bindings WHERE errand_id=? AND owner_generation=? AND stream_id=?`), binding.ErrandID, int64(binding.Generation), binding.StreamID).Scan(&existing.Sequence, &existing.ExecutionID, &existing.IncarnationID, &existing.HarnessGeneration, &existing.StreamID, &existing.NativeSessionID)
	if err == nil {
		if err = reconcileErrandHarnessBinding(ctx, tx, binding, &existing); err != nil {
			return err
		}
		binding.Sequence = existing.Sequence
		return tx.Commit()
	}
	if err != sql.ErrNoRows {
		return err
	}
	next, err := appendErrandHarnessBindingTx(ctx, tx, binding, expected)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	binding.Sequence = next
	return nil
}

func (r *Repository) CompleteErrandHarnessNativeSession(ctx context.Context, binding *models.ErrandHarnessBinding, nativeID string) error {
	if binding == nil || binding.ErrandID == "" || binding.Generation == 0 || binding.Generation > math.MaxInt64 || binding.Sequence == 0 || binding.Sequence > math.MaxInt64 || binding.ExecutionID == "" || binding.IncarnationID == "" || binding.HarnessGeneration <= 0 || binding.StreamID == "" || nativeID == "" {
		return fmt.Errorf("invalid errand harness native session")
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE errand_harness_bindings SET native_session_id=? WHERE errand_id=? AND owner_generation=? AND binding_sequence=? AND execution_id=? AND incarnation_id=? AND harness_generation=? AND stream_id=? AND (native_session_id='' OR native_session_id=?)`), nativeID, binding.ErrandID, int64(binding.Generation), int64(binding.Sequence), binding.ExecutionID, binding.IncarnationID, binding.HarnessGeneration, binding.StreamID, nativeID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("errand harness native session conflict")
	}
	return nil
}

func reconcileErrandHarnessBinding(ctx context.Context, tx *sqlx.Tx, binding, existing *models.ErrandHarnessBinding) error {
	if existing.ExecutionID != binding.ExecutionID || existing.IncarnationID != binding.IncarnationID || existing.HarnessGeneration != binding.HarnessGeneration || (binding.NativeSessionID != "" && existing.NativeSessionID != "" && existing.NativeSessionID != binding.NativeSessionID) {
		return fmt.Errorf("errand harness binding conflict")
	}
	if existing.NativeSessionID != "" || binding.NativeSessionID == "" {
		return nil
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_harness_bindings SET native_session_id=? WHERE errand_id=? AND owner_generation=? AND binding_sequence=? AND native_session_id=''`), binding.NativeSessionID, binding.ErrandID, int64(binding.Generation), int64(existing.Sequence))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("errand harness native session conflict")
	}
	return nil
}

func validErrandHarnessBinding(binding *models.ErrandHarnessBinding, expected uint64) bool {
	if binding == nil {
		return false
	}
	return binding.ErrandID != "" && binding.Generation > 0 && binding.Generation <= math.MaxInt64 && expected < math.MaxInt64 && binding.ExecutionID != "" && binding.IncarnationID != "" && binding.HarnessGeneration > 0 && binding.StreamID != ""
}

func appendErrandHarnessBindingTx(ctx context.Context, tx *sqlx.Tx, binding *models.ErrandHarnessBinding, expected uint64) (uint64, error) {
	var err error
	var execution string
	var current int64
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT generation,execution_id FROM errand_execution_generations WHERE errand_id=? ORDER BY generation DESC LIMIT 1`), binding.ErrandID).Scan(&current, &execution)
	if err != nil {
		return 0, err
	}
	if current != int64(binding.Generation) || execution != binding.ExecutionID {
		return 0, fmt.Errorf("errand execution ownership changed")
	}
	var last int64
	if err = tx.GetContext(ctx, &last, tx.Rebind(`SELECT COALESCE(MAX(binding_sequence),0) FROM errand_harness_bindings WHERE errand_id=? AND owner_generation=?`), binding.ErrandID, int64(binding.Generation)); err != nil {
		return 0, err
	}
	if last < 0 || uint64(last) != expected {
		return 0, fmt.Errorf("errand harness binding generation conflict")
	}
	next := last + 1
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO errand_harness_bindings (errand_id,owner_generation,binding_sequence,execution_id,incarnation_id,harness_generation,stream_id,native_session_id) VALUES (?,?,?,?,?,?,?,?)`), binding.ErrandID, int64(binding.Generation), next, binding.ExecutionID, binding.IncarnationID, binding.HarnessGeneration, binding.StreamID, binding.NativeSessionID)
	if err != nil {
		return 0, err
	}
	return uint64(next), nil
}

func (r *Repository) LatestErrandHarnessSequence(ctx context.Context, errandID string, generation uint64) (uint64, error) {
	if errandID == "" || generation == 0 || generation > math.MaxInt64 {
		return 0, fmt.Errorf("invalid errand generation")
	}
	var sequence int64
	err := r.ro.GetContext(ctx, &sequence, r.ro.Rebind(`SELECT COALESCE(MAX(binding_sequence),0) FROM errand_harness_bindings WHERE errand_id=? AND owner_generation=?`), errandID, int64(generation))
	if err != nil {
		return 0, err
	}
	if sequence < 0 {
		return 0, fmt.Errorf("invalid errand harness sequence")
	}
	return uint64(sequence), nil
}

func (r *Repository) ErrandHarnessBindingStale(ctx context.Context, taskID string, binding *models.ErrandHarnessBinding) (bool, error) {
	if !validErrandHarnessBinding(binding, 0) || taskID == "" {
		return false, fmt.Errorf("invalid errand harness identity")
	}
	var sequence, currentOwner, currentBinding int64
	var nativeSessionID string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT b.binding_sequence,(SELECT MAX(g.generation) FROM errand_execution_generations g WHERE g.errand_id=b.errand_id),(SELECT MAX(n.binding_sequence) FROM errand_harness_bindings n WHERE n.errand_id=b.errand_id AND n.owner_generation=b.owner_generation),b.native_session_id FROM errand_harness_bindings b JOIN task_session_errands e ON e.id=b.errand_id WHERE b.errand_id=? AND e.task_id=? AND b.owner_generation=? AND b.execution_id=? AND b.incarnation_id=? AND b.harness_generation=? AND b.stream_id=?`), binding.ErrandID, taskID, int64(binding.Generation), binding.ExecutionID, binding.IncarnationID, binding.HarnessGeneration, binding.StreamID).Scan(&sequence, &currentOwner, &currentBinding, &nativeSessionID)
	if err != nil {
		return false, err
	}
	if binding.NativeSessionID != "" && binding.NativeSessionID != nativeSessionID {
		return false, sql.ErrNoRows
	}
	return currentOwner != int64(binding.Generation) || currentBinding != sequence, nil
}
