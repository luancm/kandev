package process

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/gitremote"
)

func TestGitOperatorPushUsesActionHeadAndRejectsMissingTrackingPull(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	trackingURL := "https://github.com/canonical/widget.git"
	actionURL := "https://github.com/contributor/widget.git"
	baseURL := "https://github.com/acme/widget.git"
	trackingDir := filepath.Join(t.TempDir(), "tracking.git")
	actionDir := filepath.Join(t.TempDir(), "action.git")
	runGit(t, filepath.Dir(trackingDir), "init", "--bare", "--initial-branch=main", trackingDir)
	runGit(t, filepath.Dir(actionDir), "init", "--bare", "--initial-branch=main", actionDir)
	localOrigin := strings.TrimSpace(runGit(t, repoDir, "remote", "get-url", "origin"))
	runGit(t, repoDir, "config", "url."+localOrigin+".insteadOf", trackingURL)
	runGit(t, repoDir, "config", "url."+actionDir+".insteadOf", actionURL)
	runGit(t, repoDir, "config", "url."+localOrigin+".insteadOf", baseURL)
	runGit(t, repoDir, "remote", "rename", "origin", "tracking")
	runGit(t, repoDir, "remote", "set-url", "tracking", trackingURL)
	runGit(t, repoDir, "remote", "add", "action", actionURL)
	runGit(t, repoDir, "remote", "add", "comparison", baseURL)
	runGit(t, repoDir, "checkout", "-b", "feature/Case")
	writeFile(t, repoDir, "feature.txt", "feature\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "feature")
	runGit(t, repoDir, "config", "branch.feature/Case.remote", "tracking")
	runGit(t, repoDir, "config", "branch.feature/Case.merge", "refs/heads/main")
	runGit(t, repoDir, "config", "branch.feature/Case.pushRemote", "action")
	runGit(t, repoDir, "config", "remote.action.push", "refs/heads/feature/Case:refs/heads/Review/Case")

	target := gitremote.RemoteRefIdentity{
		Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderGitHub, Host: "github.com", RepositoryPath: "acme/widget"},
		Ref:        "main",
	}
	comparison, err := gitremote.NewComparisonContext(target, "", "comparison-generation")
	if err != nil {
		t.Fatalf("NewComparisonContext: %v", err)
	}
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracker.SetComparisonContext(&comparison)
	roles := tracker.ResolveGitRemoteRoles(context.Background(), &target)
	if roles.ActionHead.Identity == nil {
		t.Fatalf("action role did not resolve: %+v", roles.ActionHead)
	}

	operator := NewGitOperator(repoDir, newTestLogger(t), tracker)
	pushed, err := operator.PushWithExpectation(context.Background(), false, false, GitMutationExpectation{
		RemoteRolesGeneration:    roles.Generation,
		ExpectedTarget:           roles.ActionHead.Identity,
		ExpectedObservationState: gitremote.ObservationAbsent,
	})
	if err != nil || !pushed.Success {
		t.Fatalf("PushWithExpectation = %+v, err=%v", pushed, err)
	}
	if got := strings.TrimSpace(runGit(t, actionDir, "rev-parse", "refs/heads/Review/Case")); got == "" {
		t.Fatal("action remote did not receive the configured case-sensitive destination ref")
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "--abbrev-ref", "HEAD")); got != "feature/Case" {
		t.Fatalf("local branch changed to %q", got)
	}

	// Remove the explicit upstream while retaining a comparison target. Pull
	// must fail closed instead of using comparison as a convenient fallback.
	runGit(t, repoDir, "config", "--unset", "branch.feature/Case.remote")
	runGit(t, repoDir, "config", "--unset", "branch.feature/Case.merge")
	pulled, err := operator.PullWithExpectation(context.Background(), false, GitMutationExpectation{})
	if err != nil {
		t.Fatalf("PullWithExpectation returned transport error: %v", err)
	}
	if pulled.Success || !strings.Contains(pulled.Error, "tracking_upstream") {
		t.Fatalf("PullWithExpectation = %+v, want missing tracking rejection", pulled)
	}
}

func TestGitOperatorRejectsStaleRoleGenerationBeforePush(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	remoteURL := strings.TrimSpace(runGit(t, repoDir, "remote", "get-url", "origin"))
	fakeURL := "https://github.com/acme/widget.git"
	runGit(t, repoDir, "config", "url."+remoteURL+".insteadOf", fakeURL)
	runGit(t, repoDir, "remote", "set-url", "origin", fakeURL)
	runGit(t, repoDir, "checkout", "-b", "feature/stale")
	runGit(t, repoDir, "config", "branch.feature/stale.pushRemote", "origin")
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	target := gitremote.RemoteRefIdentity{
		Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderGitHub, Host: "github.com", RepositoryPath: "acme/widget"},
		Ref:        "main",
	}
	comparison, err := gitremote.NewComparisonContext(target, "", "stale-context")
	if err != nil {
		t.Fatalf("NewComparisonContext: %v", err)
	}
	tracker.SetComparisonContext(&comparison)
	operator := NewGitOperator(repoDir, newTestLogger(t), tracker)
	roles := tracker.ResolveGitRemoteRoles(context.Background(), nil)
	if roles.ActionHead.Identity == nil {
		t.Fatalf("action role did not resolve: %+v", roles.ActionHead)
	}
	result, err := operator.PushWithExpectation(context.Background(), false, false, GitMutationExpectation{
		RemoteRolesGeneration: "not-the-current-generation",
		ExpectedTarget:        roles.ActionHead.Identity,
	})
	if err != nil {
		t.Fatalf("PushWithExpectation returned transport error: %v", err)
	}
	if result.Success || !strings.Contains(result.Error, "stale") {
		t.Fatalf("PushWithExpectation = %+v, want stale-generation rejection", result)
	}
}

func TestGitOperatorPullRejectsChangedTrackingHead(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	localRemote := strings.TrimSpace(runGit(t, repoDir, "remote", "get-url", "origin"))
	remoteURL := "https://github.com/acme/widget.git"
	runGit(t, repoDir, "config", "url."+localRemote+".insteadOf", remoteURL)
	runGit(t, repoDir, "remote", "set-url", "origin", remoteURL)
	runGit(t, repoDir, "checkout", "-b", "feature/pull-stale")
	runGit(t, repoDir, "config", "branch.feature/pull-stale.remote", "origin")
	runGit(t, repoDir, "config", "branch.feature/pull-stale.merge", "refs/heads/main")

	oldSHA := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/remotes/origin/main"))
	runGit(t, repoDir, "checkout", "-b", "remote-update")
	writeFile(t, repoDir, "remote-change.txt", "remote change\n")
	runGit(t, repoDir, "add", "remote-change.txt")
	runGit(t, repoDir, "commit", "-m", "remote update")
	newSHA := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "push", "origin", "HEAD:refs/heads/main")
	runGit(t, repoDir, "checkout", "feature/pull-stale")
	// Keep the checkout-local tracking ref at the caller's observed head while
	// the authoritative remote has moved, which is the stale-head race.
	runGit(t, repoDir, "update-ref", "refs/remotes/origin/main", oldSHA)
	if newSHA == oldSHA {
		t.Fatal("remote update did not create a new head")
	}

	target := gitremote.RemoteRefIdentity{
		Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderGitHub, Host: "github.com", RepositoryPath: "acme/widget"},
		Ref:        "main",
	}
	comparison, err := gitremote.NewComparisonContext(target, "", "pull-stale-context")
	if err != nil {
		t.Fatalf("NewComparisonContext: %v", err)
	}
	tracker := NewWorkspaceTracker(repoDir, newTestLogger(t))
	tracker.SetComparisonContext(&comparison)
	roles := tracker.ResolveGitRemoteRoles(context.Background(), &target)
	if roles.TrackingUpstream.Identity == nil {
		t.Fatalf("tracking role did not resolve: %+v", roles.TrackingUpstream)
	}

	operator := NewGitOperator(repoDir, newTestLogger(t), tracker)
	result, err := operator.PullWithExpectation(context.Background(), false, GitMutationExpectation{
		RemoteRolesGeneration:    roles.Generation,
		ExpectedTarget:           roles.TrackingUpstream.Identity,
		ExpectedObservationState: gitremote.ObservationPresent,
		ExpectedRemoteHeadCommit: oldSHA,
	})
	if err != nil {
		t.Fatalf("PullWithExpectation returned transport error: %v", err)
	}
	if result.Success || !strings.Contains(result.Error, "stale") {
		t.Fatalf("PullWithExpectation = %+v, want stale-head rejection", result)
	}
	if got := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD")); got == newSHA {
		t.Fatalf("stale Pull advanced local HEAD to %s", got)
	}
}
