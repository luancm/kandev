package mcpconfig

import (
	"context"
	"slices"
)

// InheritedDefinitionIDs reads the sources eligible for session exclusions.
func (s *SelectionService) InheritedDefinitionIDs(ctx context.Context, workspaceID, sessionID string) ([]string, error) {
	if s.resolutionContext == nil {
		return nil, ErrMCPInvalidSelection
	}
	rc, err := s.resolutionContext(ctx, workspaceID, sessionID)
	if err != nil {
		return nil, err
	}
	if rc.WorkspaceID != workspaceID || rc.SessionID != sessionID {
		return nil, ErrMCPInvalidSelection
	}
	origins := []SelectionOrigin{
		{Scope: SelectionScopeProfile, WorkspaceID: workspaceID, OwnerID: rc.ProfileID},
		{Scope: SelectionScopeTask, WorkspaceID: workspaceID, OwnerID: rc.TaskID},
	}
	for _, id := range rc.RepositoryIDs {
		origins = append(origins, SelectionOrigin{Scope: SelectionScopeRepository, WorkspaceID: workspaceID, OwnerID: id})
	}
	var inherited []string
	for _, origin := range origins {
		if origin.OwnerID == "" {
			continue
		}
		ids, err := s.List(ctx, origin.Scope, workspaceID, origin.OwnerID)
		if err != nil {
			return nil, err
		}
		inherited = append(inherited, ids...)
	}
	slices.Sort(inherited)
	return slices.Compact(inherited), nil
}
