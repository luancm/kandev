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
	var promptAccepted atomic.Bool
	accepted, failed := launchOptions.OnInitialPromptAccepted, launchOptions.OnInitialPromptFailed
	launchOptions.OnInitialPromptAccepted = func(executionID string) {
		promptAccepted.Store(true)
		if err := parentDispatch.Load().decide(context.Background(), models.ErrandAdmissionAccepted); err != nil {
			s.logger.Warn("failed to persist initial errand parent acceptance", zap.Error(err))
		}
		if accepted != nil {
			accepted(executionID)
		}
	}
	launchOptions.OnInitialPromptFailed = func() {
		if !promptAccepted.Load() {
			s.resolveInitialPromptAdmissionFailure(parentDispatch.Load(), delivery.Load(), options.onInitialPromptAdmissionFailed)
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
				if options.onInitialPromptAdmissionFailed != nil {
					options.onInitialPromptAdmissionFailed(err)
				}
				return err
			}
		}
		origin := errandPromptOrigin(admissionCtx, session, deliveryOptions)
		d, reservationErr := s.reserveErrandParentDispatch(admissionCtx, session, origin, turnID, deliveryOptions.deliverySubmissionID, executionID)
		if reservationErr != nil {
			return &agentruntime.PromptAdmissionPersistenceError{Err: reservationErr}
		}
		parentDispatch.Store(d)
		prepared, admissionErr := s.prepareAgentDeliverySubmission(admissionCtx, session, executionID, finalPrompt, finalAttachments, deliveryOptions)
		if admissionErr != nil {
			admissionErr = errors.Join(admissionErr, d.decide(admissionCtx, models.ErrandAdmissionRefused))
			if options.onInitialPromptAdmissionFailed != nil {
				options.onInitialPromptAdmissionFailed(admissionErr)
			}
			return &agentruntime.PromptAdmissionPersistenceError{Err: admissionErr}
		}
		delivery.Store(prepared)
		return nil
	}
}

func (s *Service) resolveInitialPromptAdmissionFailure(parent *errandParentDispatch, delivery *agentDeliverySubmissionRuntime, refused func(error)) {
	ctx, cancel := (promptTaskOptions{}).failureContext(context.Background())
	defer cancel()
	proved, err := delivery.unsentReceipt(ctx)
	if err != nil {
		s.logger.Warn("failed to read initial non-delivery receipt", zap.Error(err))
	}
	state := models.ErrandAdmissionUnknown
	if proved && err == nil {
		state = models.ErrandAdmissionRefused
	}
	if err := parent.decide(ctx, state); err != nil {
		s.logger.Warn("failed to resolve initial errand admission", zap.Error(err))
		return
	}
	if state == models.ErrandAdmissionRefused && refused != nil {
		refused(errors.New("initial prompt was not sent"))
	}
}

func (s *Service) dispatchPreparedPrompt(
	ctx context.Context, taskID string, session *models.TaskSession, executionID, prompt string,
	attachments []v1.MessageAttachment, dispatchOnly bool, options promptTaskOptions, submissionID string,
	validateAdmission func() error, onDispatched func(),
	guard *lockedCancelInFlightGuard,
) (*executor.PromptResult, *agentDeliverySubmissionRuntime, error) {
	var delivery *agentDeliverySubmissionRuntime
	var parentDispatch *errandParentDispatch
	var accepted bool
	var decisionErr error
	beforeAdmission := func(admissionCtx context.Context, finalPrompt string, finalAttachments []v1.MessageAttachment) error {
		if validateAdmission != nil {
			if err := validateAdmission(); err != nil {
				return err
			}
		}
		origin := errandPromptOrigin(ctx, session, options)
		var admissionErr error
		parentDispatch, admissionErr = s.reserveErrandParentDispatch(admissionCtx, session, origin, options.errandAdmissionTurnID, options.deliverySubmissionID, executionID)
		if admissionErr != nil {
			return markPromptAdmissionRejected(admissionErr)
		}
		delivery, admissionErr = s.prepareAgentDeliverySubmission(admissionCtx, session, executionID, finalPrompt, finalAttachments, options)
		if admissionErr != nil {
			return markPromptAdmissionRejected(errors.Join(admissionErr, parentDispatch.decide(admissionCtx, models.ErrandAdmissionRefused)))
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
		accepted = true
		decisionErr = parentDispatch.decide(ctx, models.ErrandAdmissionAccepted)
		if onDispatched != nil {
			onDispatched()
		}
	}
	result, execErr := s.executor.PromptWithAdmissionCallbackAndSubmissionID(
		ctx, taskID, session.ID, prompt, attachments, dispatchOnly,
		beforeAdmission, onAccepted, submissionID,
		s.guardedPassthroughRunning(ctx, session.ID, guard), session,
	)
	if parentDispatch != nil && !accepted && !errors.Is(execErr, errPromptAdmissionRejected) {
		execErr = resolveErrandParentDispatchError(ctx, parentDispatch, delivery, execErr)
	}
	if parentDispatch == nil {
		return result, delivery, execErr
	}
	return result, delivery, wrapAcceptedPromptDispatchFailure(accepted, errors.Join(execErr, decisionErr), nil)
}

func resolveErrandParentDispatchError(ctx context.Context, parent *errandParentDispatch, delivery *agentDeliverySubmissionRuntime, cause error) error {
	var notSent *agentruntime.RequestNotSentError
	if delivery != nil && !errors.Is(cause, agentruntime.ErrUncertainPromptDelivery) && errors.As(cause, &notSent) {
		err := delivery.failUnsent(context.WithoutCancel(ctx))
		if err == nil {
			err = parent.decide(ctx, models.ErrandAdmissionRefused)
			if err == nil {
				return markPromptAdmissionRejected(cause)
			}
		}
		cause = errors.Join(cause, err)
	}
	if cause == nil {
		cause = errors.New("errand parent prompt acceptance was not observed")
	}
	return &errandParentPromptUncertainError{err: errors.Join(cause, agentruntime.ErrUncertainPromptDelivery,
		parent.decide(ctx, models.ErrandAdmissionUnknown))}
}
