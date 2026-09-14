package backendapp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

type gitOperationFeedbackHydrationFixture struct {
	harness   bootStateTestHarness
	taskID    string
	sessionID string
	messageID string
}

func newGitOperationFeedbackHydrationFixture(t *testing.T) gitOperationFeedbackHydrationFixture {
	t.Helper()
	const (
		workspaceID   = "git-feedback-hydration-workspace"
		taskID        = "git-feedback-hydration-task"
		environmentID = "git-feedback-hydration-environment"
		workspacePath = "/tmp/git-feedback-hydration-workspace"
		sessionID     = "git-feedback-hydration-session"
		turnID        = "git-feedback-hydration-turn"
		repositoryID  = "repo-a"
		taskRepoID    = "git-feedback-hydration-task-repo"
		messageID     = "git-feedback-hydration-message"
		workflowID    = "git-feedback-hydration-workflow"
		workflowStep  = "git-feedback-hydration-step"
	)

	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	failedAt := time.Now().UTC().Add(-2 * time.Minute)
	if err := harness.taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := harness.taskRepo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: workflowID}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := harness.taskRepo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: workflowStep,
		Title: taskID, Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := harness.taskRepo.CreateRepository(ctx, &models.Repository{
		ID: repositoryID, WorkspaceID: workspaceID, Name: repositoryID, SourceType: "local",
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if err := harness.taskRepo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: taskRepoID, TaskID: taskID, RepositoryID: repositoryID, BaseBranch: "main",
	}); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}
	if err := harness.taskRepo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		WorkspacePath: workspacePath, Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "git-feedback-hydration-environment-repo", RepositoryID: repositoryID,
			WorktreePath: workspacePath, WorktreeBranch: "main", Position: 0,
		}},
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := harness.taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		WorkspacePath: workspacePath, State: models.TaskSessionStateRunning,
		StartedAt: failedAt, UpdatedAt: failedAt,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := harness.taskRepo.CreateTurn(ctx, &models.Turn{
		ID: turnID, TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: failedAt, CreatedAt: failedAt, UpdatedAt: failedAt,
	}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if err := harness.taskRepo.CreateMessage(ctx, &models.Message{
		ID: messageID, TaskSessionID: sessionID, TaskID: taskID, TurnID: turnID,
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeError,
		Content: "Git push failed", CreatedAt: failedAt, UpdatedAt: failedAt,
		Metadata: map[string]interface{}{
			"git_operation_error":                 true,
			"operation":                           "push",
			"git_operation_error_version":         gitOperationFeedbackVersion,
			"git_operation_failed_at":             failedAt.Format(time.RFC3339Nano),
			"git_operation_attempted_remote":      "origin",
			"git_operation_attempted_branch":      "main",
			"git_operation_attempted_head_commit": "abc123",
		},
	}); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	return gitOperationFeedbackHydrationFixture{
		harness: harness, taskID: taskID, sessionID: sessionID, messageID: messageID,
	}
}

func TestAppendLiveGitStatusMessageDoesNotResolveForeignRepositoryError(t *testing.T) {
	fixture := newGitOperationFeedbackHydrationFixture(t)
	ctx := context.Background()
	log := newTestLogger()
	status := client.GitStatusResult{
		Success:          true,
		RepositoryName:   "repo-b",
		Branch:           "main",
		RemoteBranch:     "origin/main",
		HeadCommit:       "abc123",
		RemoteHeadCommit: "abc123",
		RemoteAhead:      0,
		RemoteBehind:     0,
		Timestamp:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	agentClient, closeServer := newGitStatusClientWithHandler(t, log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/git/status/multi" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.MultiRepoGitStatusResult{
			Success: true,
			Repos:   []client.PerRepoGitStatus{{RepositoryName: "repo-b", Status: status}},
		})
	}))
	defer closeServer()

	manager := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	if err := manager.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
		ID: "git-feedback-hydration-execution", TaskID: fixture.taskID, SessionID: fixture.sessionID,
		TaskEnvironmentID: "git-feedback-hydration-environment", WorkspacePath: "/tmp/git-feedback-hydration-workspace",
	}); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	execution, ok := manager.GetExecutionBySessionID(fixture.sessionID)
	if !ok {
		t.Fatal("execution was not registered")
	}
	execution.SetAgentCtlClientForTesting(agentClient)
	session, err := fixture.harness.taskRepo.GetTaskSession(ctx, fixture.sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	msgs := appendLiveGitStatusMessage(ctx, fixture.harness.taskRepo, manager, fixture.sessionID, session, nil, log, fixture.harness.taskSvc)
	if len(msgs) != 1 {
		t.Fatalf("hydration returned %d messages, want one", len(msgs))
	}
	updated, err := fixture.harness.taskRepo.GetMessage(ctx, fixture.messageID)
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if resolved, _ := updated.Metadata["git_operation_resolved"].(bool); resolved {
		t.Fatalf("foreign repository status resolved task card: metadata=%#v", updated.Metadata)
	}
}
