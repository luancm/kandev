package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
)

// Keep the existing broad GitHub-service test double compatible with the
// expanded interface without making its already-large source file grow.
func (m *mockGitHubService) FindPRByExactHeadForWorkspace(
	ctx context.Context, _, owner, repo string, head github.PRHeadRef,
) (*github.PR, error) {
	if m.client == nil {
		return nil, nil
	}
	if finder, ok := m.client.(github.ExactPRBranchFinder); ok {
		return finder.FindPRByExactHead(ctx, owner, repo, head)
	}
	return m.client.FindPRByBranch(ctx, owner, repo, head.Branch)
}

func (m *mockGitHubService) UpdatePRWatchSearchTargetIfSearching(
	ctx context.Context, id, branch, _, _, _, _ string,
) error {
	return m.UpdatePRWatchBranchIfSearching(ctx, id, branch)
}

func (m *mockGitHubService) ResolvePRWatch(ctx context.Context, id, _, _ string, prNumber int) (bool, error) {
	if err := m.UpdatePRWatchPRNumber(ctx, id, prNumber); err != nil {
		return false, err
	}
	return true, nil
}

type remoteHeadGitHubService struct {
	*mockGitHubService
	updatedHead      github.PRHeadRef
	updatedBranch    string
	lookupOwner      string
	lookupRepo       string
	createdHead      github.PRHeadRef
	createdWithHead  bool
	associatedOwner  string
	associatedRepo   string
	resolvedBase     github.PRHeadRef
	resolvedPRNumber int
}

func (m *remoteHeadGitHubService) FindPRByExactHeadForWorkspace(
	ctx context.Context, _ string, owner, repo string, head github.PRHeadRef,
) (*github.PR, error) {
	m.lookupOwner = owner
	m.lookupRepo = repo
	return m.mockGitHubService.FindPRByExactHeadForWorkspace(ctx, "", owner, repo, head)
}

func (m *remoteHeadGitHubService) CreatePRWatchForWorkspaceWithHead(
	ctx context.Context, workspaceID, sessionID, taskID, repositoryID, owner, repo string,
	prNumber int, branch string, head github.PRHeadRef,
) (*github.PRWatch, error) {
	m.createdWithHead = true
	m.createdHead = head
	return m.CreatePRWatchForWorkspace(
		ctx, workspaceID, sessionID, taskID, repositoryID, owner, repo, prNumber, branch,
	)
}

func (m *remoteHeadGitHubService) UpdatePRWatchSearchTargetIfSearching(
	_ context.Context, _, branch, headHost, headOwner, headRepo, headBranch string,
) error {
	m.updatedBranch = branch
	m.updatedHead = github.PRHeadRef{Host: headHost, Owner: headOwner, Repo: headRepo, Branch: headBranch}
	return nil
}

func (m *remoteHeadGitHubService) ResolvePRWatch(
	_ context.Context, _ string, owner, repo string, prNumber int,
) (bool, error) {
	m.resolvedBase = github.PRHeadRef{Owner: owner, Repo: repo}
	m.resolvedPRNumber = prNumber
	return true, nil
}

func (m *remoteHeadGitHubService) AssociatePRWithTask(
	ctx context.Context, taskID, repositoryID string, pr *github.PR,
) (*github.TaskPR, error) {
	if pr != nil {
		m.associatedOwner = pr.RepoOwner
		m.associatedRepo = pr.RepoName
	}
	return m.mockGitHubService.AssociatePRWithTask(ctx, taskID, repositoryID, pr)
}

func (m *remoteHeadGitHubService) AssociatePRWithTaskForWorkspace(
	ctx context.Context, workspaceID, taskID, repositoryID string, pr *github.PR,
) (*github.TaskPR, error) {
	if pr != nil {
		m.associatedOwner = pr.RepoOwner
		m.associatedRepo = pr.RepoName
	}
	return m.mockGitHubService.AssociatePRWithTaskForWorkspace(ctx, workspaceID, taskID, repositoryID, pr)
}

func TestSyncPRWatchTargetPersistsRuntimeHead(t *testing.T) {
	svc := newRemoteHeadTestService(t)
	ghSvc := &remoteHeadGitHubService{mockGitHubService: &mockGitHubService{prWatch: &github.PRWatch{
		ID: "watch-1", SessionID: "s1", TaskID: "t1", PRNumber: 0, Branch: "local-feature",
	}}}
	svc.SetGitHubService(ghSvc)

	svc.syncPRWatchTarget(context.Background(), "t1", "s1", "", "local-feature", &streams.GitHeadRemote{
		Provider: "github", Host: "github.com", Owner: "fork", Repo: "project", Branch: "review-feature",
	})

	if ghSvc.updatedBranch != "local-feature" {
		t.Fatalf("updated branch = %q, want local-feature", ghSvc.updatedBranch)
	}
	if ghSvc.updatedHead.Owner != "fork" || ghSvc.updatedHead.Repo != "project" || ghSvc.updatedHead.Branch != "review-feature" {
		t.Fatalf("runtime head = %+v, want fork/project:review-feature", ghSvc.updatedHead)
	}
}

func TestDetectPushAndAssociatePRUsesExactRuntimeHeadAndCanonicalBase(t *testing.T) {
	svc := newRemoteHeadTestService(t)
	client := github.NewMockClient()
	client.AddPR(&github.PR{
		Number:        17,
		State:         "open",
		RepoOwner:     "upstream",
		RepoName:      "project",
		HeadRepoOwner: "fork",
		HeadRepoName:  "project",
		HeadBranch:    "review-feature",
		BaseBranch:    "main",
	})
	ghSvc := &remoteHeadGitHubService{mockGitHubService: &mockGitHubService{
		client: client,
		prWatch: &github.PRWatch{
			ID: "watch-1", SessionID: "s1", TaskID: "t1", Owner: "fork", Repo: "project", PRNumber: 0,
			Branch: "local-feature", HeadHost: "github.com", HeadOwner: "fork", HeadRepo: "project", HeadBranch: "review-feature",
		},
	}}
	svc.SetGitHubService(ghSvc)

	svc.detectPushAndAssociatePR(context.Background(), "s1", "t1", "", "local-feature", &streams.GitHeadRemote{
		Provider: "github", Host: "github.com", Owner: "fork", Repo: "project", Branch: "review-feature",
	})

	if ghSvc.associatedOwner != "upstream" || ghSvc.associatedRepo != "project" {
		t.Fatalf("associated base = %s/%s, want upstream/project", ghSvc.associatedOwner, ghSvc.associatedRepo)
	}
}

func TestDetectPushAndAssociatePRPersistsRuntimeHeadOnNewWatch(t *testing.T) {
	svc := newRemoteHeadTestService(t)
	client := github.NewMockClient()
	client.AddPR(&github.PR{
		Number:        17,
		State:         "open",
		RepoOwner:     "upstream",
		RepoName:      "project",
		HeadRepoOwner: "fork",
		HeadRepoName:  "project",
		HeadBranch:    "review-feature",
		BaseBranch:    "main",
	})
	ghSvc := &remoteHeadGitHubService{mockGitHubService: &mockGitHubService{client: client}}
	svc.SetGitHubService(ghSvc)

	svc.detectPushAndAssociatePR(context.Background(), "s1", "t1", "", "local-feature", &streams.GitHeadRemote{
		Provider: "github", Host: "github.com", Owner: "fork", Repo: "project", Branch: "review-feature",
	})

	if !ghSvc.createdWithHead {
		t.Fatal("new resolved watch was created without the runtime head")
	}
	if ghSvc.createdHead.Host != "github.com" || ghSvc.createdHead.Owner != "fork" || ghSvc.createdHead.Repo != "project" || ghSvc.createdHead.Branch != "review-feature" {
		t.Fatalf("created runtime head = %+v, want github.com fork/project:review-feature", ghSvc.createdHead)
	}
}

func TestSearchExistingWatchUsesAttachedRepositoryAfterCanonicalResolution(t *testing.T) {
	svc := newRemoteHeadTestService(t)
	client := github.NewMockClient()
	client.AddPR(&github.PR{
		Number:        17,
		State:         "open",
		RepoOwner:     "upstream",
		RepoName:      "project",
		HeadRepoOwner: "fork",
		HeadRepoName:  "project",
		HeadBranch:    "review-feature",
		BaseBranch:    "main",
	})
	ghSvc := &remoteHeadGitHubService{mockGitHubService: &mockGitHubService{
		client: client,
		prWatch: &github.PRWatch{
			ID: "watch-1", SessionID: "s1", TaskID: "t1", RepositoryID: "repo1", Owner: "upstream", Repo: "project",
			PRNumber: 0, Branch: "local-feature",
		},
	}}
	svc.SetGitHubService(ghSvc)

	svc.searchPRForExistingWatch(
		context.Background(), "ws1", ghSvc.prWatch, "fork", "project", "s1", "t1", "local-feature",
		&streams.GitHeadRemote{Provider: "github", Host: "github.com", Owner: "fork", Repo: "project", Branch: "review-feature"},
	)

	if ghSvc.lookupOwner != "fork" || ghSvc.lookupRepo != "project" {
		t.Fatalf("search lookup repository = %s/%s, want persisted attachment fork/project", ghSvc.lookupOwner, ghSvc.lookupRepo)
	}
	if ghSvc.updatedHead.Owner != "fork" || ghSvc.updatedHead.Branch != "review-feature" {
		t.Fatalf("searching watch head = %+v, want exact observed action head", ghSvc.updatedHead)
	}
}

func newRemoteHeadTestService(t *testing.T) *Service {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC()
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "project", SourceType: "provider", Provider: "github",
		ProviderOwner: "fork", ProviderName: "project", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	session, _ := repo.GetTaskSession(ctx, "s1")
	session.RepositoryID = "repo1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update task session: %v", err)
	}
	return createTestService(repo, newMockStepGetter(), newMockTaskRepo())
}
