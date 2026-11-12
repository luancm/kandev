package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type queuedTerminalEffectReader interface {
	GetAgentDeliveryEffect(context.Context, string) (*models.AgentDeliveryEffect, error)
}

type queuedTerminalIdentityRepairer interface {
	RepairAgentDeliveryTerminalIdentity(context.Context, *models.AgentDeliveryEffect) (*models.AgentDeliveryEffect, error)
}

func (s *Service) reconcileAgentDeliverySettlements(ctx context.Context, sessionID string) error {
	return s.reconcileAgentDeliverySettlementScope(ctx, sessionID, "")
}

func (s *Service) reconcileAgentDeliverySubmissionSettlements(ctx context.Context, sessionID, submissionID string) error {
	if submissionID == "" {
		return nil
	}
	return s.reconcileAgentDeliverySettlementScope(ctx, sessionID, submissionID)
}

func (s *Service) reconcileAgentDeliverySettlementScope(ctx context.Context, sessionID, submissionID string) error {
	settlements, ok := s.repo.(repository.AgentDeliverySettlementRepository)
	if !ok {
		return nil
	}
	effects, err := settlements.ListPendingAgentDeliverySettlements(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("list pending delivery terminal settlements: %w", err)
	}
	submissions, ok := s.repo.(repository.AgentDeliveryRepository)
	if !ok && len(effects) > 0 {
		return errors.New("agent delivery submission repository is unavailable")
	}
	var identityErr error
	for _, effect := range effects {
		if effect == nil || submissionID != "" && effect.SubmissionID != submissionID {
			continue
		}
		err := s.settleAgentDeliveryEffect(ctx, settlements, submissions, effect)
		if err == nil {
			continue
		}
		if !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) {
			return err
		}
		// Independent submissions can settle while unresolved identities remain
		// visible to session-scoped callers. No refused effect gains authority.
		if sessionID != "" {
			identityErr = errors.Join(identityErr, err)
		}
		s.logger.Warn("retaining delivery terminal with unresolved identity",
			zap.String("effect_key", effect.EffectKey), zap.String("session_id", effect.SessionID),
			zap.Error(err))
	}
	if identityErr != nil {
		return identityErr
	}
	return s.convergeSettledDeliverySessionStates(ctx, settlements, sessionID)
}

func (s *Service) convergeSettledDeliverySessionStates(ctx context.Context, settlements repository.AgentDeliverySettlementRepository, sessionID string) error {
	if sessionID != "" {
		// Completed effects in older installations can still leave a stale
		// coarse row. The current persisted terminal binding is the only
		// candidate, so this never scans historical event payloads.
		return s.convergeSettledDeliverySessionState(ctx, sessionID, nil)
	}
	candidates, err := settlements.ListSettledAgentDeliverySessionStateCandidates(ctx, 200)
	if err != nil {
		return fmt.Errorf("list settled delivery session state candidates: %w", err)
	}
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if err := s.convergeSettledDeliverySessionState(ctx, candidate.SessionID, candidate); err != nil {
			s.logger.Warn("failed to converge settled delivery session state",
				zap.String("session_id", candidate.SessionID), zap.Error(err))
		}
	}
	return nil
}

func (s *Service) settleQueuedDeliveryPredecessor(ctx context.Context, taskID, sessionID string, effect *models.AgentDeliveryEffect) error {
	ctx = withSettledDeliverySessionStateConvergenceSkipped(context.WithoutCancel(ctx))
	effect, err := s.repairQueuedTerminalIdentity(ctx, effect)
	if err != nil {
		return err
	}
	if effect == nil || effect.TurnID == "" {
		return errors.New("terminal identity is unavailable before queued successor")
	}
	// The queue owns the successor, so this terminal intentionally has no workflow transition.
	if err := s.recordNotRequiredDeliveryWorkflowOutcome(ctx, taskID, effect); err != nil {
		return err
	}
	return s.reconcileAgentDeliverySubmissionSettlements(ctx, sessionID, effect.SubmissionID)
}

func (s *Service) repairQueuedTerminalIdentity(
	ctx context.Context,
	effect *models.AgentDeliveryEffect,
) (*models.AgentDeliveryEffect, error) {
	if effect == nil || effect.StreamID == "" || effect.Sequence <= 0 || effect.SubmissionID == "" {
		return effect, nil
	}
	reader, readOK := s.repo.(queuedTerminalEffectReader)
	repairer, repairOK := s.repo.(queuedTerminalIdentityRepairer)
	if !readOK || !repairOK {
		return effect, nil
	}
	key := fmt.Sprintf("agent_delivery.terminal:%s:%d", effect.StreamID, effect.Sequence)
	terminal, err := reader.GetAgentDeliveryEffect(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("load terminal identity before queued successor: %w", err)
	}
	repaired, err := repairer.RepairAgentDeliveryTerminalIdentity(ctx, terminal)
	if err != nil {
		return nil, fmt.Errorf("repair terminal identity before queued successor: %w", err)
	}
	if repaired == nil || repaired.TurnID == "" || effect.TurnID != "" && effect.TurnID != repaired.TurnID {
		return nil, errors.New("terminal identity is unavailable before queued successor")
	}
	workflowEffect := *effect
	workflowEffect.TurnID = repaired.TurnID
	workflowEffect.EffectKey = workflowEffectKey(repaired.TurnID)
	return &workflowEffect, nil
}

func (s *Service) settleAgentDeliveryEffect(
	ctx context.Context,
	settlements repository.AgentDeliverySettlementRepository,
	submissions repository.AgentDeliveryRepository,
	effect *models.AgentDeliveryEffect,
) error {
	outcome := models.DeliverySubmissionState(effect.Outcome)
	if !validTerminalDeliveryOutcome(outcome) {
		return fmt.Errorf("terminal settlement %q has invalid outcome %q", effect.EffectKey, effect.Outcome)
	}
	settled, err := settlements.SettleAgentDeliveryTerminal(
		ctx, effect.StreamID, effect.Sequence, outcome, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("settle delivery terminal %q: %w", effect.EffectKey, err)
	}
	if !settled {
		return nil
	}
	submission, err := submissions.GetAgentDeliverySubmission(ctx, effect.SubmissionID)
	if err != nil {
		return fmt.Errorf("load terminal submission %q: %w", effect.SubmissionID, err)
	}
	if submission.SessionID != effect.SessionID || submission.IncarnationID != effect.IncarnationID ||
		submission.HarnessGeneration != effect.HarnessGeneration {
		return fmt.Errorf("terminal submission %q owner changed before queue settlement", effect.SubmissionID)
	}
	if err := s.acknowledgeTerminalDeliveryClaims(ctx, effect, submission.DispatchAttemptID); err != nil {
		return fmt.Errorf("settle queue claim for terminal submission %q: %w", effect.SubmissionID, err)
	}
	if err := s.clearAgentDeliveryRecoveryNotice(
		ctx, effect.SessionID, effect.IncarnationID, effect.SubmissionID, effect.HarnessGeneration,
	); err != nil {
		return fmt.Errorf("clear settled recovery notice for submission %q: %w", effect.SubmissionID, err)
	}
	if err := s.convergeSettledDeliverySessionState(
		ctx, effect.SessionID, deliverySessionStateRequestFromEffect(effect),
	); err != nil {
		return fmt.Errorf("converge settled session state for submission %q: %w", effect.SubmissionID, err)
	}
	s.publishAgentDeliveryRecoveryState(ctx, effect.SessionID)
	_, err = settlements.CompleteAgentDeliveryTerminalSettlement(ctx, effect.EffectKey, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("complete delivery terminal outbox %q: %w", effect.EffectKey, err)
	}
	return nil
}

func validTerminalDeliveryOutcome(outcome models.DeliverySubmissionState) bool {
	return outcome == models.DeliverySubmissionCompleted || outcome == models.DeliverySubmissionFailed ||
		outcome == models.DeliverySubmissionCancelled
}

func (s *Service) acknowledgeTerminalDeliveryClaims(
	ctx context.Context,
	effect *models.AgentDeliveryEffect,
	dispatchAttemptID string,
) error {
	if s.messageQueue == nil || dispatchAttemptID == "" {
		return nil
	}
	queueChanged, err := s.acknowledgeTerminalQueueDispatches(ctx, effect.SessionID, dispatchAttemptID)
	if err != nil {
		return err
	}
	sendNowChanged, err := s.acknowledgeTerminalSendNowClaims(ctx, effect.SessionID, dispatchAttemptID)
	if err != nil {
		return err
	}
	if queueChanged || sendNowChanged {
		s.PublishQueueStatusEvent(ctx, effect.SessionID)
	}
	return nil
}

func (s *Service) acknowledgeTerminalQueueDispatches(
	ctx context.Context,
	sessionID, dispatchAttemptID string,
) (bool, error) {
	if !s.messageQueue.PendingQueueDispatchPersistenceAvailable() {
		return false, nil
	}
	pending, err := s.messageQueue.ListPendingQueueDispatches(ctx)
	if err != nil {
		return false, err
	}
	changed := false
	for i := range pending {
		claim := &pending[i]
		if !matchesTerminalQueueDispatch(claim.Message, sessionID, dispatchAttemptID) {
			continue
		}
		if err := s.messageQueue.AcknowledgeDurablePendingQueueDispatch(ctx, &claim.Message); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

func matchesTerminalQueueDispatch(message messagequeue.QueuedMessage, sessionID, dispatchAttemptID string) bool {
	_, submissionID, _ := message.DeliverySubmission()
	return message.SessionID == sessionID && submissionID == dispatchAttemptID
}

func (s *Service) acknowledgeTerminalSendNowClaims(
	ctx context.Context,
	sessionID, dispatchAttemptID string,
) (bool, error) {
	if !s.messageQueue.PendingSendNowClaimPersistenceAvailable() {
		return false, nil
	}
	pending, err := s.messageQueue.ListPendingSendNowClaims(ctx)
	if err != nil {
		return false, err
	}
	changed := false
	for i := range pending {
		claim := &pending[i].Claim
		if claim.Identity.SessionID != sessionID || claim.DeliverySubmissionID != dispatchAttemptID {
			continue
		}
		if err := s.messageQueue.AcknowledgeSendNowClaim(ctx, claim); err != nil {
			return false, err
		}
		changed = true
	}
	if changed {
		return true, nil
	}
	return false, nil
}
