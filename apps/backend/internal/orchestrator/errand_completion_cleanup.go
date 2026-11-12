package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

// RepairLegacyErrandCompletion adopts only a latest exact successful receipt
// whose retained session identity is unchanged and whose runtime is confirmed absent.
func (a *ErrandAdapter) RepairLegacyErrandCompletion(ctx context.Context, observed *models.Errand) error {
	if observed == nil || observed.ID == "" {
		return models.ErrInvalidErrand
	}
	_, err := a.withErrandTeardownOwner(ctx, observed.ID, func(ctx context.Context, owner *errandTeardownOwner, store errandTeardownStore) (errands.TeardownOutcome, error) {
		return owner.repairLegacyCompletion(ctx, store, observed)
	})
	return err
}

func (o *errandTeardownOwner) retryCompletion(ctx context.Context, store errandTeardownStore, row *models.Errand, intent models.ErrandTeardownIntent) (errands.TeardownOutcome, error) {
	switch row.TeardownIntent.State {
	case models.ErrandTeardownIntentCompleted:
		return errands.TeardownCompleted, nil
	case models.ErrandTeardownIntentSuperseded:
		return errands.TeardownSuperseded, nil
	case models.ErrandTeardownIntentPending:
		var err error
		row, err = o.closeCompletion(ctx, store, row, intent)
		if err != nil {
			return "", err
		}
	}
	if row.TeardownIntent.State == models.ErrandTeardownIntentSuperseded {
		return errands.TeardownSuperseded, nil
	}
	if row.TeardownIntent.State != models.ErrandTeardownIntentClosed {
		return "", errors.New("errand completion has no close receipt")
	}
	if _, err := o.validateCompletionAuthority(ctx, store, row, intent, nil); err != nil {
		return "", err
	}
	if err := o.projectCompletedSession(ctx, row, intent); err != nil {
		return "", err
	}
	latest, err := store.GetErrand(ctx, row.ID)
	if err != nil {
		return "", err
	}
	if !rowHasTeardownIntent(latest, intent) || latest.TeardownIntent.State != models.ErrandTeardownIntentClosed {
		return "", models.ErrErrandVersionConflict
	}
	if _, err := store.TransitionErrandTeardownIntent(ctx, latest.ID, intent.ID, latest.Version, models.ErrandTeardownIntentCompleted); err != nil {
		return "", err
	}
	return errands.TeardownCompleted, nil
}

func (o *errandTeardownOwner) closeCompletion(ctx context.Context, store errandTeardownStore, row *models.Errand, intent models.ErrandTeardownIntent) (*models.Errand, error) {
	session, executionID, outcome, err := o.classify(ctx, store, row, intent.Target)
	if err != nil {
		return nil, err
	}
	if outcome == errands.TeardownSuperseded {
		return store.TransitionErrandTeardownIntent(ctx, row.ID, intent.ID, row.Version, models.ErrandTeardownIntentSuperseded)
	}
	if _, err := o.validateCompletionAuthority(ctx, store, row, intent, session); err != nil {
		return nil, err
	}
	if o.successorAdmissionPending(intent.Target.SessionID) {
		return nil, errors.New("errand completion close deferred while a successor prompt is admitted")
	}
	if executionID != "" {
		if err := o.stopCompleted(ctx, intent.Target, executionID); err != nil {
			return nil, err
		}
	}
	latest, err := store.GetErrand(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	if !rowHasTeardownIntent(latest, intent) || latest.TeardownIntent.State != models.ErrandTeardownIntentPending {
		return nil, models.ErrErrandVersionConflict
	}
	if _, err := o.validateCompletionAuthority(ctx, store, latest, intent, nil); err != nil {
		return nil, err
	}
	return store.TransitionErrandTeardownIntent(ctx, latest.ID, intent.ID, latest.Version, models.ErrandTeardownIntentClosed)
}

func rowHasTeardownIntent(row *models.Errand, intent models.ErrandTeardownIntent) bool {
	return row.TeardownIntent != nil && sameTeardownIntent(*row.TeardownIntent, intent)
}

func (o *errandTeardownOwner) successorAdmissionPending(sessionID string) bool {
	return o.svc.isQueuedDispatchInFlight(sessionID) || o.svc.isSteerInFlight(sessionID) ||
		o.svc.reservedPromptTurnID(sessionID) != ""
}

func (o *errandTeardownOwner) validateCompletionAuthority(
	ctx context.Context,
	store errandTeardownStore,
	row *models.Errand,
	intent models.ErrandTeardownIntent,
	session *models.TaskSession,
) (*models.TaskSession, error) {
	if !completionRowMatchesIntent(row, intent) {
		return nil, models.ErrErrandVersionConflict
	}
	if session == nil {
		var err error
		session, err = o.svc.repo.GetTaskSession(ctx, intent.Target.SessionID)
		if err != nil {
			return nil, err
		}
	}
	if !errandTeardownSessionMatches(row, session) || session.QueueIncarnationID != intent.Target.SessionIncarnation {
		return nil, models.ErrErrandVersionConflict
	}
	executionID, absent, err := o.executionForSession(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	if !absent && executionID != intent.Target.ExecutionID {
		return nil, models.ErrErrandVersionConflict
	}
	turn, err := o.svc.currentErrandTurn(ctx, store, intent.Target.SessionID)
	if err != nil {
		return nil, err
	}
	if !completionTurnMatches(turn, intent.Target) {
		return nil, models.ErrErrandVersionConflict
	}
	effect, err := store.GetErrandTurnSettlementDecision(ctx, row.ID, intent.Target.SessionIncarnation, intent.Target.TurnID)
	if err != nil {
		return nil, err
	}
	if !completionDecisionMatchesTarget(effect, intent.Target) {
		return nil, errors.New("errand completion lacks an exact successful terminal receipt")
	}
	return session, nil
}

func (o *errandTeardownOwner) stopCompleted(ctx context.Context, target models.ErrandTeardownTarget, executionID string) error {
	claim, claimed := o.svc.claimExecutionTeardownWithToken(target.SessionID, executionID, executionTeardownIntentGraceful)
	if !claimed {
		if prior, ok := o.svc.executionTeardownClaimFor(target.SessionID, executionID); ok && prior.cleanupCompleted {
			return nil
		}
		return errors.New("exact errand execution teardown is already owned")
	}
	key := terminalExecutionKey(target.SessionID, executionID)
	o.svc.markExecutionCompleted(target.SessionID, executionID)
	o.unlock()
	o.svc.retireExecutionActivityAndPublish(ctx, target.TaskID, target.SessionID, executionID)
	if o.svc.lspLeases != nil {
		o.svc.lspLeases.StopLSPLeasesForExecution(executionID)
	}
	stopErr := o.svc.executor.StopExecution(ctx, executionID, completedExecutionCleanupReason, true)
	if stopErr != nil && !agentruntime.IsNotFound(stopErr) {
		o.svc.executionTeardownClaims.CompareAndDelete(key, claim)
		return fmt.Errorf("exact errand execution close is not confirmed: %w", stopErr)
	}
	o.svc.completeExecutionTeardownClaim(target.SessionID, executionID, claim)
	o.svc.settleBackgroundWorkAfterTeardown(ctx, target.SessionID, executionID)
	if err := o.guard.relockWithContext(ctx); err != nil {
		return err
	}
	return nil
}

func (o *errandTeardownOwner) projectCompletedSession(ctx context.Context, row *models.Errand, intent models.ErrandTeardownIntent) error {
	session, err := o.svc.repo.GetTaskSession(ctx, intent.Target.SessionID)
	if err != nil {
		return err
	}
	if !errandTeardownSessionMatches(row, session) || session.QueueIncarnationID != intent.Target.SessionIncarnation {
		return models.ErrErrandVersionConflict
	}
	if session.State == models.TaskSessionStateCompleted {
		return nil
	}
	if isTerminalSessionState(session.State) {
		return fmt.Errorf("errand session reached incompatible terminal state %s", session.State)
	}
	changed, updatedAt, err := o.svc.repo.UpdateTaskSessionStateIfCurrentIdentity(ctx, session.TaskID, session.ID,
		session.QueueIncarnationID, session.State, models.TaskSessionStateCompleted, "")
	if err != nil {
		return err
	}
	if !changed {
		return models.ErrErrandVersionConflict
	}
	after := taskSessionAfterStateWrite(session, models.TaskSessionStateCompleted, "", updatedAt)
	o.transition = &errandTeardownTransition{before: session, after: after}
	return nil
}

func (a *ErrandAdapter) cancelPendingCompletion(
	ctx context.Context,
	observed *models.Errand,
	commit func(errands.ErrandTeardownTarget) error,
) (bool, error) {
	if a.svc == nil || !completionIntentCanBeCancelled(observed.TeardownIntent) {
		return false, nil
	}
	store, ok := a.svc.repo.(errandTeardownStore)
	if !ok {
		return false, nil
	}
	guard, err := a.svc.lockCancelInFlightGuardWithContext(ctx, observed.ID)
	if err != nil {
		return true, err
	}
	defer guard.release()
	current, err := store.GetErrand(ctx, observed.ID)
	if err != nil {
		return false, err
	}
	if !sameObservedErrand(current, observed) {
		return false, models.ErrErrandVersionConflict
	}
	intent := current.TeardownIntent
	if !completionIntentCanBeCancelled(intent) {
		return false, nil
	}
	turn, err := a.svc.currentErrandTurn(ctx, store, observed.ID)
	if err != nil {
		return true, err
	}
	if turn == nil || turn.ID != intent.Target.TurnID {
		return false, nil
	}
	if err := commit(intent.Target); err != nil {
		return true, err
	}
	guard.unlock()
	return true, a.retryCompletionCancellation(ctx, store, observed.ID, intent.Target)
}

func (a *ErrandAdapter) retryCompletionCancellation(ctx context.Context, store errandTeardownStore, sessionID string, target models.ErrandTeardownTarget) error {
	committed, err := store.GetErrand(ctx, sessionID)
	if err != nil {
		return err
	}
	if !matchingPendingCancellation(committed.TeardownIntent, target) {
		return errors.New("cancellation callback did not preserve the successful close target")
	}
	waitErr := a.svc.waitForCancelInFlight(ctx, sessionID)
	if waitErr != nil && (errors.Is(waitErr, context.Canceled) || errors.Is(waitErr, context.DeadlineExceeded)) {
		return waitErr
	}
	latest, err := store.GetErrand(ctx, sessionID)
	if err != nil {
		return err
	}
	if !matchingPendingCancellation(latest.TeardownIntent, target) {
		return errors.Join(waitErr, models.ErrErrandVersionConflict)
	}
	_, err = a.RetryErrandTeardown(ctx, *latest.TeardownIntent)
	return err
}

func (owner *errandTeardownOwner) repairLegacyCompletion(ctx context.Context, store errandTeardownStore, observed *models.Errand) (errands.TeardownOutcome, error) {
	row, err := store.GetErrand(ctx, observed.ID)
	if err != nil {
		return "", err
	}
	if !legacyCompletionRowEligible(row) {
		return errands.TeardownSuperseded, nil
	}
	session, err := owner.svc.repo.GetTaskSession(ctx, row.ID)
	if errors.Is(err, models.ErrTaskSessionNotFound) {
		return errands.TeardownSuperseded, nil
	}
	if err != nil {
		return "", err
	}
	if !legacyCompletionSessionEligible(row, session) {
		return errands.TeardownSuperseded, nil
	}
	turn, err := owner.svc.currentErrandTurn(ctx, store, row.ID)
	if err != nil {
		return "", err
	}
	if turn == nil || turn.ID != row.LatestSettledTurnID || turn.CompletedAt == nil {
		return errands.TeardownSuperseded, nil
	}
	effect, err := store.GetErrandTurnSettlementDecision(ctx, row.ID, session.QueueIncarnationID, turn.ID)
	if err != nil {
		return "", err
	}
	if !successfulCompletionDecision(effect) {
		return errands.TeardownSuperseded, nil
	}
	executionID, absent, err := owner.executionForSession(ctx, row.ID)
	if err != nil {
		return "", err
	}
	if !absent {
		return errands.TeardownSuperseded, nil
	}
	return owner.adoptLegacyCompletion(ctx, store, row, session, turn, effect, executionID)
}

func (owner *errandTeardownOwner) adoptLegacyCompletion(ctx context.Context, store errandTeardownStore, row *models.Errand, session *models.TaskSession, turn *models.Turn, effect *models.AgentDeliveryEffect, executionID string) (errands.TeardownOutcome, error) {
	target := models.ErrandTeardownTarget{
		TaskID: row.TaskID, SessionID: row.ID, SessionIncarnation: session.QueueIncarnationID,
		ExecutionID: executionID, TurnID: turn.ID, SubmissionID: effect.SubmissionID,
	}
	intent := &models.ErrandTeardownIntent{
		Version: models.ErrandTeardownIntentVersion, ID: fmt.Sprintf("legacy-completion-%s", uuid.NewString()),
		State: models.ErrandTeardownIntentPending, Purpose: models.ErrandTeardownPurposeCompletion,
		RequestedAt: time.Now().UTC(), Target: target,
	}
	candidate := *row
	candidate.TeardownIntent = intent
	effect.TaskID = row.TaskID
	if _, err := store.CommitErrandTurnSettlement(ctx, &candidate, effect); err != nil {
		return "", err
	}
	committed, err := store.GetErrand(ctx, row.ID)
	if err != nil {
		return "", err
	}
	if committed.TeardownIntent == nil || !sameTeardownIntent(*committed.TeardownIntent, *intent) {
		return errands.TeardownSuperseded, nil
	}
	return owner.retry(ctx, store, *committed.TeardownIntent)
}

func legacyCompletionRowEligible(row *models.Errand) bool {
	return row.Lifecycle == models.ErrandLifecycleLaunched && row.TeardownIntent == nil && row.LatestSettledTurnID != ""
}
func legacyCompletionSessionEligible(row *models.Errand, session *models.TaskSession) bool {
	return errandTeardownSessionMatches(row, session) && session.QueueIncarnationID != ""
}
func successfulCompletionDecision(effect *models.AgentDeliveryEffect) bool {
	return effect != nil && effect.State == models.DeliveryEffectCompleted && strings.HasSuffix(effect.Outcome, ":done")
}

func sameObservedErrand(current, observed *models.Errand) bool {
	return current.Version == observed.Version && current.TaskID == observed.TaskID && current.ParentSessionID == observed.ParentSessionID
}
func completionIntentCanBeCancelled(intent *models.ErrandTeardownIntent) bool {
	return intent != nil && intent.Purpose == models.ErrandTeardownPurposeCompletion && (intent.State == models.ErrandTeardownIntentPending || intent.State == models.ErrandTeardownIntentClosed)
}
func matchingPendingCancellation(intent *models.ErrandTeardownIntent, target models.ErrandTeardownTarget) bool {
	return intent != nil && intent.Purpose == models.ErrandTeardownPurposeCancellation && intent.State == models.ErrandTeardownIntentPending && intent.Target == target
}
func completionRowMatchesIntent(row *models.Errand, intent models.ErrandTeardownIntent) bool {
	return row.Lifecycle == models.ErrandLifecycleLaunched && row.CancelSource == "" && row.LatestSettledTurnID == intent.Target.TurnID && rowHasTeardownIntent(row, intent)
}
func completionTurnMatches(turn *models.Turn, target models.ErrandTeardownTarget) bool {
	return turn != nil && turn.ID == target.TurnID && turn.CompletedAt != nil
}
func completionDecisionMatchesTarget(effect *models.AgentDeliveryEffect, target models.ErrandTeardownTarget) bool {
	return successfulCompletionDecision(effect) && effect.SessionID == target.SessionID && effect.IncarnationID == target.SessionIncarnation && effect.SubmissionID == target.SubmissionID && effect.TurnID == target.TurnID
}
