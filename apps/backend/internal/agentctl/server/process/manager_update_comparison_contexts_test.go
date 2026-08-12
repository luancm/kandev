package process

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/gitremote"
)

func TestManagerUpdateComparisonContextsPreservesUnmentionedWorktree(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	mgr := NewManager(&config.InstanceConfig{WorkDir: repoDir}, newTestLogger(t))
	root := mgr.GetWorkspaceTracker()
	first, err := gitremote.NewComparisonContext(gitremote.RemoteRefIdentity{
		Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderGitHub, Host: "github.com", RepositoryPath: "acme/widget"},
		Ref:        "main",
	}, "", "g1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := gitremote.NewComparisonContext(gitremote.RemoteRefIdentity{
		Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderGitHub, Host: "github.com", RepositoryPath: "acme/widget"},
		Ref:        "release",
	}, "", "g2")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.UpdateComparisonContexts(context.Background(), map[string]gitremote.ComparisonContext{"": first, "sibling": second}); err != nil {
		t.Fatal(err)
	}
	if got := root.ComparisonContext(); got == nil || got.Target.Ref != "main" {
		t.Fatalf("root comparison context = %#v, want main", got)
	}
	if err := mgr.UpdateComparisonContexts(context.Background(), map[string]gitremote.ComparisonContext{"sibling": second}); err != nil {
		t.Fatal(err)
	}
	if got := root.ComparisonContext(); got == nil || got.Target.Ref != "main" {
		t.Fatalf("root comparison context after sibling update = %#v, want retained main", got)
	}
	if err := mgr.UpdateComparisonContexts(context.Background(), map[string]gitremote.ComparisonContext{}); err != nil {
		t.Fatal(err)
	}
	if got := root.ComparisonContext(); got != nil {
		t.Fatalf("root comparison context after explicit clear = %#v, want nil", got)
	}
}

func TestManagerUpdateComparisonContextsRejectsCredentialIdentityAtomically(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	mgr := NewManager(&config.InstanceConfig{WorkDir: repoDir}, newTestLogger(t))
	valid, err := gitremote.NewComparisonContext(gitremote.RemoteRefIdentity{
		Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderGitHub, Host: "github.com", RepositoryPath: "acme/widget"},
		Ref:        "main",
	}, "", "g1")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.UpdateComparisonContexts(context.Background(), map[string]gitremote.ComparisonContext{"": valid}); err != nil {
		t.Fatal(err)
	}
	invalid := valid.Clone()
	invalid.Target.Repository.Host = "user:secret@github.com"
	if err := mgr.UpdateComparisonContexts(context.Background(), map[string]gitremote.ComparisonContext{"": invalid}); err == nil {
		t.Fatal("credential-bearing context was accepted")
	}
	if got := mgr.GetWorkspaceTracker().ComparisonContext(); got == nil || got.Target.Ref != "main" {
		t.Fatalf("invalid update replaced known context: %#v", got)
	}
}

func TestManagerUpdateComparisonContextsClearsOnlyNamedWorktree(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()
	mgr := NewManager(&config.InstanceConfig{WorkDir: repoDir}, newTestLogger(t))
	root := mgr.GetWorkspaceTracker()
	value, err := gitremote.NewComparisonContext(gitremote.RemoteRefIdentity{
		Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderGitHub, Host: "github.com", RepositoryPath: "acme/widget"},
		Ref:        "main",
	}, "", "g1")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.UpdateComparisonContexts(context.Background(), map[string]gitremote.ComparisonContext{"": value, "sibling": value}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.UpdateComparisonContexts(context.Background(), map[string]gitremote.ComparisonContext{"sibling": gitremote.ClearComparisonContext("g2")}); err != nil {
		t.Fatal(err)
	}
	if got := root.ComparisonContext(); got == nil || got.Target.Ref != "main" {
		t.Fatalf("root comparison context after sibling clear = %#v, want retained main", got)
	}
	if _, ok := mgr.ComparisonContexts()["sibling"]; ok {
		t.Fatal("cleared sibling remained in manager context map")
	}
}
