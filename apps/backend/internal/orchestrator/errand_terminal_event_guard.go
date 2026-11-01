package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

type completedTurnObservationKey struct{}

// A late turn terminal on the retained execution cannot retire its successor.
// A rotated execution still owns its own independent teardown bookkeeping.
func (s *Service) completedEventOwnsObservedTurn(ctx context.Context, data watcher.AgentEventData) bool {
	expectedTurn := data.TurnID
	observedTurn, observed := ctx.Value(completedTurnObservationKey{}).(string)
	if observed {
		expectedTurn = observedTurn
	}
	if (expectedTurn == "" && !observed) || s.turnService == nil {
		return true
	}
	turnID, err := s.peekActiveTurnID(ctx, data.SessionID)
	if err != nil {
		return false
	}
	if turnID == "" {
		return true
	}
	if turnID == expectedTurn {
		if data.PromptGeneration != 0 {
			owner, ok := s.agentManager.(interface {
				OwnsPromptGeneration(string, string, uint64) bool
			})
			return ok && owner.OwnsPromptGeneration(data.SessionID, data.AgentExecutionID, data.PromptGeneration)
		}
		return true
	}
	liveID, err := s.agentManager.GetExecutionIDForSession(ctx, data.SessionID)
	return err == nil && data.AgentExecutionID != "" && liveID != "" && liveID != data.AgentExecutionID
}
