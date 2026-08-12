package process

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/gitremote"
)

func TestResolveGitRemoteRoles(t *testing.T) {
	t.Setenv(gitLabHostEnv, "https://gitlab.example.com")
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	runGit(t, repoDir, "checkout", "-b", "feature/Local")
	runGit(t, repoDir, "remote", "add", "tracking-name", "https://gitlab.example.com/group/nested/project.git")
	runGit(t, repoDir, "remote", "add", "publish-name", "https://github.com/other/widget.git")
	runGit(t, repoDir, "remote", "set-url", "--push", "publish-name", "https://user:secret@github.com/Contributor/Widget.git")
	runGit(t, repoDir, "config", "branch.feature/Local.remote", "tracking-name")
	runGit(t, repoDir, "config", "branch.feature/Local.merge", "refs/heads/Track/Case")
	runGit(t, repoDir, "config", "branch.feature/Local.pushRemote", "publish-name")
	runGit(t, repoDir, "config", "remote.publish-name.push", "refs/heads/feature/Local:refs/heads/Review/Case")

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	roles := tracker.ResolveGitRemoteRoles(context.Background(), nil)

	if roles.ActionHead.State != gitremote.ResolutionResolved {
		t.Fatalf("action head state = %q, want resolved: %+v", roles.ActionHead.State, roles.ActionHead)
	}
	if got := roles.ActionHead.Identity.Repository.RepositoryPath; got != "Contributor/Widget" {
		t.Fatalf("action repository path = %q, want Contributor/Widget", got)
	}
	if got := roles.ActionHead.Identity.Ref; got != "Review/Case" {
		t.Fatalf("action ref = %q, want Review/Case", got)
	}
	if got := roles.TrackingUpstream.Identity.Repository.RepositoryPath; got != "group/nested/project" {
		t.Fatalf("tracking repository path = %q, want group/nested/project", got)
	}
	if got := roles.TrackingUpstream.Identity.Ref; got != "Track/Case" {
		t.Fatalf("tracking ref = %q, want Track/Case", got)
	}
	if roles.ActionHead.RemoteName != "publish-name" || roles.TrackingUpstream.RemoteName != "tracking-name" {
		t.Fatalf("remote names = (%q, %q), want custom names", roles.ActionHead.RemoteName, roles.TrackingUpstream.RemoteName)
	}
	if roles.ActionHead.Identity.Repository.RepositoryPath == roles.TrackingUpstream.Identity.Repository.RepositoryPath {
		t.Fatal("action and tracking repositories unexpectedly collapsed")
	}
}

func TestResolveGitRemoteRolesComparisonRejectsAmbiguousRemotes(t *testing.T) {
	t.Setenv(gitLabHostEnv, "https://gitlab.example.com")
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	runGit(t, repoDir, "checkout", "-b", "feature/local")
	runGit(t, repoDir, "remote", "add", "one", "https://gitlab.example.com/group/project.git")
	runGit(t, repoDir, "remote", "add", "two", "https://GITLAB.example.com/group/project.git")
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	target := &gitremote.RemoteRefIdentity{
		Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderGitLab, Host: "gitlab.example.com", RepositoryPath: "group/project"},
		Ref:        "main",
	}

	roles := tracker.ResolveGitRemoteRoles(context.Background(), target)
	if roles.Comparison.State != gitremote.ResolutionAmbiguous {
		t.Fatalf("comparison state = %q, want ambiguous: %+v", roles.Comparison.State, roles.Comparison)
	}
}

func TestResolveGitRemoteRolesStripsCredentialsAndPreservesBranchCase(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	runGit(t, repoDir, "checkout", "-b", "feature/Case")
	runGit(t, repoDir, "remote", "rename", "origin", "publish")
	runGit(t, repoDir, "remote", "set-url", "publish", "https://alice:password@github.com/acme/widget.git")
	runGit(t, repoDir, "config", "branch.feature/Case.pushRemote", "publish")
	runGit(t, repoDir, "config", "remote.publish.push", "refs/heads/feature/Case:refs/heads/Review/Case")

	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	roles := tracker.ResolveGitRemoteRoles(context.Background(), nil)
	if roles.ActionHead.Identity.Repository.Host != "github.com" {
		t.Fatalf("host = %q, want github.com", roles.ActionHead.Identity.Repository.Host)
	}
	if roles.ActionHead.Identity.Ref != "Review/Case" {
		t.Fatalf("ref = %q, want Review/Case", roles.ActionHead.Identity.Ref)
	}
	if roles.ActionHead.Identity.Repository.RepositoryPath != "acme/widget" {
		t.Fatalf("repository path = %q, want acme/widget", roles.ActionHead.Identity.Repository.RepositoryPath)
	}
	if roles.ActionHead.Identity.Repository.ProviderRepositoryID != "" {
		t.Fatalf("unexpected provider ID %q", roles.ActionHead.Identity.Repository.ProviderRepositoryID)
	}
}

func TestObserveGitRemoteRefDistinguishesAbsentDestination(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	runGit(t, repoDir, "checkout", "-b", "first-push")
	localRemote := strings.TrimSpace(runGit(t, repoDir, "remote", "get-url", "origin"))
	runGit(t, repoDir, "config", "url."+localRemote+"/.insteadOf", "https://github.com/acme/widget.git")
	runGit(t, repoDir, "remote", "set-url", "origin", "https://github.com/acme/widget.git")
	runGit(t, repoDir, "config", "branch.first-push.pushRemote", "origin")
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	roles := tracker.ResolveGitRemoteRoles(context.Background(), nil)
	if roles.ActionHead.State != gitremote.ResolutionResolved {
		t.Fatalf("action head state = %q, want resolved: %+v", roles.ActionHead.State, roles.ActionHead)
	}

	observation := tracker.ObserveGitRemoteRef(context.Background(), roles.ActionHead)
	if observation.State != gitremote.ObservationAbsent {
		t.Fatalf("observation state = %q, want absent: %+v", observation.State, observation)
	}
	if observation.Identity == nil || observation.Identity.Ref != "first-push" {
		t.Fatalf("observation identity = %+v, want first-push", observation.Identity)
	}
	if observation.RemoteHeadCommit != "" || observation.Ahead != nil || observation.Behind != nil {
		t.Fatalf("absent observation carried present/count evidence: %+v", observation)
	}
}

func TestResolveGitRemoteRolesRejectsConflictingPushURLs(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	runGit(t, repoDir, "checkout", "-b", "conflicting")
	runGit(t, repoDir, "remote", "set-url", "--push", "origin", "https://github.com/acme/one.git")
	runGit(t, repoDir, "remote", "set-url", "--add", "--push", "origin", "https://github.com/acme/two.git")
	runGit(t, repoDir, "config", "branch.conflicting.pushRemote", "origin")
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	roles := tracker.ResolveGitRemoteRoles(context.Background(), nil)
	if roles.ActionHead.State != gitremote.ResolutionAmbiguous {
		t.Fatalf("action head state = %q, want ambiguous: %+v", roles.ActionHead.State, roles.ActionHead)
	}
}

func TestResolveGitRemoteRolesRejectsLocalOnlyRemote(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	runGit(t, repoDir, "checkout", "-b", "local-only")
	runGit(t, repoDir, "remote", "add", "local-only", "/tmp/local-only-repository.git")
	runGit(t, repoDir, "config", "branch.local-only.pushRemote", "local-only")
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	roles := tracker.ResolveGitRemoteRoles(context.Background(), nil)
	if roles.ActionHead.State != gitremote.ResolutionUnresolved {
		t.Fatalf("action head state = %q, want unresolved: %+v", roles.ActionHead.State, roles.ActionHead)
	}
}

func TestParseRemoteRepositoryIdentityPreservesProviderPaths(t *testing.T) {
	t.Setenv(gitLabHostEnv, "https://gitlab.example.com")
	tests := []struct {
		name      string
		remoteURL string
		provider  gitremote.Provider
		host      string
		path      string
	}{
		{
			name:      "nested gitlab ssh",
			remoteURL: "git@gitlab.example.com:group/nested/project.git",
			provider:  gitremote.ProviderGitLab,
			host:      "gitlab.example.com",
			path:      "group/nested/project",
		},
		{
			name:      "azure devops http",
			remoteURL: "https://dev.azure.com/acme/Platform/_git/Widget.git",
			provider:  gitremote.ProviderAzureRepos,
			host:      "dev.azure.com",
			path:      "acme/Platform/Widget",
		},
		{
			name:      "azure visualstudio http",
			remoteURL: "https://acme.visualstudio.com/Platform/_git/Widget.git",
			provider:  gitremote.ProviderAzureRepos,
			host:      "acme.visualstudio.com",
			path:      "acme/Platform/Widget",
		},
		{
			name:      "azure devops ssh",
			remoteURL: "git@ssh.dev.azure.com:v3/acme/Platform/Widget",
			provider:  gitremote.ProviderAzureRepos,
			host:      "ssh.dev.azure.com",
			path:      "acme/Platform/Widget",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity, ok := parseRemoteRepositoryIdentity(tt.remoteURL)
			if !ok {
				t.Fatalf("parseRemoteRepositoryIdentity() rejected %q", tt.remoteURL)
			}
			if identity.Provider != tt.provider || identity.Host != tt.host || identity.RepositoryPath != tt.path {
				t.Fatalf("identity = %+v, want provider=%q host=%q path=%q", identity, tt.provider, tt.host, tt.path)
			}
		})
	}
}
