package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

var errErrandParentAdmissionReplay = errors.New("errand parent submission already reserved; reconciliation required")

const errandAdmissionTurnMetadataKey = "errand_parent_admission"

// errandParentPromptUncertainError retains admission evidence across caller
// cleanup. The reservation remains fenced until recovery proves its outcome.
type errandParentPromptUncertainError struct{ err error }

func (e *errandParentPromptUncertainError) Error() string                   { return e.err.Error() }
func (e *errandParentPromptUncertainError) Unwrap() error                   { return e.err }
func (*errandParentPromptUncertainError) ErrandParentPromptUncertain() bool { return true }

type errandParentAdmitter interface {
	ReserveParentAdmission(context.Context, models.ErrandParentAdmission) (models.ErrandAdmissionReservation, error)
	DecideParentAdmission(context.Context, models.ErrandParentAdmission, models.ErrandAdmissionState) error
	ReconcileParentAdmissions(context.Context, string, errands.ParentAdmissionEvidenceReader) error
}

type errandParentDispatch struct {
	svc        *Service
	controller errandParentAdmitter
	identity   models.ErrandParentAdmission
	accepted   atomic.Bool
}

func errandQueuedParentOrigin(sources []messagequeue.QueuedMessage, parentID, submissionID string) errands.ParentPromptOrigin {
	for i := range sources {
		if sources[i].QueuedBy != messagequeue.QueuedByAgent {
			continue
		}
		sender, _ := sources[i].Metadata[errands.ParentPromptSenderMetadataKey].(string)
		if sender != "" && sender == parentID {
			return errands.ParentPromptOrigin{SenderSessionID: sender, SubmissionID: submissionID}
		}
	}
	return errands.ParentPromptOrigin{}
}

func errandPromptOrigin(ctx context.Context, session *models.TaskSession, options promptTaskOptions) errands.ParentPromptOrigin {
	if !models.IsErrandSession(session) {
		return errands.ParentPromptOrigin{}
	}
	if options.errandParentSources != nil {
		return errandQueuedParentOrigin(options.errandParentSources, session.ParentSessionID, options.deliverySubmissionID)
	}
	if options.queuedMessage != nil {
		return errandQueuedParentOrigin([]messagequeue.QueuedMessage{*options.queuedMessage}, session.ParentSessionID, options.deliverySubmissionID)
	}
	if options.errandParentOrigin.SenderSessionID != "" {
		return options.errandParentOrigin
	}
	return errands.ParentPromptOriginFromContext(ctx)
}

func (s *Service) reserveErrandParentDispatch(ctx context.Context, session *models.TaskSession, origin errands.ParentPromptOrigin, turnID, submissionID string, executionIDs ...string) (*errandParentDispatch, error) {
	if !models.IsErrandSession(session) {
		return nil, nil
	}
	if err := s.reconcileErrandTerminalDecisions(ctx, session.ID); err != nil {
		return nil, err
	}
	var turnErr error
	turnID, turnErr = s.errandDispatchTurnID(ctx, session.ID, turnID)
	if turnErr != nil {
		return nil, turnErr
	}

	executionID := session.AgentExecutionID
	if len(executionIDs) > 0 {
		executionID = executionIDs[0]
	}
	if err := s.reserveErrandTerminalIntent(ctx, session, origin, turnID, submissionID, executionID); err != nil {
		return nil, err
	}
	if origin.SenderSessionID == "" || origin.SenderSessionID != session.ParentSessionID {
		return nil, nil
	}
	controller, ok := s.errands.(errandParentAdmitter)
	if !ok {
		return nil, errors.New("errand parent admission service is unavailable")
	}
	if err := controller.ReconcileParentAdmissions(ctx, session.ID, s); err != nil {
		return nil, err
	}
	identity := models.ErrandParentAdmission{TaskID: session.TaskID, ParentSessionID: session.ParentSessionID,
		ErrandID: session.ID, SenderSessionID: origin.SenderSessionID, TurnID: turnID, SubmissionID: normalizeDeliverySubmissionID(submissionID)}
	if turnID == "" || submissionID == "" {
		return nil, errors.New("errand parent dispatch requires stable turn and submission identity")
	}
	reservation, err := controller.ReserveParentAdmission(ctx, identity)
	if err != nil {
		return nil, err
	}
	if !reservation.Created {
		return nil, errErrandParentAdmissionReplay
	}
	return &errandParentDispatch{svc: s, controller: controller, identity: identity}, nil
}

func (d *errandParentDispatch) decide(ctx context.Context, state models.ErrandAdmissionState) error {
	if d == nil {
		return nil
	}
	if state == models.ErrandAdmissionAccepted {
		d.accepted.Store(true)
	}
	decisionCtx, cancel := (promptTaskOptions{}).failureContext(ctx)
	defer cancel()
	// This marker survives a decision-write failure for both delivery protocols.
	var evidenceErr error
	if state != models.ErrandAdmissionUnknown {
		evidenceErr = d.svc.recordErrandAdmissionEvidence(decisionCtx, d.identity, state)
	}
	if err := d.controller.DecideParentAdmission(decisionCtx, d.identity, state); err != nil {
		d.svc.logger.Warn("failed to resolve errand parent admission", zap.String("session_id", d.identity.ErrandID), zap.Error(err))
		return errors.Join(err, evidenceErr)
	}
	return nil
}

func (s *Service) recordErrandAdmissionEvidence(ctx context.Context, a models.ErrandParentAdmission, state models.ErrandAdmissionState) error {
	if s.turnService == nil {
		return errors.New("errand admission turn storage is unavailable")
	}
	turn, err := s.turnService.GetTurn(ctx, a.TurnID)
	if err != nil {
		return err
	}
	if turn == nil || turn.TaskID != a.TaskID || turn.TaskSessionID != a.ErrandID {
		return models.ErrInvalidErrand
	}
	return s.turnService.PatchTurnMetadata(ctx, a.ErrandID, a.TurnID, map[string]interface{}{
		errandAdmissionTurnMetadataKey: map[string]interface{}{"submission_id": a.SubmissionID, "sender_session_id": a.SenderSessionID, "state": string(state)},
	})
}

// ReadErrandParentAdmissionEvidence never interprets local dispatch preparation,
// an abandoned turn, or missing data as a provider acceptance/refusal.
func (s *Service) ReadErrandParentAdmissionEvidence(ctx context.Context, a models.ErrandParentAdmission) (models.ErrandAdmissionState, error) {
	state, err := s.errandParentTurnEvidence(ctx, a)
	if err != nil || state != models.ErrandAdmissionUnknown {
		return state, err
	}
	return s.errandParentSubmissionEvidence(ctx, a), nil
}

func (s *Service) errandParentTurnEvidence(ctx context.Context, a models.ErrandParentAdmission) (models.ErrandAdmissionState, error) {
	if s.turnService == nil {
		return models.ErrandAdmissionUnknown, nil
	}
	turn, err := s.turnService.GetTurn(ctx, a.TurnID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return models.ErrandAdmissionUnknown, err
	}
	if turn == nil || turn.TaskID != a.TaskID || turn.TaskSessionID != a.ErrandID {
		return models.ErrandAdmissionUnknown, nil
	}
	marker, _ := turn.Metadata[errandAdmissionTurnMetadataKey].(map[string]interface{})
	if marker["submission_id"] != a.SubmissionID || marker["sender_session_id"] != a.SenderSessionID {
		return models.ErrandAdmissionUnknown, nil
	}
	state, _ := marker["state"].(string)
	if state == string(models.ErrandAdmissionAccepted) || state == string(models.ErrandAdmissionRefused) {
		return models.ErrandAdmissionState(state), nil
	}
	return models.ErrandAdmissionUnknown, nil
}

func (s *Service) errandParentSubmissionEvidence(ctx context.Context, a models.ErrandParentAdmission) models.ErrandAdmissionState {
	reader, ok := s.repo.(interface {
		GetAgentDeliverySubmissionMetadata(context.Context, string) (*models.AgentDeliverySubmission, error)
	})
	if !ok {
		return models.ErrandAdmissionUnknown
	}
	submission, err := reader.GetAgentDeliverySubmissionMetadata(ctx, a.SubmissionID)
	if err != nil || submission == nil {
		return models.ErrandAdmissionUnknown
	}
	if submission.SessionID == a.ErrandID && submission.TerminalTurnID == a.TurnID && submission.TerminalStreamID != "" &&
		submission.TerminalSequence > 0 && terminalDeliverySubmission(submission.State) {
		return models.ErrandAdmissionAccepted
	}
	return models.ErrandAdmissionUnknown
}

func (s *Service) reconcileErrandParentAdmissions(ctx context.Context) error {
	controller, ok := s.errands.(errandParentAdmitter)
	if !ok {
		return nil
	}
	if err := controller.ReconcileParentAdmissions(ctx, "", s); err != nil {
		return fmt.Errorf("recover errand parent admissions: %w", err)
	}
	return nil
}

func (s *Service) errandDispatchTurnID(ctx context.Context, sessionID, turnID string) (string, error) {
	if turnID != "" || s.turnService == nil {
		return turnID, nil
	}
	turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil || turn == nil {
		return "", err
	}
	return turn.ID, nil
}
