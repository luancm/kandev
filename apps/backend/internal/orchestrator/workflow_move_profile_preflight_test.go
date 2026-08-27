package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
)

type workflowMoveProfilePreflightStore struct {
	settingsstore.Repository
	profiles map[string]*settingsmodels.AgentProfile
	agents   map[string]*settingsmodels.Agent
}

func (s *workflowMoveProfilePreflightStore) GetAgentProfile(_ context.Context, id string) (*settingsmodels.AgentProfile, error) {
	profile, ok := s.profiles[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return profile, nil
}

func (s *workflowMoveProfilePreflightStore) GetAgent(_ context.Context, id string) (*settingsmodels.Agent, error) {
	agent, ok := s.agents[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return agent, nil
}

func TestValidateWorkflowMoveEntryOptionsRejectsUnavailableProfilesBeforeModelLookup(t *testing.T) {
	ctx := context.Background()
	deletedAt := time.Now()
	profileStore := &workflowMoveProfilePreflightStore{
		profiles: map[string]*settingsmodels.AgentProfile{
			"disabled": {ID: "disabled", AgentID: "agent", Enabled: false},
			"deleted":  {ID: "deleted", AgentID: "agent", Enabled: true, DeletedAt: &deletedAt},
			"foreign":  {ID: "foreign", AgentID: "agent", Enabled: true, WorkspaceID: "workspace-2"},
		},
		agents: map[string]*settingsmodels.Agent{
			"agent": {ID: "agent", Name: "codex"},
		},
	}
	resolver := agentruntime.NewProfileExecutionResolver(profileStore, nil, true)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-profile-preflight", "session-profile-preflight", models.TaskSessionStateWaitingForInput)
	task, err := repo.GetTask(ctx, "task-profile-preflight")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkspaceID = "workspace-1"
	step := &wfmodels.WorkflowStep{ID: "target-step", WorkflowID: task.WorkflowID, AgentProfileID: "disabled"}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{
		resolveProfileInfo: &executor.AgentProfileInfo{ProfileID: "disabled", AgentID: "agent", AgentName: "codex"},
	})
	svc.SetProfileExecutionResolver(resolver)

	for _, profileID := range []string{"missing", "disabled", "deleted", "foreign"} {
		err := svc.ValidateWorkflowMoveEntryOptions(ctx, task, step, &workflowmove.EntryOptions{
			AgentProfileID: profileID,
		})
		if err == nil {
			t.Errorf("profile %q error = nil, want ErrProfileUnavailable", profileID)
			continue
		}
		if !errors.Is(err, workflowmove.ErrProfileUnavailable) {
			t.Errorf("profile %q error = %v, want ErrProfileUnavailable", profileID, err)
		}
		if strings.Contains(err.Error(), profileID) {
			t.Errorf("profile %q leaked from normalized error %q", profileID, err)
		}
	}
}

func TestValidateWorkflowMoveEntryOptionsAllowsEnabledExplicitProfiles(t *testing.T) {
	ctx := context.Background()
	profileStore := &workflowMoveProfilePreflightStore{
		profiles: map[string]*settingsmodels.AgentProfile{
			"global":    {ID: "global", AgentID: "agent", Enabled: true},
			"workspace": {ID: "workspace", AgentID: "agent", Enabled: true, WorkspaceID: "workspace-1"},
		},
		agents: map[string]*settingsmodels.Agent{
			"agent": {ID: "agent", Name: "codex"},
		},
	}
	resolver := agentruntime.NewProfileExecutionResolver(profileStore, nil, true)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-profile-success", "session-profile-success", models.TaskSessionStateWaitingForInput)
	task, err := repo.GetTask(ctx, "task-profile-success")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkspaceID = "workspace-1"
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{
		resolveProfileInfo: &executor.AgentProfileInfo{ProfileID: "global", AgentID: "agent", AgentName: "codex"},
	})
	svc.SetProfileExecutionResolver(resolver)
	step := &wfmodels.WorkflowStep{ID: "target-step", WorkflowID: task.WorkflowID}
	for _, profileID := range []string{"global", "workspace"} {
		if err := svc.ValidateWorkflowMoveEntryOptions(ctx, task, step, &workflowmove.EntryOptions{AgentProfileID: profileID}); err != nil {
			t.Fatalf("profile %q validation error = %v, want success", profileID, err)
		}
	}
}

func TestValidateWorkflowMoveEntryOptionsAllowsDisabledInheritedProfileForExistingSession(t *testing.T) {
	profileStore := &workflowMoveProfilePreflightStore{
		profiles: map[string]*settingsmodels.AgentProfile{
			"disabled": {ID: "disabled", AgentID: "agent", Enabled: false},
		},
		agents: map[string]*settingsmodels.Agent{
			"agent": {ID: "agent", Name: "codex"},
		},
	}
	resolver := agentruntime.NewProfileExecutionResolver(profileStore, nil, true)
	svc := &Service{
		profileExecutionResolver: resolver,
		agentManager: &mockAgentManager{
			resolveProfileInfo: &executor.AgentProfileInfo{ProfileID: "disabled", AgentID: "agent", AgentName: "codex"},
		},
		logger: testLogger(),
	}
	task := &models.Task{ID: "task-profile-reuse", WorkspaceID: "workspace-1"}
	source := &models.TaskSession{ID: "session-profile-reuse", AgentProfileID: "disabled"}
	if err := svc.validateWorkflowMoveProfile(context.Background(), task, "disabled", false, source); err != nil {
		t.Fatalf("inherited disabled profile validation error = %v, want existing-session reuse to continue", err)
	}
	if err := svc.validateWorkflowMoveProfile(context.Background(), task, "disabled", true, source); !errors.Is(err, workflowmove.ErrProfileUnavailable) {
		t.Fatalf("explicit disabled profile error = %v, want ErrProfileUnavailable", err)
	}
}
