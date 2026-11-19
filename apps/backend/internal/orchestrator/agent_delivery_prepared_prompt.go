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
	}
	var delivery atomic.Pointer[agentDeliverySubmissionRuntime]
	outcome := options.initialPromptOutcome
	if outcome == nil {
		outcome = &promptDispatchOutcome{}
	}
	accepted, failed := launchOptions.OnInitialPromptAccepted, launchOptions.OnInitialPromptFailed
	launchOptions.OnInitialPromptAccepted = func(executionID string) {
		outcome.recordAccepted(nil)
		if accepted != nil {
			accepted(executionID)
		}
	}
	launchOptions.OnInitialPromptFailed = func() {
		if !outcome.snapshot().accepted() {
			s.resolveInitialPromptAdmissionFailure(delivery.Load(), outcome, options.onInitialPromptAdmissionFailed)
		}
		if failed != nil {
			failed()
		}
	}
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
				outcome.recordRejection()
				if options.onInitialPromptAdmissionFailed != nil {
					options.onInitialPromptAdmissionFailed(err)
				}
				return err
			}
		}
		prepared, admissionErr := s.prepareAgentDeliverySubmission(admissionCtx, session, executionID, finalPrompt, finalAttachments, deliveryOptions)
		if admissionErr != nil {
			outcome.recordRejection()
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
	delivery *agentDeliverySubmissionRuntime,
	outcome *promptDispatchOutcome, refused func(error),
) {
	if outcome.snapshot().evidence == promptDispatchRejected {
		return
	}
	ctx, cancel := (promptTaskOptions{}).failureContext(context.Background())
	defer cancel()
	proved, err := delivery.unsentReceipt(ctx)
	if err != nil {
		s.logger.Warn("failed to read initial non-delivery receipt", zap.Error(err))
	}
	if proved && err == nil {
		outcome.recordNonDelivery()
	} else {
		outcome.recordUnresolved()
	}
	if outcome.snapshot().refused() && refused != nil {
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
	beforeAdmission := func(admissionCtx context.Context, finalPrompt string, finalAttachments []v1.MessageAttachment) error {
		if validateAdmission != nil {
			if err := validateAdmission(); err != nil {
				outcome.recordRejection()
				return err
			}
		}
		var admissionErr error
		delivery, admissionErr = s.prepareAgentDeliverySubmission(admissionCtx, session, executionID, finalPrompt, finalAttachments, options)
		if admissionErr != nil {
			outcome.recordRejection()
			return admissionErr
		}
		if delivery != nil {
			delivery.dispatchOnly = dispatchOnly
		}
		return nil
	}
	if options.deliveryProtocol == messagequeue.DeliveryProtocolLegacy && validateAdmission == nil && options.deliveryClaimUpdater == nil {
		beforeAdmission = nil
	}
	onAccepted := func() {
		outcome.recordAccepted(nil)
		if onDispatched != nil {
			onDispatched()
		}
	}
	result, execErr := s.executor.PromptWithAdmissionCallbackAndSubmissionID(
		ctx, taskID, session.ID, prompt, attachments, dispatchOnly,
		beforeAdmission, onAccepted, submissionID,
		s.guardedPassthroughRunning(ctx, session.ID, guard), session,
	)
	execErr = s.resolvePromptDispatchOutcome(ctx, delivery, outcome, protocol, execErr)
	return result, delivery, outcome.snapshot().dispatchError(execErr)
}

func (s *Service) resolvePromptDispatchOutcome(
	ctx context.Context,
	delivery *agentDeliverySubmissionRuntime, outcome *promptDispatchOutcome, protocol string, cause error,
) error {
	observed := outcome.snapshot()
	if observed.accepted() || observed.refused() || cause == nil {
		return cause
	}
	if observed.evidence == promptDispatchPending {
		cause = s.resolvePendingPromptDispatch(ctx, delivery, outcome, cause)
		if outcome.snapshot().refused() {
			return cause
		}
	}
	// Non-durable failures can retain ordinary restoration without proof of non-delivery.
	if delivery == nil && protocol == messagequeue.DeliveryProtocolLegacy && !errors.Is(cause, agentruntime.ErrUncertainPromptDelivery) {
		outcome.allowOrdinaryFailureRollback()
	}
	return cause
}

func (s *Service) resolvePendingPromptDispatch(
	ctx context.Context, delivery *agentDeliverySubmissionRuntime,
	outcome *promptDispatchOutcome, cause error,
) error {
	var notSent *agentruntime.RequestNotSentError
	if delivery == nil || errors.Is(cause, agentruntime.ErrUncertainPromptDelivery) || !errors.As(cause, &notSent) {
		outcome.recordUnresolved()
		return cause
	}
	if err := delivery.failUnsent(context.WithoutCancel(ctx)); err != nil {
		outcome.recordUnresolved()
		return errors.Join(cause, err)
	}
	outcome.recordNonDelivery()
	return cause
}
