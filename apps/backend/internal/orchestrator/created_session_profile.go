package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) resolveCreatedSessionProfile(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	requestedProfileID string,
) (string, error) {
	if requestedProfileID == "" {
		requestedProfileID = session.AgentProfileID
	}
	effectiveProfileID, err := s.resolveEffectiveAgentProfile(ctx, taskID, "", requestedProfileID)
	if err != nil {
		return "", err
	}
	if effectiveProfileID == "" {
		return "", fmt.Errorf("agent_profile_id is required")
	}
	if s.profileExecutionResolver != nil {
		if err := s.profileExecutionResolver.ValidateProfile(ctx, effectiveProfileID); err != nil {
			return "", err
		}
	}
	if err := s.refuseMaterializedRouteProfileChange(ctx, session, effectiveProfileID); err != nil {
		return "", err
	}
	return effectiveProfileID, nil
}
