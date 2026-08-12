package github

import (
	"context"
	"errors"
	"testing"
)

func TestFindPRForWatchUsesAttachedRepositoryAfterCanonicalResolution(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	if _, err := store.db.Exec(`CREATE TABLE repositories (
		id TEXT PRIMARY KEY,
		provider TEXT NOT NULL DEFAULT '',
		provider_owner TEXT NOT NULL DEFAULT '',
		provider_name TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create repositories table: %v", err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO repositories (id, provider, provider_owner, provider_name) VALUES (?, ?, ?, ?)`,
		"attached-repository", "github", "attached", "project",
	); err != nil {
		t.Fatalf("seed attached repository: %v", err)
	}

	mockClient.AddPR(&PR{
		Number:     42,
		State:      "open",
		RepoOwner:  "attached",
		RepoName:   "project",
		HeadBranch: "Feature/Case",
	})
	watch := withTestWorkspace(&PRWatch{
		ID:           "watch-anchor",
		SessionID:    "session-1",
		TaskID:       "task-1",
		RepositoryID: "attached-repository",
		Owner:        "canonical",
		Repo:         "project",
		Branch:       "Feature/Case",
	})
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	got, err := svc.findPRForWatch(ctx, watch)
	if err != nil {
		t.Fatalf("find PR for watch: %v", err)
	}
	if got == nil || got.Number != 42 {
		t.Fatalf("found PR = %+v, want attached/project#42", got)
	}
}

func TestFindPRForWatchRejectsUnresolvedAttachedRepository(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	mockClient.AddPR(&PR{Number: 42, State: "open", RepoOwner: "canonical", RepoName: "project", HeadBranch: "feature"})

	watch := withTestWorkspace(&PRWatch{
		ID:           "watch-unresolved-anchor",
		SessionID:    "session-1",
		TaskID:       "task-1",
		RepositoryID: "missing-repository",
		Owner:        "canonical",
		Repo:         "project",
		Branch:       "feature",
	})
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	got, err := svc.findPRForWatch(ctx, watch)
	if got != nil {
		t.Fatalf("found PR through mutable canonical fields: %+v", got)
	}
	if !errors.Is(err, ErrRepoNotResolvable) {
		t.Fatalf("find error = %v, want ErrRepoNotResolvable", err)
	}
}

func TestFindPRForWatchExactHeadUsesAttachedRepositoryAndLiteralBranch(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	if _, err := store.db.Exec(`CREATE TABLE repositories (
		id TEXT PRIMARY KEY,
		provider TEXT NOT NULL DEFAULT '',
		provider_owner TEXT NOT NULL DEFAULT '',
		provider_name TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create repositories table: %v", err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO repositories (id, provider, provider_owner, provider_name) VALUES (?, ?, ?, ?)`,
		"attached-fork", "github", "fork", "project",
	); err != nil {
		t.Fatalf("seed attached repository: %v", err)
	}
	mockClient.AddRepos("upstream", []GitHubRepo{{Owner: "upstream", Name: "project", FullName: "upstream/project"}})
	mockClient.AddPR(&PR{
		Number:        43,
		State:         "open",
		RepoOwner:     "upstream",
		RepoName:      "project",
		HeadRepoOwner: "fork",
		HeadRepoName:  "project",
		HeadBranch:    "Review/Case",
	})
	watch := withTestWorkspace(&PRWatch{
		ID: "watch-exact-anchor", SessionID: "session-2", TaskID: "task-2", RepositoryID: "attached-fork",
		Owner: "upstream", Repo: "project", Branch: "local-name",
		HeadHost: "github.com", HeadOwner: "fork", HeadRepo: "project", HeadBranch: "Review/Case",
	})
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	got, err := svc.findPRForWatch(ctx, watch)
	if err != nil {
		t.Fatalf("find exact PR for watch: %v", err)
	}
	if got == nil || got.Number != 43 {
		t.Fatalf("found PR = %+v, want exact fork head PR #43", got)
	}
}
