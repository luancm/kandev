package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) validateCreatedErrandProfile(ctx context.Context, session *models.TaskSession) (string, error) {
	if !models.IsErrandSession(session) {
		return "", errors.New("created errand profile requires an errand session")
	}
	profileID := session.AgentProfileID
	if profileID == "" {
		return "", fmt.Errorf("agent_profile_id is required for errand session %s", session.ID)
	}
	if s.profileExecutionResolver != nil {
		if err := s.profileExecutionResolver.ValidateProfile(ctx, profileID); err != nil {
			return "", err
		}
	}
	return profileID, nil
}

func (s *Service) resolveCreatedSessionProfile(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	requestedProfileID string,
) (string, error) {
	if models.IsErrandSession(session) {
		return s.validateCreatedErrandProfile(ctx, session)
	}
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
