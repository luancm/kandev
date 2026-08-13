package process

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/gitremote"
)

func TestGetGitStatus_UsesDeliveredComparisonContext(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	initial := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	writeFile(t, repoDir, "origin-only.txt", "origin\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "origin-only")
	runGit(t, repoDir, "push", "origin", "main")

	runGit(t, repoDir, "checkout", "-b", "canonical", initial)
	writeFile(t, repoDir, "canonical-only.txt", "canonical\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "canonical-only")
	canonicalTip := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "checkout", "-b", "task", "canonical")
	writeFile(t, repoDir, "task-only.txt", "task\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "task-only")

	runGit(t, repoDir, "remote", "add", "canonical-remote", "https://github.com/acme/canonical.git")
	runGit(t, repoDir, "update-ref", "refs/remotes/canonical-remote/base", canonicalTip)

	target := gitremote.RemoteRefIdentity{
		Repository: gitremote.RemoteRepositoryIdentity{
			Provider:       gitremote.ProviderGitHub,
			Host:           "github.com",
			RepositoryPath: "acme/canonical",
		},
		Ref: "base",
	}
	comparison, err := gitremote.NewComparisonContext(target, "", "comparison-1")
	if err != nil {
		t.Fatalf("NewComparisonContext: %v", err)
	}
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracker.SetComparisonContext(&comparison)

	status, err := tracker.GetGitStatus(context.Background(), true)
	if err != nil {
		t.Fatalf("GetGitStatus: %v", err)
	}
	if status.BaseCommit != canonicalTip {
		t.Fatalf("BaseCommit = %q, want canonical comparison tip %q", status.BaseCommit, canonicalTip)
	}
	if status.BranchAdditions != 1 || status.BranchDeletions != 0 {
		t.Fatalf("branch diff = +%d -%d, want +1 -0", status.BranchAdditions, status.BranchDeletions)
	}
	if status.Ahead != 1 || status.Behind != 0 {
		t.Fatalf("comparison divergence = ahead %d behind %d, want 1/0", status.Ahead, status.Behind)
	}
	if status.HeadRemote != nil && status.HeadRemote.Host == "github.com" && status.HeadRemote.Repo == "canonical" {
		t.Fatal("comparison remote was incorrectly used as the writable head")
	}
}

func TestGetGitStatus_UnresolvedComparisonRetainsQualifiedStoredAnchor(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	storedBase := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	writeFile(t, repoDir, "origin-only.txt", "origin\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "origin-only")
	runGit(t, repoDir, "push", "origin", "main")

	target := gitremote.RemoteRefIdentity{
		Repository: gitremote.RemoteRepositoryIdentity{
			Provider:       gitremote.ProviderGitHub,
			Host:           "github.com",
			RepositoryPath: "acme/missing",
		},
		Ref: "main",
	}
	comparison, err := gitremote.NewComparisonContext(target, storedBase, "comparison-2")
	if err != nil {
		t.Fatalf("NewComparisonContext: %v", err)
	}
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracker.SetComparisonContext(&comparison)

	status, err := tracker.GetGitStatus(context.Background(), true)
	if err != nil {
		t.Fatalf("GetGitStatus: %v", err)
	}
	if status.BaseCommit != storedBase {
		t.Fatalf("status base = %q, want qualified stored anchor %q", status.BaseCommit, storedBase)
	}
	if status.BranchAdditions != 0 || status.BranchDeletions != 0 || status.Ahead != 0 || status.Behind != 0 {
		t.Fatalf("legacy counts should remain zero projections for unresolved evidence: %+v", status)
	}
}
