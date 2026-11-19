package orchestrator

import (
	"context"
	"database/sql"
	"errors"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

type ownedErrandTeardownStore interface {
	errands.TeardownIntentStore
	GetErrandExecutionDescriptor(context.Context, string) (*models.ErrandExecutionDescriptor, error)
}

func (a *ErrandAdapter) cancelOwnedErrand(ctx context.Context, row *models.Errand, commit func(errands.ErrandTeardownTarget) error) error {
	guard, err := a.svc.lockCancelInFlightGuardWithContext(ctx, row.ID)
	if err != nil {
		return err
	}
	defer guard.release()
	store, ok := a.svc.repo.(ownedErrandTeardownStore)
	if !ok {
		return errors.New("errand teardown persistence is unavailable")
	}
	current, err := store.GetErrand(ctx, row.ID)
	if err != nil {
		return err
	}
	if !sameObservedErrand(current, row) {
		return models.ErrErrandVersionConflict
	}
	target := models.ErrandTeardownTarget{TaskID: row.TaskID, ErrandID: row.ID, NoTurn: true}
	descriptor, err := store.GetErrandExecutionDescriptor(ctx, row.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if descriptor != nil {
		target.OwnerGeneration, target.ExecutionID = descriptor.Generation, descriptor.ExecutionID
		target.NoTurn = descriptor.ActiveTurnID == ""
		target.TurnID, target.SubmissionID = descriptor.ActiveTurnID, descriptor.ActiveSubmissionID
		target.PromptGeneration = descriptor.PromptGeneration
	}
	if err := commit(target); err != nil {
		return err
	}
	committed, err := store.GetErrand(ctx, row.ID)
	if err != nil {
		return err
	}
	if committed.TeardownIntent == nil || committed.TeardownIntent.State != models.ErrandTeardownIntentPending ||
		committed.TeardownIntent.Target != target || committed.Lifecycle != models.ErrandLifecycleCancelled {
		return models.ErrInvalidErrand
	}
	_, err = a.retryOwnedErrandTeardown(ctx, *committed.TeardownIntent)
	return err
}

func (a *ErrandAdapter) retryOwnedErrandTeardown(ctx context.Context, intent models.ErrandTeardownIntent) (errands.TeardownOutcome, error) {
	store, ok := a.svc.repo.(ownedErrandTeardownStore)
	if !ok {
		return "", errors.New("errand teardown persistence is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, cancellationOperationTTL)
	defer cancel()
	guard, err := a.svc.lockCancelInFlightGuardWithContext(ctx, "errand-teardown:"+intent.Target.ErrandID)
	if err != nil {
		return "", err
	}
	defer guard.release()
	row, err := store.GetErrand(ctx, intent.Target.ErrandID)
	if err != nil {
		return "", err
	}
	if row.TeardownIntent == nil || !sameTeardownIntent(*row.TeardownIntent, intent) {
		return "", models.ErrErrandVersionConflict
	}
	switch row.TeardownIntent.State {
	case models.ErrandTeardownIntentCompleted:
		return errands.TeardownCompleted, nil
	case models.ErrandTeardownIntentSuperseded:
		return errands.TeardownSuperseded, nil
	}
	if err := a.confirmOwnedErrandStopped(ctx, store, row, intent.Target); err != nil {
		return "", err
	}
	if _, err := store.TransitionErrandTeardownIntent(ctx, row.ID, intent.ID, row.Version, models.ErrandTeardownIntentCompleted); err != nil {
		return "", err
	}
	a.svc.signalCeilingSweep()
	return errands.TeardownCompleted, nil
}

func (a *ErrandAdapter) confirmOwnedErrandStopped(ctx context.Context, store ownedErrandTeardownStore, row *models.Errand, target models.ErrandTeardownTarget) error {
	descriptor, err := store.GetErrandExecutionDescriptor(ctx, row.ID)
	if errors.Is(err, sql.ErrNoRows) && target.OwnerGeneration == 0 {
		// Cancellation is durable and no generation was ever admitted.
		return nil
	}
	if err != nil {
		return err
	}
	if descriptor.Generation != target.OwnerGeneration || descriptor.TaskID != target.TaskID ||
		(target.ExecutionID != "" && target.ExecutionID != descriptor.ExecutionID) {
		return models.ErrExecutionRotated
	}
	if descriptor.ExecutionID == "" {
		return runtimeapi.ErrErrandRecoveryRequiresReconnect
	}
	task, err := a.svc.repo.GetTask(ctx, row.TaskID)
	if err != nil {
		return err
	}
	owner := runtimeapi.ExecutionOwner{
		Kind: runtimeapi.ExecutionOwnerErrand, WorkspaceID: task.WorkspaceID, TaskID: row.TaskID,
		ErrandID: row.ID, Generation: descriptor.Generation, AgentProfileID: row.AgentProfileID,
	}
	return a.stopOwnedErrandExecution(ctx, owner, descriptor.ExecutionID)
}

func (a *ErrandAdapter) stopOwnedErrandExecution(ctx context.Context, owner runtimeapi.ExecutionOwner, executionID string) error {
	execution, err := a.runtime.GetExecution(ctx, executionID)
	if err == nil {
		if execution == nil || execution.ID != executionID || execution.Owner != owner || execution.SessionID != "" || execution.TaskID != "" {
			return models.ErrInvalidErrand
		}
		err = a.runtime.Stop(ctx, executionID, errands.StopReasonErrandCancelled)
	}
	if !errors.Is(err, runtimeapi.ErrNotFound) {
		return err
	}
	manager, ok := a.svc.agentManager.(interface {
		StopErrandOwnerForRecovery(context.Context, runtimeapi.ExecutionOwner, string) error
	})
	if !ok {
		return runtimeapi.ErrErrandRecoveryRequiresReconnect
	}
	return manager.StopErrandOwnerForRecovery(ctx, owner, executionID)
}
