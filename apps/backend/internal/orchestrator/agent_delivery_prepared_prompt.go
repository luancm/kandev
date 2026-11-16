package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"

	"go.uber.org/zap"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (s *Service) configureInitialDeliveryAdmission(session *models.TaskSession, options startCreatedSessionOptions, launchOptions *executor.LaunchOptions) {
	launchOptions.DeliverySubmissionID = options.deliverySubmissionID
	deliveryOptions := promptTaskOptions{
		deliveryProtocol: options.deliveryProtocol, deliverySubmissionID: options.deliverySubmissionID,
		deliveryPayloadHash: options.deliveryPayloadHash, deliveryClaimUpdater: options.deliveryClaimUpdater,
		errandParentOrigin: options.errandParentOrigin, errandParentSources: options.errandParentSources,
	}
	if deliveryOptions.deliverySubmissionID == "" {
		deliveryOptions.deliverySubmissionID = options.errandParentOrigin.SubmissionID
	}
	launchOptions.DeliverySubmissionID = deliveryOptions.deliverySubmissionID
	var parentDispatch atomic.Pointer[errandParentDispatch]
	var delivery atomic.Pointer[agentDeliverySubmissionRuntime]
	outcome := options.initialPromptOutcome
	if outcome == nil {
		outcome = &promptDispatchOutcome{}
	}
	accepted, failed := launchOptions.OnInitialPromptAccepted, launchOptions.OnInitialPromptFailed
	launchOptions.OnInitialPromptAccepted = func(executionID string) {
		outcome.recordAccepted(nil)
		if err := parentDispatch.Load().decide(context.Background(), models.ErrandAdmissionAccepted); err != nil {
			outcome.recordAdmissionError(err)
			s.logger.Warn("failed to persist initial errand parent acceptance", zap.Error(err))
		}
		if accepted != nil {
			accepted(executionID)
		}
	}
	launchOptions.OnInitialPromptFailed = func() {
		if !outcome.snapshot().accepted() {
			s.resolveInitialPromptAdmissionFailure(parentDispatch.Load(), delivery.Load(), outcome, options.onInitialPromptAdmissionFailed)
		}
		if failed != nil {
			failed()
		}
	}
	turnID := launchOptions.TurnID
	// Executor binds these options before starting the goroutine that admits the prompt.
	launchOptions.BeforeAgentStart = func(admissionCtx context.Context, executionID, _ string) error {
		var bindErr error
		deliveryOptions, bindErr = s.bindInitialAgentDeliverySubmission(admissionCtx, session, executionID, deliveryOptions)
		return bindErr
	}
	validateAdmission := launchOptions.BeforeInitialPromptDispatch
	launchOptions.BeforeInitialPromptDispatch = func(admissionCtx context.Context, executionID, finalPrompt string, finalAttachments []v1.MessageAttachment) error {
		if validateAdmission != nil {
			if err := validateAdmission(admissionCtx, executionID, finalPrompt, finalAttachments); err != nil {
				outcome.recordRejection(nil)
				if options.onInitialPromptAdmissionFailed != nil {
					options.onInitialPromptAdmissionFailed(err)
				}
				return err
			}
		}
		origin := errandPromptOrigin(admissionCtx, session, deliveryOptions)
		d, reservationErr := s.reserveErrandParentDispatch(admissionCtx, session, origin, turnID, deliveryOptions.deliverySubmissionID, executionID)
		if reservationErr != nil {
			outcome.recordUnresolved(reservationErr)
			return &agentruntime.PromptAdmissionPersistenceError{Err: reservationErr}
		}
		parentDispatch.Store(d)
		prepared, admissionErr := s.prepareAgentDeliverySubmission(admissionCtx, session, executionID, finalPrompt, finalAttachments, deliveryOptions)
		if admissionErr != nil {
			outcome.recordRejection(d.decide(admissionCtx, models.ErrandAdmissionRefused))
			admissionErr = outcome.snapshot().dispatchError(admissionErr)
			if options.onInitialPromptAdmissionFailed != nil {
				options.onInitialPromptAdmissionFailed(admissionErr)
			}
			return &agentruntime.PromptAdmissionPersistenceError{Err: admissionErr}
		}
		delivery.Store(prepared)
		return nil
	}
}

func (s *Service) resolveInitialPromptAdmissionFailure(
	parent *errandParentDispatch, delivery *agentDeliverySubmissionRuntime,
	outcome *promptDispatchOutcome, refused func(error),
) {
	if outcome.snapshot().evidence == promptDispatchRejected {
		ctx, cancel := (promptTaskOptions{}).failureContext(context.Background())
		defer cancel()
		if err := parent.decide(ctx, models.ErrandAdmissionUnknown); err != nil {
			outcome.recordAdmissionError(err)
			s.logger.Warn("failed to retain rejected initial errand admission state", zap.Error(err))
		}
		return
	}
	ctx, cancel := (promptTaskOptions{}).failureContext(context.Background())
	defer cancel()
	proved, err := delivery.unsentReceipt(ctx)
	if err != nil {
		s.logger.Warn("failed to read initial non-delivery receipt", zap.Error(err))
	}
	if proved && err == nil {
		outcome.recordNonDelivery(parent.decide(ctx, models.ErrandAdmissionRefused))
	} else {
		outcome.recordUnresolved(parent.decide(ctx, models.ErrandAdmissionUnknown))
	}
	resolved := outcome.snapshot()
	if resolved.admissionErr != nil {
		s.logger.Warn("failed to resolve initial errand admission", zap.Error(resolved.admissionErr))
		return
	}
	if resolved.refused() && refused != nil {
		refused(errors.New("initial prompt was not sent"))
	}
}

func (s *Service) dispatchPreparedPrompt(
	ctx context.Context, taskID string, session *models.TaskSession, executionID, prompt string,
	attachments []v1.MessageAttachment, dispatchOnly bool, options promptTaskOptions, submissionID string,
	validateAdmission func() error, onDispatched func(), protocol string,
	guard *lockedCancelInFlightGuard, outcome *promptDispatchOutcome,
) (*executor.PromptResult, *agentDeliverySubmissionRuntime, error) {
	var delivery *agentDeliverySubmissionRuntime
	var parentDispatch *errandParentDispatch
	beforeAdmission := func(admissionCtx context.Context, finalPrompt string, finalAttachments []v1.MessageAttachment) error {
		if validateAdmission != nil {
			if err := validateAdmission(); err != nil {
				outcome.recordRejection(nil)
				return err
			}
		}
		origin := errandPromptOrigin(ctx, session, options)
		var admissionErr error
		parentDispatch, admissionErr = s.reserveErrandParentDispatch(admissionCtx, session, origin, options.errandAdmissionTurnID, options.deliverySubmissionID, executionID)
		if admissionErr != nil {
			outcome.recordRejection(nil)
			return admissionErr
		}
		delivery, admissionErr = s.prepareAgentDeliverySubmission(admissionCtx, session, executionID, finalPrompt, finalAttachments, options)
		if admissionErr != nil {
			outcome.recordRejection(parentDispatch.decide(admissionCtx, models.ErrandAdmissionRefused))
			return admissionErr
		}
		if delivery != nil {
			delivery.dispatchOnly = dispatchOnly
		}
		return nil
	}
	origin := errandPromptOrigin(ctx, session, options)
	if !models.IsErrandSession(session) && origin.SenderSessionID == "" && options.deliveryProtocol == messagequeue.DeliveryProtocolLegacy && validateAdmission == nil && options.deliveryClaimUpdater == nil {
		beforeAdmission = nil
	}
	onAccepted := func() {
		outcome.recordAccepted(nil)
		outcome.recordAdmissionError(parentDispatch.decide(ctx, models.ErrandAdmissionAccepted))
		if onDispatched != nil {
			onDispatched()
		}
	}
	result, execErr := s.executor.PromptWithAdmissionCallbackAndSubmissionID(
		ctx, taskID, session.ID, prompt, attachments, dispatchOnly,
		beforeAdmission, onAccepted, submissionID,
		s.guardedPassthroughRunning(ctx, session.ID, guard), session,
	)
	execErr = s.resolvePromptDispatchOutcome(ctx, session, parentDispatch, delivery, outcome, protocol, execErr)
	return result, delivery, outcome.snapshot().dispatchError(execErr)
}

func (s *Service) resolvePromptDispatchOutcome(
	ctx context.Context, session *models.TaskSession, parent *errandParentDispatch,
	delivery *agentDeliverySubmissionRuntime, outcome *promptDispatchOutcome, protocol string, cause error,
) error {
	observed := outcome.snapshot()
	if observed.accepted() || observed.refused() || (cause == nil && parent == nil) {
		return cause
	}
	if observed.evidence == promptDispatchPending {
		cause = s.resolvePendingPromptDispatch(ctx, parent, delivery, outcome, cause)
		if outcome.snapshot().refused() {
			return cause
		}
	}
	// Ordinary non-durable failures retain their restoration policy without
	// claiming proof of non-delivery. Errand and durable attempts cannot use it.
	if ordinaryPromptFailureRollbackEligible(session, parent, delivery, protocol, cause) {
		outcome.allowOrdinaryFailureRollback()
	}
	if parent == nil {
		return cause
	}
	return s.resolveUncertainPromptParentAdmission(ctx, parent, outcome, cause)
}

func (s *Service) resolvePendingPromptDispatch(
	ctx context.Context, parent *errandParentDispatch, delivery *agentDeliverySubmissionRuntime,
	outcome *promptDispatchOutcome, cause error,
) error {
	var notSent *agentruntime.RequestNotSentError
	if delivery == nil || errors.Is(cause, agentruntime.ErrUncertainPromptDelivery) || !errors.As(cause, &notSent) {
		outcome.recordUnresolved(nil)
		return cause
	}
	if err := delivery.failUnsent(context.WithoutCancel(ctx)); err != nil {
		outcome.recordUnresolved(nil)
		return errors.Join(cause, err)
	}
	outcome.recordNonDelivery(parent.decide(ctx, models.ErrandAdmissionRefused))
	return cause
}

func ordinaryPromptFailureRollbackEligible(
	session *models.TaskSession, parent *errandParentDispatch, delivery *agentDeliverySubmissionRuntime,
	protocol string, cause error,
) bool {
	return delivery == nil && parent == nil && !models.IsErrandSession(session) &&
		protocol == messagequeue.DeliveryProtocolLegacy && !errors.Is(cause, agentruntime.ErrUncertainPromptDelivery)
}

func (s *Service) resolveUncertainPromptParentAdmission(
	ctx context.Context, parent *errandParentDispatch, outcome *promptDispatchOutcome, cause error,
) error {
	outcome.recordAdmissionError(parent.decide(ctx, models.ErrandAdmissionUnknown))
	if cause == nil {
		cause = errors.New("errand parent prompt acceptance was not observed")
	}
	return &errandParentPromptUncertainError{err: errors.Join(cause, agentruntime.ErrUncertainPromptDelivery)}
}
