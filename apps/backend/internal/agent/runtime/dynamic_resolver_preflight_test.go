package runtime

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
)

type profilePreflightStore struct {
	settingsstore.Repository
	profiles   map[string]*settingsmodels.AgentProfile
	agents     map[string]*settingsmodels.Agent
	profileErr error
}

func (s *profilePreflightStore) GetAgentProfile(_ context.Context, id string) (*settingsmodels.AgentProfile, error) {
	if s.profileErr != nil {
		return nil, s.profileErr
	}
	profile, ok := s.profiles[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return profile, nil
}

func (s *profilePreflightStore) GetAgent(_ context.Context, id string) (*settingsmodels.Agent, error) {
	return s.agents[id], nil
}

func TestProfileExecutionResolverValidateProfileForNewWorkAllowsEnabledVisibleProfiles(t *testing.T) {
	ctx := context.Background()
	profiles := &profilePreflightStore{
		profiles: map[string]*settingsmodels.AgentProfile{
			"global":    {ID: "global", AgentID: "agent", Enabled: true},
			"workspace": {ID: "workspace", AgentID: "agent", Enabled: true, WorkspaceID: "workspace-1"},
		},
		agents: map[string]*settingsmodels.Agent{
			"agent": {ID: "agent", Name: "codex"},
		},
	}
	resolver := NewProfileExecutionResolver(profiles, nil, true)

	for _, profileID := range []string{"global", "workspace"} {
		if err := resolver.ValidateProfileForNewWork(ctx, profileID, "workspace-1"); err != nil {
			t.Fatalf("ValidateProfileForNewWork(%q) error = %v", profileID, err)
		}
	}
}

func TestProfileExecutionResolverValidateProfileForNewWorkRejectsUnavailableProfiles(t *testing.T) {
	deletedAt := time.Now()
	profiles := &profilePreflightStore{
		profiles: map[string]*settingsmodels.AgentProfile{
			"deleted":  {ID: "deleted", AgentID: "agent", Enabled: true, DeletedAt: &deletedAt},
			"disabled": {ID: "disabled", AgentID: "agent"},
			"foreign":  {ID: "foreign", AgentID: "agent", Enabled: true, WorkspaceID: "workspace-2"},
		},
		agents: map[string]*settingsmodels.Agent{
			"agent": {ID: "agent", Name: "codex"},
		},
	}
	resolver := NewProfileExecutionResolver(profiles, nil, true)

	for _, profileID := range []string{"missing", "deleted", "disabled", "foreign"} {
		err := resolver.ValidateProfileForNewWork(context.Background(), profileID, "workspace-1")
		if !errors.Is(err, ErrProfileUnavailableForNewWork) {
			t.Errorf("ValidateProfileForNewWork(%q) error = %v, want ErrProfileUnavailableForNewWork", profileID, err)
		}
	}
}

func TestProfileExecutionResolverValidateProfilePreservesDisabledSessionContinuation(t *testing.T) {
	profiles := &profilePreflightStore{
		profiles: map[string]*settingsmodels.AgentProfile{
			"disabled": {ID: "disabled", AgentID: "agent", Enabled: false},
		},
		agents: map[string]*settingsmodels.Agent{
			"agent": {ID: "agent", Name: "codex"},
		},
	}
	resolver := NewProfileExecutionResolver(profiles, nil, true)

	if err := resolver.ValidateProfile(context.Background(), "disabled"); err != nil {
		t.Fatalf("ValidateProfile(disabled) error = %v, want existing-session continuation to remain valid", err)
	}
	if err := resolver.ValidateProfileForNewWork(context.Background(), "disabled", "workspace-1"); !errors.Is(err, ErrProfileUnavailableForNewWork) {
		t.Fatalf("ValidateProfileForNewWork(disabled) error = %v, want ErrProfileUnavailableForNewWork", err)
	}
	if err := resolver.ValidateProfileWorkspace(context.Background(), "disabled", "workspace-1"); err != nil {
		t.Fatalf("ValidateProfileWorkspace(disabled) error = %v, want existing-session continuation to remain valid", err)
	}
}

func TestProfileExecutionResolverValidateProfileWorkspaceKeepsDynamicFeatureGate(t *testing.T) {
	profiles := &profilePreflightStore{
		profiles: map[string]*settingsmodels.AgentProfile{
			"dynamic-disabled": {ID: "dynamic-disabled", AgentID: "dynamic-agent", Enabled: false},
		},
		agents: map[string]*settingsmodels.Agent{
			"dynamic-agent": {ID: "dynamic-agent", Name: agents.DynamicAgentID},
		},
	}
	resolver := NewProfileExecutionResolver(profiles, nil, false)

	if err := resolver.ValidateProfileWorkspace(context.Background(), "dynamic-disabled", "workspace-1"); !errors.Is(err, ErrDynamicRoutingDisabled) {
		t.Fatalf("ValidateProfileWorkspace(dynamic-disabled) error = %v, want ErrDynamicRoutingDisabled", err)
	}
}
