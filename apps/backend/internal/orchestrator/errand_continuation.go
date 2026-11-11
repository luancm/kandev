package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// ValidateErrandContinuation checks exact decisions before message persistence.
// Turn reservation repeats the check under its transaction's ownership locks.
func (s *Service) ValidateErrandContinuation(ctx context.Context, sessionID string) error {
	if err := s.reconcileErrandTerminalDecisions(ctx, sessionID); err != nil {
		return err
	}
	if guard, ok := s.repo.(interface {
		ValidateErrandContinuation(context.Context, string) error
	}); ok {
		return guard.ValidateErrandContinuation(ctx, sessionID)
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if models.IsErrandSession(session) {
		return fmt.Errorf("errand continuation evidence is unavailable")
	}
	return nil
}

func (s *Service) interruptedErrandNativeResume(ctx context.Context, session *models.TaskSession) (string, error) {
	if !models.IsErrandSession(session) || session.State != models.TaskSessionStateWaitingForInput || session.IsPassthrough {
		return "", nil
	}
	store, ok := s.repo.(interface {
		GetErrand(context.Context, string) (*models.Errand, error)
		GetActiveTurnBySessionID(context.Context, string) (*models.Turn, error)
	})
	if !ok {
		return "", nil
	}
	if err := s.ValidateErrandContinuation(ctx, session.ID); err != nil {
		return "", err
	}
	active, err := store.GetActiveTurnBySessionID(ctx, session.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if active != nil {
		return "", nil
	}
	row, err := store.GetErrand(ctx, session.ID)
	if err != nil {
		return "", err
	}
	if row.Lifecycle != models.ErrandLifecycleLaunched || row.ResultKind != models.ErrandResultInterrupted || row.ResultTurnID == "" || row.LatestSettledTurnID != row.ResultTurnID {
		return "", nil
	}
	return s.retainedErrandNativeIdentity(ctx, session)
}

func (s *Service) retainedErrandNativeIdentity(ctx context.Context, session *models.TaskSession) (string, error) {
	nativeID := continuationNativeID(session)
	if nativeID == "" {
		return "", nil
	}
	generation, err := s.restartRecoveryNativeGeneration(ctx, session)
	if err != nil {
		return "", err
	}
	if generation == nil || generation.NativeSessionID != nativeID {
		return "", nil
	}
	profileID := session.ExecutionProfileID
	if profileID == "" {
		profileID = session.AgentProfileID
	}
	if profileID == "" || s.agentManager == nil {
		return "", nil
	}
	profile, err := s.agentManager.ResolveAgentProfile(ctx, profileID)
	if err != nil {
		return "", err
	}
	if profile == nil || !profile.NativeSessionResume || profile.CLIPassthrough {
		return "", nil
	}
	return nativeID, nil
}

func (s *Service) interruptedErrandResumeOptions(ctx context.Context, session *models.TaskSession, running *models.ExecutorRunning, err error, options executor.ResumeOptions) (executor.ResumeOptions, error) {
	if err != nil && models.IsErrandSession(session) && !errors.Is(err, models.ErrExecutorRunningNotFound) {
		return options, err
	}
	if running == nil && (err == nil || errors.Is(err, models.ErrExecutorRunningNotFound)) {
		nativeID, resumeErr := s.interruptedErrandNativeResume(ctx, session)
		if resumeErr != nil {
			return options, resumeErr
		}
		if nativeID != "" {
			if options.RequiredNativeConversationID != "" && options.RequiredNativeConversationID != nativeID {
				return options, fmt.Errorf("errand native conversation identity changed")
			}
			options.RequiredNativeConversationID = nativeID
		}
	}

	return options, nil
}

// Exact receipts retire only the completed prompt's cached ownership. Recovery
// can settle through storage without a terminal callback to this orchestrator.
func (s *Service) retireSettledErrandPrompt(ctx context.Context, sessionID string) error {
	cached, ok := s.activeTurns.Load(sessionID)
	turnID, _ := cached.(string)
	if !ok || turnID == "" || s.turnService == nil {
		return nil
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if !models.IsErrandSession(session) {
		return nil
	}
	turn, err := s.turnService.GetTurn(ctx, turnID)
	if err != nil {
		return err
	}
	if turn == nil || turn.TaskSessionID != sessionID || turn.CompletedAt == nil {
		return nil
	}
	guard, ok := s.repo.(interface {
		ValidateErrandContinuation(context.Context, string) error
	})
	if !ok {
		return nil
	}
	if err := guard.ValidateErrandContinuation(ctx, sessionID); err != nil {
		return err
	}
	s.activeTurns.CompareAndDelete(sessionID, turnID)
	if accepted := s.acceptedQueuedDispatchForSession(sessionID); accepted != nil && accepted.successorTurnID() == turnID {
		s.acceptedQueuedDispatch.CompareAndDelete(sessionID, accepted)
	}
	return nil
}
