package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
)

type movePreflightRepoErrorStub struct {
	*sqliterepo.Repository
	activeErr error
}

func (r *movePreflightRepoErrorStub) ListActiveTaskSessionsByTaskID(context.Context, string) ([]*models.TaskSession, error) {
	if r.activeErr != nil {
		return nil, r.activeErr
	}
	return nil, nil
}

func TestValidateWorkflowMoveEntryOptionsDoesNotClassifyInactiveHistoryAsPassthrough(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-inactive-history", "session-inactive-history", models.TaskSessionStateCompleted)
	history, err := repo.GetTaskSession(ctx, "session-inactive-history")
	if err != nil {
		t.Fatalf("load historical session: %v", err)
	}
	history.AgentProfileID = "profile-old"
	history.IsPassthrough = true
	if err := repo.UpdateTaskSession(ctx, history); err != nil {
		t.Fatalf("update historical session: %v", err)
	}
	task, err := repo.GetTask(ctx, history.TaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{
		resolveProfileInfo: &executor.AgentProfileInfo{ProfileID: "profile-target", AgentName: "agent-target"},
	})
	if err := svc.ValidateWorkflowMoveEntryOptions(ctx, task, &wfmodels.WorkflowStep{
		ID: "target-step", WorkflowID: task.WorkflowID, AgentProfileID: "profile-target",
	}, &workflowmove.EntryOptions{AgentProfileID: "profile-target"}); err != nil {
		t.Fatalf("inactive passthrough history must not reject a new destination profile: %v", err)
	}
}

func TestValidateWorkflowMoveEntryOptionsPropagatesRepositoryLookupErrors(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedTaskAndSession(t, baseRepo, "task-preflight-repo-error", "session-preflight-repo-error", models.TaskSessionStateWaitingForInput)
	task, err := baseRepo.GetTask(ctx, "task-preflight-repo-error")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	activeErr := errors.New("active session lookup failed")
	svc := &Service{
		repo:         &movePreflightRepoErrorStub{Repository: baseRepo, activeErr: activeErr},
		logger:       testLogger(),
		agentManager: &mockAgentManager{},
	}
	if err := svc.ValidateWorkflowMoveEntryOptions(ctx, task, &wfmodels.WorkflowStep{AgentProfileID: "profile-target"}, &workflowmove.EntryOptions{AgentProfileID: "profile-target"}); !errors.Is(err, activeErr) {
		t.Fatalf("active-session lookup error = %v, want %v", err, activeErr)
	}
}

func TestValidateWorkflowMoveEntryOptionsRejectsUnsupportedOfficeAndPassthroughOverrides(t *testing.T) {
	ctx := context.Background()

	t.Run("Office instructions are rejected before move admission", func(t *testing.T) {
		svc := &Service{logger: testLogger()}
		err := svc.ValidateWorkflowMoveEntryOptions(ctx, &models.Task{ID: "office-task", IsFromOffice: true}, &wfmodels.WorkflowStep{ID: "qa"}, &workflowmove.EntryOptions{Instructions: "handoff"})
		if err == nil {
			t.Fatal("Office entry options accepted even though the Office move path cannot apply them")
		}
	})

	t.Run("Office profile override is classified as unavailable", func(t *testing.T) {
		svc := &Service{logger: testLogger()}
		err := svc.ValidateWorkflowMoveEntryOptions(ctx, &models.Task{ID: "office-task", IsFromOffice: true, AssigneeAgentProfileID: "office-profile"}, &wfmodels.WorkflowStep{ID: "qa"}, &workflowmove.EntryOptions{AgentProfileID: "other-profile"})
		if !errors.Is(err, workflowmove.ErrProfileUnavailable) {
			t.Fatalf("Office profile error = %v, want ErrProfileUnavailable", err)
		}
	})

	t.Run("passthrough profile switch is rejected instead of ignored", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedTaskAndSession(t, repo, "passthrough-task", "passthrough-session", models.TaskSessionStateWaitingForInput)
		session, err := repo.GetTaskSession(ctx, "passthrough-session")
		if err != nil {
			t.Fatalf("load passthrough session: %v", err)
		}
		session.AgentProfileID = "profile-passthrough"
		session.IsPassthrough = true
		session.IsPrimary = true
		if err := repo.UpdateTaskSession(ctx, session); err != nil {
			t.Fatalf("update passthrough session: %v", err)
		}
		task, err := repo.GetTask(ctx, session.TaskID)
		if err != nil {
			t.Fatalf("load task: %v", err)
		}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{
			resolveProfileInfo: &executor.AgentProfileInfo{ProfileID: "profile-other", AgentID: "agent-other"},
		})
		err = svc.ValidateWorkflowMoveEntryOptions(ctx, task, &wfmodels.WorkflowStep{ID: "qa"}, &workflowmove.EntryOptions{AgentProfileID: "profile-other"})
		if err == nil {
			t.Fatal("passthrough profile override accepted even though entry keeps the source PTY")
		}
	})
}

func TestValidateWorkflowMoveEntryOptionsAcceptsPassthroughResetAndInstructions(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "passthrough-options-task", "passthrough-options-session", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, "passthrough-options-session")
	if err != nil {
		t.Fatalf("load passthrough session: %v", err)
	}
	session.AgentProfileID = "profile-passthrough"
	session.IsPassthrough = true
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update passthrough session: %v", err)
	}
	task, err := repo.GetTask(ctx, session.TaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	step := &wfmodels.WorkflowStep{ID: "passthrough-step", WorkflowID: task.WorkflowID, AgentProfileID: "profile-target"}
	if err := svc.ValidateWorkflowMoveEntryOptions(ctx, task, step, &workflowmove.EntryOptions{ResetContext: true, Instructions: "handoff"}); err != nil {
		t.Fatalf("passthrough supported options rejected: %v", err)
	}
}
