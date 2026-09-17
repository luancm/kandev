package backendapp

import (
	"context"
	"testing"
	"time"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

type gitPushScopeFixture struct {
	repo      *bootStateTestHarness
	taskID    string
	sessionID string
}

func newGitPushScopeFixture(t *testing.T) gitPushScopeFixture {
	t.Helper()
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	const (
		workspaceID = "git-push-scope-workspace"
		taskID      = "git-push-scope-task"
		environment = "git-push-scope-environment"
		sessionID   = "git-push-scope-session"
		repoA       = "git-push-scope-repo-a"
		repoB       = "git-push-scope-repo-b"
		taskRepoA   = "git-push-scope-task-repo-a"
		taskRepoB   = "git-push-scope-task-repo-b"
		envRepoA    = "git-push-scope-env-repo-a"
		envRepoB    = "git-push-scope-env-repo-b"
	)
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	if err := harness.taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := harness.taskRepo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: workspaceID, Title: taskID, Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	for _, repository := range []*models.Repository{
		{ID: repoA, WorkspaceID: workspaceID, Name: "repo-a", SourceType: "local"},
		{ID: repoB, WorkspaceID: workspaceID, Name: "repo-b", SourceType: "local"},
	} {
		if err := harness.taskRepo.CreateRepository(ctx, repository); err != nil {
			t.Fatalf("CreateRepository(%s): %v", repository.ID, err)
		}
	}
	for _, link := range []*models.TaskRepository{
		{ID: taskRepoA, TaskID: taskID, RepositoryID: repoA, BaseBranch: "main", Position: 0},
		{ID: taskRepoB, TaskID: taskID, RepositoryID: repoB, BaseBranch: "main", Position: 1},
	} {
		if err := harness.taskRepo.CreateTaskRepository(ctx, link); err != nil {
			t.Fatalf("CreateTaskRepository(%s): %v", link.ID, err)
		}
	}
	if err := harness.taskRepo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environment, TaskID: taskID, ExecutorType: string(models.ExecutorTypeLocal),
		WorkspacePath: "/tmp/git-push-scope", Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{
			{ID: envRepoA, RepositoryID: repoA, WorktreePath: "/tmp/git-push-scope/repo-a", WorktreeBranch: "main", Position: 0},
			{ID: envRepoB, RepositoryID: repoB, WorktreePath: "/tmp/git-push-scope/repo-b", WorktreeBranch: "main", Position: 1},
		},
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := harness.taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environment,
		WorkspacePath: "/tmp/git-push-scope", State: models.TaskSessionStateWaitingForInput,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	return gitPushScopeFixture{repo: &harness, taskID: taskID, sessionID: sessionID}
}

func TestResolveTaskRepositoryIDForSubpathUsesTaskRepositoryLinkID(t *testing.T) {
	fixture := newGitPushScopeFixture(t)
	ctx := context.Background()
	log := newTestLogger()

	if got := resolveTaskRepositoryIDForSubpath(ctx, fixture.repo.taskRepo, fixture.taskID, "repo-b", log); got != "git-push-scope-task-repo-b" {
		t.Fatalf("named repository scope = %q, want task repository link ID", got)
	}
	if got := resolveTaskRepositoryIDForSubpath(ctx, fixture.repo.taskRepo, fixture.taskID, "", log); got != "" {
		t.Fatalf("empty multi-repository scope = %q, want fail closed", got)
	}
}

func TestResolveTaskRepositoryIDForStatusRequiresEnvironmentScope(t *testing.T) {
	fixture := newGitPushScopeFixture(t)
	ctx := context.Background()
	log := newTestLogger()

	if got := resolveTaskRepositoryIDForStatus(ctx, fixture.repo.taskRepo, fixture.sessionID, fixture.taskID, "repo-b", log); got != "git-push-scope-task-repo-b" {
		t.Fatalf("status repository scope = %q, want task repository link ID", got)
	}
	if got := resolveTaskRepositoryIDForStatus(ctx, fixture.repo.taskRepo, fixture.sessionID, fixture.taskID, "unrelated-repository", log); got != "" {
		t.Fatalf("foreign status scope = %q, want fail closed", got)
	}
	if got := resolveTaskRepositoryIDForStatus(ctx, fixture.repo.taskRepo, fixture.sessionID, fixture.taskID, "", log); got != "" {
		t.Fatalf("empty multi-repository status scope = %q, want fail closed", got)
	}
}

func TestGitPushStatusObservationDoesNotAssumeMissingRemoteCountersAreKnown(t *testing.T) {
	observation := gitPushStatusObservationFromStatus(runtimeapi.GitStatusResult{
		Success:          true,
		Branch:           "main",
		RemoteBranch:     "origin/main",
		HeadCommit:       "head",
		RemoteHeadCommit: "head",
	}, "task-repo")
	if observation.RemoteAheadKnown || observation.RemoteBehindKnown {
		t.Fatalf("missing remote counters marked known: %+v", observation)
	}
}

func TestGitPushStatusObservationPrefersSourceTimestamp(t *testing.T) {
	sourceAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	receivedAt := sourceAt.Add(time.Hour)
	observation := gitPushStatusObservationFromStatus(runtimeapi.GitStatusResult{
		Timestamp: sourceAt.Format(time.RFC3339Nano),
	}, "task-repo", receivedAt)
	if !observation.ObservedAt.Equal(sourceAt) {
		t.Fatalf("source timestamp = %s, want %s", observation.ObservedAt, sourceAt)
	}
	observation = gitPushStatusObservationFromStatus(runtimeapi.GitStatusResult{}, "task-repo", receivedAt)
	if !observation.ObservedAt.Equal(receivedAt) {
		t.Fatalf("fallback timestamp = %s, want %s", observation.ObservedAt, receivedAt)
	}
}

func TestGitPushStatusObservationFromSnapshotPreservesMissingRemoteCounters(t *testing.T) {
	observation := gitPushStatusObservationFromSnapshot(&models.GitSnapshot{
		Branch:       "main",
		RemoteBranch: "origin/main",
		HeadCommit:   "head",
		Metadata:     map[string]interface{}{"remote_head_commit": "head"},
		CreatedAt:    time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
	}, "task-repo")
	if observation.RemoteAheadKnown || observation.RemoteBehindKnown {
		t.Fatalf("missing snapshot remote counters marked known: %+v", observation)
	}
}

func TestGitPushStatusObservationFromSnapshotPrefersSourceTimestamp(t *testing.T) {
	sourceAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	persistedAt := sourceAt.Add(time.Hour)
	observation := gitPushStatusObservationFromSnapshot(&models.GitSnapshot{
		CreatedAt: persistedAt,
		Metadata: map[string]interface{}{
			"timestamp": sourceAt.Format(time.RFC3339Nano),
		},
	}, "task-repo")
	if !observation.ObservedAt.Equal(sourceAt) {
		t.Fatalf("snapshot source timestamp = %s, want %s", observation.ObservedAt, sourceAt)
	}
}
