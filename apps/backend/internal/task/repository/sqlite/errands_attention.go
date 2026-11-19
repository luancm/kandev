package sqlite

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func ownedErrandRecoveryAttention(driver string) string {
	marker := dialect.JSONExtract(driver, "t.metadata", models.ErrandTurnRecoveryUnresolvedKey)
	return `(e.lifecycle='launched' AND EXISTS (
	 SELECT 1 FROM errand_execution_generations g JOIN errand_turns t ON t.errand_id=g.errand_id AND t.id=g.active_turn_id
	 JOIN errand_harness_bindings b ON b.errand_id=g.errand_id AND b.owner_generation=g.generation AND b.execution_id=g.execution_id
	 WHERE g.errand_id=e.id AND g.active_submission_id<>'' AND t.completed_at IS NULL
	 AND g.generation=(SELECT MAX(generation) FROM errand_execution_generations WHERE errand_id=e.id)
	 AND b.stream_id=` + marker + `
	 AND b.binding_sequence=(SELECT MAX(binding_sequence) FROM errand_harness_bindings WHERE errand_id=e.id AND owner_generation=g.generation)))`
}

// ErrandRecoveryAttention reads the same exact observation used for capacity.
func (r *Repository) ErrandRecoveryAttention(ctx context.Context, id string) (bool, error) {
	var attention bool
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT EXISTS (SELECT 1
	FROM task_session_errands e
 WHERE e.id = ? AND `+ownedErrandRecoveryAttention(r.ro.DriverName())+`)`), id).Scan(&attention)
	if err != nil {
		return false, fmt.Errorf("read errand recovery attention: %w", err)
	}
	return attention, nil
}

func liveErrandPredicate(driver string) string {
	return `(e.lifecycle = 'queued' OR (e.lifecycle = 'launched' AND ` + ownedErrandWorking(driver) + `))`
}

// HasWorkingErrands reports owned allocations and admitted work that can still use the task workspace.
func (r *Repository) HasWorkingErrands(ctx context.Context, taskID string) (bool, error) {
	var working bool
	err := r.ro.GetContext(ctx, &working, r.ro.Rebind(`SELECT EXISTS (
		SELECT 1 FROM task_session_errands e WHERE e.task_id=?
		 AND e.lifecycle IN ('queued','launched') AND `+ownedErrandWorking(r.ro.DriverName())+`
	)`), taskID)
	if err != nil {
		return false, fmt.Errorf("read errand workspace activity: %w", err)
	}
	return working, nil
}

const ownedErrandExists = `EXISTS (SELECT 1 FROM errand_execution_generations owner WHERE owner.errand_id=e.id)`

func ownedErrandWorking(driver string) string {
	closedGeneration := dialect.JSONExtractPath(driver, "NULLIF(e.teardown_intent,'')", "target", "owner_generation")
	return `EXISTS (SELECT 1 FROM errand_execution_generations owner
 WHERE owner.errand_id=e.id AND owner.generation=(
 SELECT MAX(current.generation) FROM errand_execution_generations current WHERE current.errand_id=e.id)
 AND (e.last_outcome='' OR owner.active_submission_id<>'' OR owner.generation > CAST(` + closedGeneration + ` AS BIGINT)))`
}
