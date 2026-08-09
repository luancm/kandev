package process

import (
	"context"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// getGitHeadRemote resolves the exact remote repository and branch that Git
// considers the current branch's push target. A branch's push target may be
// different from its upstream tracking target, so the latter is only a
// fallback. Invalid or conflicting configuration is deliberately treated as
// absent rather than selecting an unsafe candidate.
func (wt *WorkspaceTracker) getGitHeadRemote(ctx context.Context, branch string) *types.GitHeadRemote {
	remoteName, remoteBranch := wt.gitBranchRemote(ctx, branch)
	if remoteName == "" || remoteBranch == "" {
		return nil
	}

	urls := wt.gitRemoteConfigValues(ctx, remoteName, "pushurl")
	if len(urls) == 0 {
		urls = wt.gitRemoteConfigValues(ctx, remoteName, "url")
	}
	if len(urls) == 0 {
		return nil
	}

	var identity *types.GitHeadRemote
	for _, remoteURL := range urls {
		candidate, ok := parseGitHeadRemote(remoteURL, remoteBranch)
		if !ok {
			return nil
		}
		if identity == nil {
			identity = candidate
			continue
		}
		if *identity != *candidate {
			return nil
		}
	}
	return identity
}

func (wt *WorkspaceTracker) gitBranchRemote(ctx context.Context, branch string) (string, string) {
	ref := "refs/heads/" + branch
	out, err := wt.runGitOutput(ctx, "for-each-ref", "--format=%(push:remotename)\t%(push:remoteref)\t%(upstream:remotename)\t%(upstream:remoteref)", ref)
	if err != nil {
		return "", ""
	}
	parts := strings.Split(strings.TrimRight(string(out), "\r\n"), "\t")
	if len(parts) < 4 {
		return "", ""
	}
	remoteName, remoteRef := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if remoteName != "" && remoteRef == "" {
		// Git can resolve the configured push remote while leaving the
		// remote ref empty when the destination branch has not been fetched
		// yet. The default push mapping uses the local branch name.
		return remoteName, branch
	}
	if remoteName == "" || remoteRef == "" {
		remoteName, remoteRef = strings.TrimSpace(parts[2]), strings.TrimSpace(parts[3])
	}
	if remoteName == "" || remoteRef == "" {
		return "", ""
	}
	return remoteName, remoteBranchName(remoteName, remoteRef)
}

func remoteBranchName(remoteName, remoteRef string) string {
	for _, prefix := range []string{
		"refs/heads/",
		"refs/remotes/" + remoteName + "/",
	} {
		if branch, ok := strings.CutPrefix(remoteRef, prefix); ok {
			return branch
		}
	}
	return strings.TrimSpace(remoteRef)
}

func (wt *WorkspaceTracker) gitRemoteConfigValues(ctx context.Context, remoteName, key string) []string {
	out, err := wt.runGitOutput(ctx, "config", "--get-all", "remote."+remoteName+"."+key)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		if value := strings.TrimSpace(line); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func parseGitHeadRemote(remoteURL, branch string) (*types.GitHeadRemote, bool) {
	host := remoteHostFromURL(remoteURL)
	if host == "" || detectPRProvider(remoteURL) != prProviderGitHub {
		return nil, false
	}
	path := remoteRepositoryPath(remoteURL)
	parts := splitRemotePath(path)
	if len(parts) < 2 {
		return nil, false
	}
	return &types.GitHeadRemote{
		Provider: string(prProviderGitHub),
		Host:     host,
		Owner:    parts[len(parts)-2],
		Repo:     trimGitSuffix(parts[len(parts)-1]),
		Branch:   branch,
	}, true
}

func remoteRepositoryPath(remoteURL string) string {
	trimmed := strings.TrimSpace(remoteURL)
	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			return parsed.Path
		}
	}
	if _, after, ok := strings.Cut(trimmed, "@"); ok {
		trimmed = after
	}
	if _, after, ok := strings.Cut(trimmed, ":"); ok {
		return after
	}
	return trimmed
}
