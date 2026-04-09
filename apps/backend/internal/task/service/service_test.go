package service

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// MockEventBus implements bus.EventBus for testing
type MockEventBus struct {
	mu              sync.Mutex
	publishedEvents []*bus.Event
	closed          bool
}

func NewMockEventBus() *MockEventBus {
	return &MockEventBus{
		publishedEvents: make([]*bus.Event, 0),
	}
}

func (m *MockEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedEvents = append(m.publishedEvents, event)
	return nil
}

func (m *MockEventBus) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBus) QueueSubscribe(subject, queue string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBus) Request(ctx context.Context, subject string, event *bus.Event, timeout time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (m *MockEventBus) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
}

func (m *MockEventBus) IsConnected() bool {
	return !m.closed
}

func (m *MockEventBus) GetPublishedEvents() []*bus.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.publishedEvents
}

func (m *MockEventBus) ClearEvents() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedEvents = make([]*bus.Event, 0)
}

func createTestService(t *testing.T) (*Service, *MockEventBus, *sqliterepo.Repository) {
	t.Helper()
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("failed to create test repository: %v", err)
	}
	if _, err := worktree.NewSQLiteStore(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("failed to init worktree store: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlxDB.Close(); err != nil {
			t.Errorf("failed to close sqlite db: %v", err)
		}
		if err := cleanup(); err != nil {
			t.Errorf("failed to close repo: %v", err)
		}
	})
	eventBus := NewMockEventBus()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := NewService(Repos{
		Workspaces:       repo,
		Tasks:            repo,
		TaskRepos:        repo,
		Workflows:        repo,
		Messages:         repo,
		Turns:            repo,
		Sessions:         repo,
		GitSnapshots:     repo,
		RepoEntities:     repo,
		Executors:        repo,
		Environments:     repo,
		TaskEnvironments: repo,
		Reviews:          repo,
	}, eventBus, log, RepositoryDiscoveryConfig{})
	return svc, eventBus, repo
}

// Task tests

func TestService_CreateTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	// Create workflow first
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	workflow := &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	repository := &models.Repository{ID: "repo-123", WorkspaceID: "ws-1", Name: "Test Repo"}
	_ = repo.CreateRepository(ctx, repository)

	req := &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-123",
		WorkflowStepID: "step-123",
		Title:          "Test Task",
		Description:    "A test task",
		Priority:       5,
		Repositories: []TaskRepositoryInput{
			{
				RepositoryID: "repo-123",
				BaseBranch:   "main",
			},
		},
	}

	task, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	if task.ID == "" {
		t.Error("expected task ID to be set")
	}
	if task.Title != "Test Task" {
		t.Errorf("expected title 'Test Task', got %s", task.Title)
	}
	if task.State != v1.TaskStateCreated {
		t.Errorf("expected state CREATED, got %s", task.State)
	}

	// Check event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "task.created" {
		t.Errorf("expected event type 'task.created', got %s", events[0].Type)
	}
}

func TestService_CreateRepository_DefaultWorktreeBranchPrefix(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})

	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID: "ws-1",
		Name:        "Test Repo",
	})
	if err != nil {
		t.Fatalf("CreateRepository failed: %v", err)
	}
	if created.WorktreeBranchPrefix != worktree.DefaultBranchPrefix {
		t.Fatalf("expected default prefix %q, got %q", worktree.DefaultBranchPrefix, created.WorktreeBranchPrefix)
	}
	if !created.PullBeforeWorktree {
		t.Fatalf("expected pull_before_worktree to default to true")
	}

	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository failed: %v", err)
	}
	if stored.WorktreeBranchPrefix != worktree.DefaultBranchPrefix {
		t.Fatalf("expected stored prefix %q, got %q", worktree.DefaultBranchPrefix, stored.WorktreeBranchPrefix)
	}
	if !stored.PullBeforeWorktree {
		t.Fatalf("expected stored pull_before_worktree to default to true")
	}
}

func TestService_CreateRepository_PullBeforeWorktreeFalse(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})

	pullFalse := false
	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:        "ws-1",
		Name:               "Test Repo",
		PullBeforeWorktree: &pullFalse,
	})
	if err != nil {
		t.Fatalf("CreateRepository failed: %v", err)
	}
	if created.PullBeforeWorktree {
		t.Fatalf("expected pull_before_worktree to be false when explicitly set")
	}

	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository failed: %v", err)
	}
	if stored.PullBeforeWorktree {
		t.Fatalf("expected stored pull_before_worktree to be false")
	}
}

func TestService_GetTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	// Create required entities
	setupTestTask(t, repo)

	retrieved, err := svc.GetTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if retrieved.Title != "Test Task" {
		t.Errorf("expected title 'Test Task', got %s", retrieved.Title)
	}
}

func TestService_GetTaskNotFound(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()

	_, err := svc.GetTask(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestService_UpdateTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Original"})
	eventBus.ClearEvents()

	newTitle := "Updated Title"
	req := &UpdateTaskRequest{Title: &newTitle}

	updated, err := svc.UpdateTask(ctx, "task-123", req)
	if err != nil {
		t.Fatalf("failed to update task: %v", err)
	}
	if updated.Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %s", updated.Title)
	}

	// Check event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "task.updated" {
		t.Errorf("expected event type 'task.updated', got %s", events[0].Type)
	}
}

// mockWorkflowStepGetter implements WorkflowStepGetter for testing.
type mockWorkflowStepGetter struct {
	steps map[string]*wfmodels.WorkflowStep
}

func (m *mockWorkflowStepGetter) GetStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	if s, ok := m.steps[stepID]; ok {
		return s, nil
	}
	return nil, fmt.Errorf("step not found: %s", stepID)
}

func (m *mockWorkflowStepGetter) GetNextStepByPosition(_ context.Context, _ string, _ int) (*wfmodels.WorkflowStep, error) {
	return nil, nil
}

// mockStartStepResolver implements StartStepResolver for testing.
type mockStartStepResolver struct {
	startSteps map[string]string // workflowID -> stepID
}

func (m *mockStartStepResolver) ResolveStartStep(_ context.Context, workflowID string) (string, error) {
	if id, ok := m.startSteps[workflowID]; ok {
		return id, nil
	}
	return "", fmt.Errorf("no start step for workflow: %s", workflowID)
}

func (m *mockStartStepResolver) ResolveFirstStep(_ context.Context, workflowID string) (string, error) {
	return m.ResolveStartStep(context.Background(), workflowID)
}

func TestService_UnarchiveTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})

	task := &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test"}
	_ = repo.CreateTask(ctx, task)
	_ = repo.ArchiveTask(ctx, "task-123")
	eventBus.ClearEvents()

	err := svc.UnarchiveTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to unarchive task: %v", err)
	}

	// Verify task is no longer archived
	got, err := svc.GetTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if got.ArchivedAt != nil {
		t.Error("expected archived_at to be nil after unarchive")
	}

	// Check task.updated event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "task.updated" {
		t.Errorf("expected event type 'task.updated', got %s", events[0].Type)
	}
}

func TestService_UnarchiveTask_CancelsStaleSession(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test"})

	// Create sessions in various states
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-active", TaskID: "task-123", AgentProfileID: "p1",
		State: models.TaskSessionStateWaitingForInput,
	})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-completed", TaskID: "task-123", AgentProfileID: "p1",
		State: models.TaskSessionStateCompleted,
	})

	_ = repo.ArchiveTask(ctx, "task-123")

	err := svc.UnarchiveTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to unarchive task: %v", err)
	}

	// WAITING_FOR_INPUT session should now be CANCELLED
	sess, err := repo.GetTaskSession(ctx, "sess-active")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if sess.State != models.TaskSessionStateCancelled {
		t.Errorf("expected stale session to be CANCELLED, got %s", sess.State)
	}

	// COMPLETED session should remain unchanged
	sess, err = repo.GetTaskSession(ctx, "sess-completed")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if sess.State != models.TaskSessionStateCompleted {
		t.Errorf("expected completed session to stay COMPLETED, got %s", sess.State)
	}
}

func TestService_UnarchiveTask_NotArchived(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test"})

	err := svc.UnarchiveTask(ctx, "task-123")
	if err == nil {
		t.Error("expected error when unarchiving a non-archived task")
	}
}

func TestService_UnarchiveTask_FallsBackToStartStep(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	// Wire mock step getter that does NOT know "step-deleted"
	svc.SetWorkflowStepGetter(&mockWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{}})
	svc.SetStartStepResolver(&mockStartStepResolver{startSteps: map[string]string{"wf-123": "step-start"}})

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})

	// Create task in a step that will be "deleted"
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-deleted", Title: "Test"})
	_ = repo.ArchiveTask(ctx, "task-123")
	eventBus.ClearEvents()

	err := svc.UnarchiveTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to unarchive task: %v", err)
	}

	// Verify task was moved to start step
	got, err := svc.GetTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if got.WorkflowStepID != "step-start" {
		t.Errorf("expected workflow_step_id 'step-start', got %s", got.WorkflowStepID)
	}
	if got.ArchivedAt != nil {
		t.Error("expected archived_at to be nil")
	}
}

func TestService_DeleteTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test"})
	eventBus.ClearEvents()

	err := svc.DeleteTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to delete task: %v", err)
	}

	// Verify task is deleted
	_, err = svc.GetTask(ctx, "task-123")
	if err == nil {
		t.Error("expected task to be deleted")
	}

	// Check event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestService_ListTasks(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Task 1"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Task 2"})

	tasks, err := svc.ListTasks(ctx, "wf-123")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestService_UpdateTaskState(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	task := &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test", State: v1.TaskStateTODO}
	_ = repo.CreateTask(ctx, task)
	eventBus.ClearEvents()

	updated, err := svc.UpdateTaskState(ctx, "task-123", v1.TaskStateInProgress)
	if err != nil {
		t.Fatalf("failed to update task state: %v", err)
	}
	if updated.State != v1.TaskStateInProgress {
		t.Errorf("expected state IN_PROGRESS, got %s", updated.State)
	}

	// Check event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "task.state_changed" {
		t.Errorf("expected event type 'task.state_changed', got %s", events[0].Type)
	}
}

func TestService_MoveTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})

	task := &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-todo", Title: "Test", State: v1.TaskStateTODO}
	_ = repo.CreateTask(ctx, task)
	eventBus.ClearEvents()

	moved, err := svc.MoveTask(ctx, "task-123", "wf-123", "step-done", 0)
	if err != nil {
		t.Fatalf("failed to move task: %v", err)
	}
	if moved.Task.WorkflowStepID != "step-done" {
		t.Errorf("expected workflow step 'step-done', got %s", moved.Task.WorkflowStepID)
	}
}

// Workflow tests

func TestService_CreateWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	req := &CreateWorkflowRequest{
		WorkspaceID: "ws-1",
		Name:        "Test Workflow",
		Description: "A test workflow",
	}

	workflow, err := svc.CreateWorkflow(ctx, req)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	if workflow.ID == "" {
		t.Error("expected workflow ID to be set")
	}
	if workflow.Name != "Test Workflow" {
		t.Errorf("expected name 'Test Workflow', got %s", workflow.Name)
	}
}

func TestService_GetWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	workflow := &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	retrieved, err := svc.GetWorkflow(ctx, "wf-123")
	if err != nil {
		t.Fatalf("failed to get workflow: %v", err)
	}
	if retrieved.Name != "Test Workflow" {
		t.Errorf("expected name 'Test Workflow', got %s", retrieved.Name)
	}
}

func TestService_UpdateWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	workflow := &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Original"}
	_ = repo.CreateWorkflow(ctx, workflow)

	newName := "Updated"
	req := &UpdateWorkflowRequest{Name: &newName}

	updated, err := svc.UpdateWorkflow(ctx, "wf-123", req)
	if err != nil {
		t.Fatalf("failed to update workflow: %v", err)
	}
	if updated.Name != "Updated" {
		t.Errorf("expected name 'Updated', got %s", updated.Name)
	}
}
func TestService_DeleteWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	workflow := &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	err := svc.DeleteWorkflow(ctx, "wf-123")
	if err != nil {
		t.Fatalf("failed to delete workflow: %v", err)
	}

	_, err = svc.GetWorkflow(ctx, "wf-123")
	if err == nil {
		t.Error("expected workflow to be deleted")
	}
}

func TestService_ListWorkflows(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow 1"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-2", WorkspaceID: "ws-1", Name: "Workflow 2"})

	workflows, err := svc.ListWorkflows(ctx, "ws-1")
	if err != nil {
		t.Fatalf("failed to list workflows: %v", err)
	}
	if len(workflows) != 2 {
		t.Errorf("expected 2 workflows, got %d", len(workflows))
	}
}

// Message tests

func setupTestTask(t *testing.T, repo *sqliterepo.Repository) {
	t.Helper()
	ctx := context.Background()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task"})
}

func setupTestSession(t *testing.T, repo *sqliterepo.Repository) string {
	t.Helper()
	ctx := context.Background()
	session := &models.TaskSession{
		ID:             "session-123",
		TaskID:         "task-123",
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateStarting,
	}
	_ = repo.CreateTaskSession(ctx, session)
	return session.ID
}

func setupTestTurn(t *testing.T, repo *sqliterepo.Repository, sessionID, taskID, turnID string) string {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	turn := &models.Turn{
		ID:            turnID,
		TaskSessionID: sessionID,
		TaskID:        taskID,
		StartedAt:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		t.Fatalf("failed to create test turn: %v", err)
	}
	return turn.ID
}

func TestService_CreateMessage(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	// Create a task first
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-123")
	eventBus.ClearEvents()

	req := &CreateMessageRequest{
		TaskSessionID: sessionID,
		TurnID:        turnID,
		Content:       "This is a test comment",
		AuthorType:    "user",
		AuthorID:      "user-123",
	}

	comment, err := svc.CreateMessage(ctx, req)
	if err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}

	if comment.ID == "" {
		t.Error("expected comment ID to be set")
	}
	if comment.Content != "This is a test comment" {
		t.Errorf("expected content 'This is a test comment', got %s", comment.Content)
	}
	if comment.AuthorType != models.MessageAuthorUser {
		t.Errorf("expected author type 'user', got %s", comment.AuthorType)
	}

	// Check event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "message.added" {
		t.Errorf("expected event type 'message.added', got %s", events[0].Type)
	}
}

func TestService_CreateAgentMessage(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-123")

	req := &CreateMessageRequest{
		TaskSessionID: sessionID,
		TurnID:        turnID,
		Content:       "What should I do next?",
		AuthorType:    "agent",
		AuthorID:      "agent-123",
		RequestsInput: true,
	}

	comment, err := svc.CreateMessage(ctx, req)
	if err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}

	if comment.AuthorType != models.MessageAuthorAgent {
		t.Errorf("expected author type 'agent', got %s", comment.AuthorType)
	}
	if !comment.RequestsInput {
		t.Error("expected RequestsInput to be true")
	}
}

func TestService_CreateMessageSessionNotFound(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()

	req := &CreateMessageRequest{
		TaskSessionID: "nonexistent",
		Content:       "Test comment",
	}

	_, err := svc.CreateMessage(ctx, req)
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestService_GetMessage(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-123")

	comment := &models.Message{ID: "comment-123", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorUser, Content: "Test"}
	_ = repo.CreateMessage(ctx, comment)

	retrieved, err := svc.GetMessage(ctx, "comment-123")
	if err != nil {
		t.Fatalf("failed to get comment: %v", err)
	}
	if retrieved.Content != "Test" {
		t.Errorf("expected content 'Test', got %s", retrieved.Content)
	}
}

func TestService_ListMessages(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-123")

	_ = repo.CreateMessage(ctx, &models.Message{ID: "comment-1", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorUser, Content: "Comment 1"})
	_ = repo.CreateMessage(ctx, &models.Message{ID: "comment-2", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorAgent, Content: "Comment 2"})

	comments, err := svc.ListMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to list comments: %v", err)
	}
	if len(comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(comments))
	}
}

func TestService_DeleteMessage(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-123")

	comment := &models.Message{ID: "comment-123", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorUser, Content: "Test"}
	_ = repo.CreateMessage(ctx, comment)

	err := svc.DeleteMessage(ctx, "comment-123")
	if err != nil {
		t.Fatalf("failed to delete comment: %v", err)
	}

	_, err = svc.GetMessage(ctx, "comment-123")
	if err == nil {
		t.Error("expected comment to be deleted")
	}
}
