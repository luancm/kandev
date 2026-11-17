package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) ReserveErrandExecutionGeneration(ctx context.Context, errandID string, expected uint64) (*models.ErrandExecutionDescriptor, error) {
	if errandID == "" || expected >= math.MaxInt64 {
		return nil, fmt.Errorf("invalid errand generation")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var taskID string
	if err = tx.GetContext(ctx, &taskID, tx.Rebind(`SELECT task_id FROM task_session_errands WHERE id=?`), errandID); err != nil {
		return nil, err
	}
	if err = r.lockTaskRowInTx(ctx, tx, taskID); err != nil {
		return nil, err
	}
	var current int64
	if err = tx.GetContext(ctx, &current, tx.Rebind(`SELECT COALESCE(MAX(generation),0) FROM errand_execution_generations WHERE errand_id=?`), errandID); err != nil {
		return nil, err
	}
	if current < 0 || uint64(current) != expected {
		return nil, fmt.Errorf("errand generation conflict")
	}
	next := current + 1
	if _, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO errand_execution_generations (errand_id,generation,execution_id) VALUES (?,?,'')`), errandID, next); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &models.ErrandExecutionDescriptor{ErrandID: errandID, TaskID: taskID, Generation: uint64(next)}, nil
}

func (r *Repository) BindErrandExecution(ctx context.Context, errandID string, generation uint64, executionID string) (bool, error) {
	if errandID == "" || generation == 0 || generation > math.MaxInt64 || executionID == "" {
		return false, fmt.Errorf("invalid errand execution identity")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var taskID string
	if err = tx.GetContext(ctx, &taskID, tx.Rebind(`SELECT task_id FROM task_session_errands WHERE id=?`), errandID); err != nil {
		return false, err
	}
	if err = r.lockTaskRowInTx(ctx, tx, taskID); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_execution_generations SET execution_id=? WHERE errand_id=? AND generation=? AND execution_id='' AND generation=(SELECT MAX(generation) FROM errand_execution_generations WHERE errand_id=?)`), executionID, errandID, int64(generation), errandID)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return count == 1, nil
}

func (r *Repository) GetErrandExecutionDescriptor(ctx context.Context, errandID string) (*models.ErrandExecutionDescriptor, error) {
	var result models.ErrandExecutionDescriptor
	var generation int64
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT g.errand_id,e.task_id,g.generation,g.execution_id FROM errand_execution_generations g JOIN task_session_errands e ON e.id=g.errand_id WHERE g.errand_id=? ORDER BY g.generation DESC LIMIT 1`), errandID).Scan(&result.ErrandID, &result.TaskID, &generation, &result.ExecutionID)
	if err != nil {
		return nil, err
	}
	if generation <= 0 {
		return nil, sql.ErrNoRows
	}
	result.Generation = uint64(generation)
	return &result, nil
}
