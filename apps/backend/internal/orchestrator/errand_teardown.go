package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

type errandTeardownStore interface {
	errands.TeardownIntentStore
	GetLatestTurnBySessionID(context.Context, string) (*models.Turn, error)
	ListAgentDeliverySubmissions(context.Context, string) ([]*models.AgentDeliverySubmission, error)
}

type errandTeardownOwner struct {
	svc              *Service
	guard            *lockedCancelInFlightGuard
	operation        *cancelOperation
	releaseLifecycle func()
	transition       *errandTeardownTransition
}

type errandTeardownTransition struct {
	before *models.TaskSession
	after  *models.TaskSession
}

// CancelErrandWithTarget commits cancellation against a guarded runtime snapshot
// and acknowledges only a durably completed or superseded exact-target intent.
func (a *ErrandAdapter) CancelErrandWithTarget(ctx context.Context, row *models.Errand, commit func(errands.ErrandTeardownTarget) error) error {
	if row == nil || row.ID == "" || commit == nil {
		return models.ErrInvalidErrand
	}
	_, err := a.withErrandTeardownOwner(ctx, row.ID, func(ctx context.Context, owner *errandTeardownOwner, store errandTeardownStore) (errands.TeardownOutcome, error) {
		return owner.cancel(ctx, store, row, commit)
	})
	return err
}

func (o *errandTeardownOwner) cancel(ctx context.Context, store errandTeardownStore, row *models.Errand, commit func(errands.ErrandTeardownTarget) error) (errands.TeardownOutcome, error) {
	current, err := store.GetErrand(ctx, row.ID)
	if err != nil {
		return "", err
	}
	if current.Version != row.Version || current.TaskID != row.TaskID || current.ParentSessionID != row.ParentSessionID {
		return "", models.ErrErrandVersionConflict
	}
	if current.TeardownIntent != nil {
		return o.retry(ctx, store, *current.TeardownIntent)
	}
	var target models.ErrandTeardownTarget
	if current.Lifecycle == models.ErrandLifecycleCancelled {
		target, err = o.captureLegacyTarget(ctx, current)
	} else {
		target, err = o.captureTarget(ctx, store, current)
	}
	if err != nil {
		return "", err
	}
	if err := commit(target); err != nil {
		return "", err
	}
	committed, err := store.GetErrand(ctx, row.ID)
	if err != nil {
		return "", err
	}
	if committed.TeardownIntent == nil || committed.TeardownIntent.State != models.ErrandTeardownIntentPending || committed.TeardownIntent.Target != target {
		return "", errors.New("cancellation callback did not persist the captured pending teardown")
	}
	return o.retry(ctx, store, *committed.TeardownIntent)
}

// Legacy rows may recover only their immutable initial reservation, backed by
// positive admission evidence. Today's active turn is never historical proof.
func (o *errandTeardownOwner) captureLegacyTarget(ctx context.Context, row *models.Errand) (models.ErrandTeardownTarget, error) {
	target := models.ErrandTeardownTarget{TaskID: row.TaskID, SessionID: row.ID,
		FirstTurnID: row.FirstTurnID, ReservedSubmissionID: row.SubmissionID}
	turn, err := o.svc.turnService.GetTurn(ctx, row.FirstTurnID)
	if err != nil {
		return target, err
	}
	if !reservedTeardownSubmissionBound(turn, target, row.ParentSessionID) || !teardownTurnMatches(row, turn) {
		return target, errors.New("legacy errand lacks positive initial admission authority")
	}
	return target, validateCapturedTeardownTarget(row, target)
}

// RetryErrandTeardown uses only the persisted target. Startup callers must first
// finish authenticated runtime recovery so typed absence is authoritative.
func (a *ErrandAdapter) RetryErrandTeardown(ctx context.Context, intent errands.ErrandTeardownIntent) (errands.TeardownOutcome, error) {
	if intent.Target.SessionID == "" || intent.ID == "" {
		return "", models.ErrInvalidErrand
	}
	return a.withErrandTeardownOwner(ctx, intent.Target.SessionID, func(ctx context.Context, owner *errandTeardownOwner, store errandTeardownStore) (errands.TeardownOutcome, error) {
		return owner.retry(ctx, store, intent)
	})
}

func (a *ErrandAdapter) withErrandTeardownOwner(
	ctx context.Context, sessionID string,
	run func(context.Context, *errandTeardownOwner, errandTeardownStore) (errands.TeardownOutcome, error),
) (outcome errands.TeardownOutcome, err error) {
	if a.svc == nil || a.svc.agentManager == nil || a.svc.executor == nil || a.svc.turnService == nil {
		return "", errors.New("errand teardown runtime authority is unavailable")
	}
	store, ok := a.svc.repo.(errandTeardownStore)
	if !ok {
		return "", errors.New("errand teardown persistence capability is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, cancellationOperationTTL)
	defer cancel()
	operation, owner, _, accepted := a.svc.claimCancellationWithActionExclusive(sessionID, cancellationKindInternal, nil)
	if !owner || !accepted {
		return "", ErrCancelInFlight
	}
	defer func() { a.svc.finishCancellationWithActions(ctx, sessionID, operation, err) }()
	guard, err := a.svc.lockCancelInFlightGuardWithContext(ctx, sessionID)
	if err != nil {
		return "", err
	}
	defer guard.release()
	releaseLifecycle, acquired := a.svc.tryAcquireSessionLifecycleLock(sessionID)
	if !acquired {
		return "", errors.New("errand teardown yielded to session lifecycle ownership")
	}
	owned := &errandTeardownOwner{svc: a.svc, guard: guard, operation: operation, releaseLifecycle: releaseLifecycle}
	defer func() {
		owned.unlock()
		owned.publish(ctx)
	}()
	return run(ctx, owned, store)
}

func (o *errandTeardownOwner) unlock() {
	o.guard.unlock()
	if o.releaseLifecycle != nil {
		o.releaseLifecycle()
		o.releaseLifecycle = nil
	}
}

func (o *errandTeardownOwner) publish(ctx context.Context) {
	if o.transition == nil {
		return
	}
	before, after := o.transition.before, o.transition.after
	o.svc.releaseCeilingIfLeftPopulation(after.ID, before.State, after.State)
	o.svc.publishAcceptedTaskSessionState(ctx, after.TaskID, after.ID, before.State, after.State,
		after.ErrorMessage, &after.UpdatedAt, after)
}

func (o *errandTeardownOwner) captureTarget(ctx context.Context, store errandTeardownStore, row *models.Errand) (models.ErrandTeardownTarget, error) {
	target := models.ErrandTeardownTarget{TaskID: row.TaskID, SessionID: row.ID}
	session, err := o.svc.repo.GetTaskSession(ctx, row.ID)
	if errors.Is(err, models.ErrTaskSessionNotFound) && row.Lifecycle == models.ErrandLifecycleQueued {
		_, absent, lookupErr := o.executionForSession(ctx, row.ID)
		if lookupErr != nil || !absent {
			return target, errors.Join(lookupErr, errors.New("reserved errand runtime absence is unconfirmed"))
		}
		target.FirstTurnID, target.ReservedSubmissionID = row.FirstTurnID, row.SubmissionID
		return target, validateCapturedTeardownTarget(row, target)
	}
	if err != nil {
		return target, err
	}
	if !errandTeardownSessionMatches(row, session) || strings.TrimSpace(session.QueueIncarnationID) == "" {
		return target, models.ErrInvalidErrand
	}
	identity, err := o.svc.captureCancellationIdentity(ctx, row.ID)
	if err != nil {
		return target, err
	}
	if err := o.svc.capturePromptActivity(ctx, row.ID, &identity); err != nil {
		return target, err
	}
	executionID, _, err := o.executionForSession(ctx, row.ID)
	if err != nil {
		return target, err
	}
	if executionID != identity.executionID {
		return target, errors.New("errand prompt and execution inventories disagree")
	}
	turn, err := o.currentTurn(ctx, store, row.ID)
	if err != nil {
		return target, err
	}
	target.SessionIncarnation, target.ExecutionID = session.QueueIncarnationID, executionID
	target.PromptGeneration, target.NoTurn = identity.promptGeneration, turn == nil
	if turn != nil {
		target.TurnID = turn.ID
	}
	target.SubmissionID, err = teardownSubmission(ctx, store, row, session, turn)
	if err != nil {
		return target, err
	}
	return target, validateCapturedTeardownTarget(row, target)
}

func validateCapturedTeardownTarget(row *models.Errand, target models.ErrandTeardownTarget) error {
	cancelled := *row
	cancelled.Lifecycle, cancelled.CancelSource = models.ErrandLifecycleCancelled, models.ErrandCancelSourceUser
	intent := models.ErrandTeardownIntent{Version: models.ErrandTeardownIntentVersion, ID: "validation",
		State: models.ErrandTeardownIntentPending, RequestedAt: time.Now().UTC(), Target: target}
	return intent.ValidateFor(&cancelled)
}

func errandTeardownSessionMatches(row *models.Errand, session *models.TaskSession) bool {
	return session != nil && session.ID == row.ID && session.TaskID == row.TaskID &&
		models.IsErrandSession(session) && session.ParentSessionID == row.ParentSessionID
}

func (o *errandTeardownOwner) executionForSession(ctx context.Context, sessionID string) (string, bool, error) {
	id, err := o.svc.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if errors.Is(err, agentruntime.ErrNoExecutionForSession) && id == "" {
		if err := o.confirmRuntimeAbsence(ctx, sessionID); err != nil {
			return "", false, err
		}
		return "", true, nil
	}
	if err != nil {
		return "", false, err
	}
	if strings.TrimSpace(id) == "" {
		return "", false, errors.New("errand runtime lookup returned an empty execution identity")
	}
	return id, false, nil
}

func (o *errandTeardownOwner) confirmRuntimeAbsence(ctx context.Context, sessionID string) error {
	running, err := o.svc.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read persisted errand runtime inventory: %w", err)
	}
	if running == nil {
		return nil
	}
	if o.svc.hasPendingRemoteRecovery(sessionID, running.AgentExecutionID) {
		return errors.New("errand runtime authenticated recovery is pending")
	}
	if claim, ok := o.svc.executionTeardownClaimFor(sessionID, running.AgentExecutionID); ok && claim.cleanupCompleted {
		return nil
	}
	if running.Status == models.ExecutorRunningStatusStopped && running.LocalPID == 0 {
		return nil
	}
	if o.svc.rowLiveness(running) != models.ProcessLivenessDead {
		return errors.New("errand runtime absence is unconfirmed by recovered inventory")
	}
	return nil
}

func (o *errandTeardownOwner) currentTurn(ctx context.Context, store errandTeardownStore, sessionID string) (*models.Turn, error) {
	turnID, err := o.svc.peekActiveTurnID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if turnID != "" {
		return o.svc.turnService.GetTurn(ctx, turnID)
	}
	turn, err := store.GetLatestTurnBySessionID(ctx, sessionID)
	if turn != nil && turn.Metadata[models.TurnMetaKeyLifecycleOnly] == true {
		return nil, errors.New("latest errand turn has only lifecycle authority")
	}
	return turn, err
}

func teardownSubmission(ctx context.Context, store errandTeardownStore, row *models.Errand, session *models.TaskSession, turn *models.Turn) (string, error) {
	submissions, err := store.ListAgentDeliverySubmissions(ctx, row.ID)
	if err != nil {
		return "", err
	}
	selected, err := observedTeardownSubmission(submissions, row, session, turn)
	if err != nil || selected != "" || turn == nil {
		return selected, err
	}
	if marker, ok := turn.Metadata[errandAdmissionTurnMetadataKey].(map[string]interface{}); ok {
		if submissionID, ok := marker["submission_id"].(string); ok && submissionID != "" {
			return submissionID, nil
		}
	}
	if turn.ID == row.FirstTurnID {
		return row.SubmissionID, nil
	}
	return "", nil
}

func observedTeardownSubmission(submissions []*models.AgentDeliverySubmission, row *models.Errand, session *models.TaskSession, turn *models.Turn) (string, error) {
	if turn != nil && !teardownTurnMatches(row, turn) {
		return "", models.ErrInvalidErrand
	}
	var active, terminal string
	for _, submission := range submissions {
		if submission.IncarnationID != session.QueueIncarnationID {
			continue
		}
		if turn == nil || submission.SessionID != session.ID || strings.TrimSpace(submission.ID) == "" {
			return "", errors.New("errand submission ownership is ambiguous")
		}
		isActive, isTerminal, err := teardownSubmissionEvidence(submission, turn)
		if err != nil {
			return "", err
		}
		if isActive {
			if active != "" {
				return "", errors.New("errand active submission ownership is ambiguous")
			}
			active = submission.ID
		}
		if isTerminal {
			if terminal != "" {
				return "", errors.New("errand terminal submission ownership is ambiguous")
			}
			terminal = submission.ID
		}
	}
	if active != "" {
		return active, nil
	}
	return terminal, nil
}

func teardownTurnMatches(row *models.Errand, turn *models.Turn) bool {
	return turn.TaskID == row.TaskID && turn.TaskSessionID == row.ID && strings.TrimSpace(turn.ID) != ""
}

func teardownSubmissionEvidence(submission *models.AgentDeliverySubmission, turn *models.Turn) (active, terminal bool, err error) {
	switch submission.State {
	case models.DeliverySubmissionAccepted, models.DeliverySubmissionDispatching:
		return true, false, nil
	case models.DeliverySubmissionCompleted, models.DeliverySubmissionFailed, models.DeliverySubmissionCancelled:
		if submission.TerminalTurnID == turn.ID {
			return false, true, nil
		}
		if submission.TerminalTurnID == "" && !submission.CreatedAt.Before(turn.StartedAt) {
			return false, false, errors.New("errand terminal submission lacks turn ownership")
		}
		return false, false, nil
	default:
		return false, false, errors.New("errand submission admission remains unresolved")
	}
}

func (o *errandTeardownOwner) retry(ctx context.Context, store errandTeardownStore, intent models.ErrandTeardownIntent) (errands.TeardownOutcome, error) {
	row, err := store.GetErrand(ctx, intent.Target.SessionID)
	if err != nil {
		return "", err
	}
	if row.TeardownIntent == nil || !sameTeardownIntent(*row.TeardownIntent, intent) {
		return "", models.ErrErrandVersionConflict
	}
	if err := intent.ValidateFor(row); err != nil {
		return "", err
	}
	switch row.TeardownIntent.State {
	case models.ErrandTeardownIntentCompleted:
		return errands.TeardownCompleted, nil
	case models.ErrandTeardownIntentSuperseded:
		return errands.TeardownSuperseded, nil
	}
	session, executionID, outcome, err := o.classify(ctx, store, row, intent.Target)
	if err != nil {
		return "", err
	}
	if executionID != "" {
		if err := o.stop(ctx, session, intent.Target, executionID); err != nil {
			return "", err
		}
		outcome = errands.TeardownCompleted
	}
	state := models.ErrandTeardownIntentCompleted
	if outcome == errands.TeardownSuperseded {
		state = models.ErrandTeardownIntentSuperseded
	} else if outcome != errands.TeardownCompleted {
		return "", errors.New("errand teardown has no confirmed outcome")
	}
	_, err = store.TransitionErrandTeardownIntent(ctx, row.ID, intent.ID, row.Version, state)
	if err != nil {
		return "", err
	}
	return outcome, nil
}

func sameTeardownIntent(a, b models.ErrandTeardownIntent) bool {
	return a.ID == b.ID && a.Version == b.Version && a.RequestedAt.Equal(b.RequestedAt) && a.Target == b.Target
}

func (o *errandTeardownOwner) classify(ctx context.Context, store errandTeardownStore, row *models.Errand, target models.ErrandTeardownTarget) (*models.TaskSession, string, errands.TeardownOutcome, error) {
	session, err := o.svc.repo.GetTaskSession(ctx, target.SessionID)
	if errors.Is(err, models.ErrTaskSessionNotFound) {
		return o.classifyMissingSession(ctx, target)
	}
	if err != nil {
		return nil, "", "", err
	}
	if !errandTeardownSessionMatches(row, session) {
		return nil, "", "", models.ErrInvalidErrand
	}
	if target.SessionIncarnation != "" && session.QueueIncarnationID != target.SessionIncarnation {
		return nil, "", errands.TeardownSuperseded, nil
	}
	executionID, absent, err := o.executionForSession(ctx, target.SessionID)
	if err != nil {
		return nil, "", "", err
	}
	if target.FirstTurnID == "" && executionID != target.ExecutionID {
		return o.classifyOldExecution(target)
	}
	return o.classifyObservedPrompt(ctx, store, row, target, session, executionID, absent)
}

func (o *errandTeardownOwner) classifyMissingSession(ctx context.Context, target models.ErrandTeardownTarget) (*models.TaskSession, string, errands.TeardownOutcome, error) {
	if target.FirstTurnID == "" {
		return o.classifyOldExecution(target)
	}
	_, absent, err := o.executionForSession(ctx, target.SessionID)
	if err != nil || !absent {
		return nil, "", "", errors.Join(err, errors.New("reserved launch runtime absence is unconfirmed"))
	}
	return nil, "", errands.TeardownCompleted, nil
}

func (o *errandTeardownOwner) classifyObservedPrompt(ctx context.Context, store errandTeardownStore, row *models.Errand, target models.ErrandTeardownTarget, session *models.TaskSession, executionID string, absent bool) (*models.TaskSession, string, errands.TeardownOutcome, error) {
	turn, err := o.currentTurn(ctx, store, target.SessionID)
	if err != nil {
		return nil, "", "", err
	}
	if turn == nil && (target.TurnID != "" || target.FirstTurnID != "") {
		return nil, "", "", errors.New("captured errand turn evidence is unavailable")
	}
	if teardownTurnChanged(target, turn) {
		return nil, "", errands.TeardownSuperseded, nil
	}
	submissionID, err := teardownSubmission(ctx, store, row, session, turn)
	if err != nil {
		return nil, "", "", err
	}
	wantSubmission := target.SubmissionID
	if target.FirstTurnID != "" {
		wantSubmission = target.ReservedSubmissionID
	}
	if submissionID != wantSubmission {
		if submissionID == "" {
			return nil, "", "", errors.New("captured errand submission evidence is unavailable")
		}
		return nil, "", errands.TeardownSuperseded, nil
	}
	if target.FirstTurnID != "" && !reservedTeardownSubmissionBound(turn, target, row.ParentSessionID) {
		return nil, "", "", errors.New("reserved errand runtime lacks exact initial admission evidence")
	}
	if absent {
		return nil, "", errands.TeardownCompleted, nil
	}
	return session, executionID, "", nil
}

func reservedTeardownSubmissionBound(turn *models.Turn, target models.ErrandTeardownTarget, parentID string) bool {
	if turn == nil {
		return false
	}
	marker, _ := turn.Metadata[errandAdmissionTurnMetadataKey].(map[string]interface{})
	return marker["submission_id"] == target.ReservedSubmissionID &&
		marker["sender_session_id"] == parentID && marker["state"] == string(models.ErrandAdmissionAccepted)
}

func teardownTurnChanged(target models.ErrandTeardownTarget, turn *models.Turn) bool {
	wantTurn := target.TurnID
	if target.FirstTurnID != "" {
		wantTurn = target.FirstTurnID
	}
	if turn == nil {
		return wantTurn != ""
	}
	return target.NoTurn || turn.ID != wantTurn
}

func (o *errandTeardownOwner) classifyOldExecution(target models.ErrandTeardownTarget) (*models.TaskSession, string, errands.TeardownOutcome, error) {
	if target.ExecutionID == "" {
		return nil, "", errands.TeardownSuperseded, nil
	}
	reader, ok := o.svc.agentManager.(interface {
		GetExecution(string) (*agentruntime.AgentExecution, bool)
	})
	if !ok {
		return nil, "", "", errors.New("exact errand runtime inventory is unavailable")
	}
	execution, exists := reader.GetExecution(target.ExecutionID)
	if !exists {
		return nil, "", errands.TeardownCompleted, nil
	}
	if execution == nil || execution.ID != target.ExecutionID || execution.TaskID != target.TaskID ||
		execution.SessionID != target.SessionID || execution.DeliveryIncarnationID != target.SessionIncarnation {
		return nil, "", "", errors.New("captured execution ownership is unverified")
	}
	return nil, target.ExecutionID, "", nil
}

func (o *errandTeardownOwner) stop(ctx context.Context, session *models.TaskSession, target models.ErrandTeardownTarget, executionID string) error {
	sessionID := target.SessionID
	claim, claimed := o.svc.claimExecutionTeardownWithToken(sessionID, executionID, executionTeardownIntentGraceful)
	if !claimed {
		if prior, ok := o.svc.executionTeardownClaimFor(sessionID, executionID); ok && prior.cleanupCompleted {
			return nil
		}
		return errors.New("exact errand execution teardown is already owned")
	}
	key := terminalExecutionKey(sessionID, executionID)
	if session != nil {
		if err := o.cancelSession(ctx, session); err != nil {
			o.svc.executionTeardownClaims.CompareAndDelete(key, claim)
			return err
		}
	}
	turnID := target.TurnID
	if target.FirstTurnID != "" {
		turnID = target.FirstTurnID
	}
	// Prompt generations are process-local; a recovered intent uses its durable
	// turn/submission authority rather than a generation from the former process.
	o.svc.setCancellationIdentity(sessionID, o.operation, cancellationIdentity{executionID: executionID, turnID: turnID})
	o.svc.setCancellationCompletionEligible(sessionID, o.operation, session != nil)
	// The operation marker survives unlock: prompt admission cannot pass while
	// lifecycle events consume their guard during the synchronous provider stop.
	o.unlock()
	stopErr := o.svc.executor.StopExecution(ctx, executionID, errands.StopReasonErrandCancelled, false)
	if stopErr != nil && !agentruntime.IsNotFound(stopErr) {
		o.svc.executionTeardownClaims.CompareAndDelete(key, claim)
		return stopErr
	}
	o.svc.markExecutionFailed(sessionID, executionID)
	o.svc.completeExecutionTeardownClaim(sessionID, executionID, claim)
	if err := o.guard.relockWithContext(ctx); err != nil {
		return err
	}
	return nil
}

func (o *errandTeardownOwner) cancelSession(ctx context.Context, session *models.TaskSession) error {
	if isTerminalSessionState(session.State) {
		return nil
	}
	changed, updatedAt, err := o.svc.repo.UpdateTaskSessionStateIfCurrentIdentity(ctx, session.TaskID,
		session.ID, session.QueueIncarnationID, session.State, models.TaskSessionStateCancelled, errands.StopReasonErrandCancelled)
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("errand session changed before exact teardown: %w", models.ErrErrandVersionConflict)
	}
	after := taskSessionAfterStateWrite(session, models.TaskSessionStateCancelled, errands.StopReasonErrandCancelled, updatedAt)
	o.transition = &errandTeardownTransition{before: session, after: after}
	return nil
}
