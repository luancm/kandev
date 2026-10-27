package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/errands"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// parentErrandPromptOptions reports whether a session's first-turn context
// advertises the errand tools and which profiles it offers. It reads the same
// MCP profile resolver as the server, so the prompt and the registered tools
// agree. Any failure leaves the guidance off.
func (s *Service) parentErrandPromptOptions(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
) (bool, []sysprompt.ErrandProfile) {
	lister, ok := s.errands.(errandProfileLister)
	if !ok || s.executor == nil || session == nil {
		return false, nil
	}
	profile, err := s.executor.ResolveTaskSessionMCPProfile(ctx, taskID, session, true)
	if err != nil {
		s.warnErrandPrompt("resolve MCP profile for errand guidance", err)
		return false, nil
	}
	if !profile.HasCapability(mcpprofile.CapabilityErrands) {
		return false, nil
	}
	infos, err := lister.CompatibleProfiles(ctx, taskID)
	if err != nil {
		s.warnErrandPrompt("list errand profiles for guidance", err)
		return false, nil
	}
	return true, errandPromptProfiles(infos)
}

// errandProfileLister is the part of the errands service that the parent's
// first-turn guidance needs. It is optional on ErrandController, so controllers
// that only drive lifecycle hooks keep working.
type errandProfileLister interface {
	CompatibleProfiles(ctx context.Context, taskID string) ([]errands.ProfileInfo, error)
}

func errandPromptProfiles(infos []errands.ProfileInfo) []sysprompt.ErrandProfile {
	profiles := make([]sysprompt.ErrandProfile, 0, len(infos))
	for _, info := range infos {
		profiles = append(profiles, sysprompt.ErrandProfile{Name: info.Name, Agent: info.Agent, Model: info.Model})
	}
	return profiles
}

func (s *Service) warnErrandPrompt(action string, err error) {
	if s.logger != nil {
		s.logger.Warn("errand guidance unavailable", zap.String("action", action), zap.Error(err))
	}
}
