package process

import (
	"context"
	"net"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/gitremote"
)

// GitRemoteRole is the executor-local resolution of one independent Git
// role. RemoteName is only useful inside this checkout; callers must use
// Identity for semantic comparisons and mutation authorization.
type GitRemoteRole struct {
	Role        gitremote.RemoteRole
	State       gitremote.ResolutionState
	RemoteName  string
	Identity    *gitremote.RemoteRefIdentity
	Observation gitremote.RemoteRefObservation
}

// GitRemoteRoles is one coherent resolution of the current checkout's Git
// roles. Comparison is optional because its target is supplied by backend
// context rather than inferred from Git remote names.
type GitRemoteRoles struct {
	Branch           string
	Generation       string
	ActionHead       GitRemoteRole
	TrackingUpstream GitRemoteRole
	Comparison       GitRemoteRole
}

type configuredGitRef struct {
	RemoteName string
	Branch     string
}

// ResolveGitRemoteRoles resolves the writable action head, explicit tracking
// upstream, and optional comparison target from the current checkout. It
// never gives a remote name semantic meaning and does not perform network
// observation; callers that need authoritative absent/present evidence can
// use ObserveGitRemoteRef on one of the returned roles.
func (wt *WorkspaceTracker) ResolveGitRemoteRoles(ctx context.Context, comparison *gitremote.RemoteRefIdentity) GitRemoteRoles {
	return wt.resolveGitRemoteRoles(ctx, wt.currentGitBranch(ctx), comparison)
}

// ResolveGitRemoteRolesForBranch is the explicit-branch seam for callers that
// already observed a branch as part of a larger status snapshot. It avoids a
// second symbolic-ref lookup and keeps the role generation bound to that
// caller-supplied branch.
func (wt *WorkspaceTracker) ResolveGitRemoteRolesForBranch(ctx context.Context, branch string, comparison *gitremote.RemoteRefIdentity) GitRemoteRoles {
	return wt.resolveGitRemoteRoles(ctx, branch, comparison)
}

func (wt *WorkspaceTracker) resolveGitRemoteRoles(ctx context.Context, branch string, comparison *gitremote.RemoteRefIdentity) GitRemoteRoles {
	push, upstream := wt.gitConfiguredRefs(ctx, branch)

	action := wt.resolveConfiguredRole(ctx, gitremote.ActionHeadRole, push, true)
	tracking := wt.resolveConfiguredRole(ctx, gitremote.TrackingUpstreamRole, upstream, false)
	comparisonRole := wt.resolveComparisonRole(ctx, comparison)

	input := gitremote.RemoteRolesInput{
		Branch:               branch,
		ActionRemoteName:     action.RemoteName,
		TrackingRemoteName:   tracking.RemoteName,
		ComparisonRemoteName: comparisonRole.RemoteName,
	}
	if action.Identity != nil {
		input.ActionHead = *action.Identity
	}
	if tracking.Identity != nil {
		input.TrackingUpstream = *tracking.Identity
	}
	if comparison != nil {
		// Bind the generation to the complete accepted backend context even
		// when no executor-local remote resolves it. Otherwise changing from
		// one unresolved target to another could leave stale mutation evidence
		// looking current.
		input.ComparisonTarget = *comparison
	} else if comparisonRole.Identity != nil {
		input.ComparisonTarget = *comparisonRole.Identity
	}

	return GitRemoteRoles{
		Branch:           branch,
		Generation:       gitremote.NewGeneration(input),
		ActionHead:       action,
		TrackingUpstream: tracking,
		Comparison:       comparisonRole,
	}
}

// ObserveGitRemoteRef performs an authoritative exact-ref probe for a
// resolved role. The resolver intentionally keeps this separate so routine
// status computation does not unexpectedly perform network I/O. A successful
// empty ls-remote result proves absence; transport/configuration errors remain
// unknown.
func (wt *WorkspaceTracker) ObserveGitRemoteRef(ctx context.Context, role GitRemoteRole) gitremote.RemoteRefObservation {
	observation := role.Observation
	if role.Identity == nil || role.RemoteName == "" || role.State != gitremote.ResolutionResolved {
		return gitremote.RemoteRefObservation{State: gitremote.ObservationUnknown}
	}
	identity := *role.Identity
	observation.Identity = &identity
	observation.State = gitremote.ObservationUnknown
	observation.RemoteHeadCommit = ""
	observation.Ahead = nil
	observation.Behind = nil

	ref := "refs/heads/" + identity.Ref
	output, err := wt.runGitOutput(ctx, "ls-remote", "--heads", role.RemoteName, ref)
	if err != nil {
		return observation
	}
	line := strings.TrimSpace(string(output))
	if line == "" {
		observation.State = gitremote.ObservationAbsent
		return observation
	}
	fields := strings.Fields(line)
	if len(fields) < 1 || fields[0] == "" {
		return observation
	}
	observation.State = gitremote.ObservationPresent
	observation.RemoteHeadCommit = fields[0]
	return observation
}

// getGitHeadRemote retains the additive status projection used by older
// callers. The writable push target wins, with the explicit tracking target
// as a compatibility fallback when no push target is configured.
func (wt *WorkspaceTracker) getGitHeadRemote(ctx context.Context, branch string) *types.GitHeadRemote {
	roles := wt.ResolveGitRemoteRolesForBranch(ctx, branch, nil)
	if roles.ActionHead.State == gitremote.ResolutionResolved {
		return projectGitHeadRemote(roles.ActionHead.Identity)
	}
	if roles.TrackingUpstream.State == gitremote.ResolutionResolved {
		return projectGitHeadRemote(roles.TrackingUpstream.Identity)
	}
	return nil
}

func (wt *WorkspaceTracker) currentGitBranch(ctx context.Context) string {
	output, err := wt.runGitOutput(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (wt *WorkspaceTracker) gitConfiguredRefs(ctx context.Context, branch string) (configuredGitRef, configuredGitRef) {
	if branch == "" {
		return configuredGitRef{}, configuredGitRef{}
	}
	ref := "refs/heads/" + branch
	output, err := wt.runGitOutput(ctx, "for-each-ref", "--format=%(push:remotename)\t%(push:remoteref)\t%(upstream:remotename)\t%(upstream:remoteref)", ref)
	if err != nil {
		return configuredGitRef{}, configuredGitRef{}
	}
	parts := strings.Split(strings.TrimRight(string(output), "\r\n"), "\t")
	if len(parts) < 4 {
		return configuredGitRef{}, configuredGitRef{}
	}

	push := configuredGitRef{
		RemoteName: strings.TrimSpace(parts[0]),
		Branch:     remoteBranchName(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])),
	}
	if push.RemoteName != "" && push.Branch == "" {
		push.Branch = branch
	}
	upstream := configuredGitRef{
		RemoteName: strings.TrimSpace(parts[2]),
		Branch:     remoteBranchName(strings.TrimSpace(parts[2]), strings.TrimSpace(parts[3])),
	}
	if upstream.RemoteName != "" && upstream.Branch == "" {
		upstream = configuredGitRef{}
	}
	return push, upstream
}

// gitBranchRemote is kept for package-local compatibility with status code
// and tests. Its result follows Git's push target first, then upstream.
func (wt *WorkspaceTracker) gitBranchRemote(ctx context.Context, branch string) (string, string) {
	push, upstream := wt.gitConfiguredRefs(ctx, branch)
	if push.RemoteName != "" && push.Branch != "" {
		return push.RemoteName, push.Branch
	}
	if upstream.RemoteName != "" && upstream.Branch != "" {
		return upstream.RemoteName, upstream.Branch
	}
	return "", ""
}

func (wt *WorkspaceTracker) resolveConfiguredRole(ctx context.Context, role gitremote.RemoteRole, ref configuredGitRef, usePushURL bool) GitRemoteRole {
	result := GitRemoteRole{Role: role, State: gitremote.ResolutionUnresolved, RemoteName: ref.RemoteName}
	if ref.RemoteName == "" || ref.Branch == "" {
		return result
	}

	urls := wt.gitRemoteConfigValues(ctx, ref.RemoteName, "url")
	if usePushURL {
		if pushURLs := wt.gitRemoteConfigValues(ctx, ref.RemoteName, "pushurl"); len(pushURLs) > 0 {
			urls = pushURLs
		}
	}
	identity, state := parseRemoteRepositoryIdentities(urls)
	if state != gitremote.ResolutionResolved {
		result.State = state
		return result
	}

	resolved := gitremote.RemoteRefIdentity{Repository: *identity, Ref: ref.Branch}
	result.State = gitremote.ResolutionResolved
	result.Identity = &resolved
	result.Observation = gitremote.RemoteRefObservation{
		Identity: &resolved,
		State:    gitremote.ObservationUnknown,
	}
	return result
}

func (wt *WorkspaceTracker) resolveComparisonRole(ctx context.Context, target *gitremote.RemoteRefIdentity) GitRemoteRole {
	result := GitRemoteRole{Role: gitremote.ComparisonTargetRole, State: gitremote.ResolutionUnresolved}
	if target == nil || target.Ref == "" || target.Repository.Host == "" || (target.Repository.RepositoryPath == "" && target.Repository.ProviderRepositoryID == "") {
		return result
	}

	output, err := wt.runGitOutput(ctx, "remote")
	if err != nil {
		return result
	}
	var match configuredGitRef
	matchCount := 0
	for _, name := range strings.Fields(string(output)) {
		urls := wt.gitRemoteConfigValues(ctx, name, "url")
		if len(urls) == 0 {
			urls = wt.gitRemoteConfigValues(ctx, name, "pushurl")
		}
		identity, state := parseRemoteRepositoryIdentities(urls)
		if state == gitremote.ResolutionAmbiguous {
			// A conflicting URL on a remote that could be the target cannot be
			// safely ignored. Preserve ambiguity until a fresh configuration is
			// observed.
			if remoteIdentityCouldMatch(urls, *target) {
				return GitRemoteRole{Role: gitremote.ComparisonTargetRole, State: gitremote.ResolutionAmbiguous}
			}
			continue
		}
		if state != gitremote.ResolutionResolved || !identityMatches(*identity, target.Repository) {
			continue
		}
		matchCount++
		match = configuredGitRef{RemoteName: name, Branch: target.Ref}
	}

	switch matchCount {
	case 0:
		return result
	case 1:
		resolved := gitremote.RemoteRefIdentity{Repository: target.Repository, Ref: target.Ref}
		result.RemoteName = match.RemoteName
		result.State = gitremote.ResolutionResolved
		result.Identity = &resolved
		result.Observation = gitremote.RemoteRefObservation{Identity: &resolved, State: gitremote.ObservationUnknown}
		return result
	default:
		return GitRemoteRole{Role: gitremote.ComparisonTargetRole, State: gitremote.ResolutionAmbiguous}
	}
}

func identityMatches(left, right gitremote.RemoteRepositoryIdentity) bool {
	return left.EqualRepository(right)
}

func remoteIdentityCouldMatch(urls []string, target gitremote.RemoteRefIdentity) bool {
	for _, remoteURL := range urls {
		identity, ok := parseRemoteRepositoryIdentity(remoteURL)
		if ok && identityMatches(identity, target.Repository) {
			return true
		}
	}
	return false
}

func parseRemoteRepositoryIdentities(urls []string) (*gitremote.RemoteRepositoryIdentity, gitremote.ResolutionState) {
	if len(urls) == 0 {
		return nil, gitremote.ResolutionUnresolved
	}
	var identity *gitremote.RemoteRepositoryIdentity
	invalidCount := 0
	for _, remoteURL := range urls {
		candidate, ok := parseRemoteRepositoryIdentity(remoteURL)
		if !ok {
			invalidCount++
			continue
		}
		if identity == nil {
			identity = &candidate
			continue
		}
		if !identity.EqualRepository(candidate) {
			return nil, gitremote.ResolutionAmbiguous
		}
	}
	if identity == nil {
		return nil, gitremote.ResolutionUnresolved
	}
	if invalidCount > 0 {
		return nil, gitremote.ResolutionAmbiguous
	}
	return identity, gitremote.ResolutionResolved
}

func parseRemoteRepositoryIdentity(remoteURL string) (gitremote.RemoteRepositoryIdentity, bool) {
	host := remoteIdentityHost(remoteURL)
	if host == "" {
		return gitremote.RemoteRepositoryIdentity{}, false
	}
	provider := gitremote.Provider(detectPRProvider(remoteURL))
	segments := splitRemotePath(remoteRepositoryPath(remoteURL))
	if len(segments) == 0 {
		return gitremote.RemoteRepositoryIdentity{}, false
	}
	for i := range segments {
		segments[i] = strings.TrimSpace(segments[i])
	}
	if segments[len(segments)-1] == "" {
		return gitremote.RemoteRepositoryIdentity{}, false
	}
	segments[len(segments)-1] = trimGitSuffix(segments[len(segments)-1])

	path := strings.Join(segments, "/")
	switch provider {
	case gitremote.ProviderGitHub:
		// GitHub's adapter owns owner/repository splitting. Keep the complete
		// path here so the shared identity remains provider-neutral.
		if len(segments) < 2 {
			return gitremote.RemoteRepositoryIdentity{}, false
		}
	case gitremote.ProviderAzureRepos:
		path = normalizeAzureRepositoryPath(host, segments)
		if path == "" {
			return gitremote.RemoteRepositoryIdentity{}, false
		}
	}

	return gitremote.RemoteRepositoryIdentity{
		Provider:       provider,
		Host:           host,
		RepositoryPath: path,
	}, true
}

// remoteIdentityHost preserves an explicit non-default HTTP(S) port because
// it is part of a self-hosted provider's repository identity. SSH transport
// ports are intentionally ignored: provider authorization is based on the
// web host that owns the repository, matching the existing provider adapter
// normalization.
func remoteIdentityHost(remoteURL string) string {
	trimmed := strings.TrimSpace(remoteURL)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err != nil || parsed.Hostname() == "" {
			return ""
		}
		host := strings.ToLower(parsed.Hostname())
		if strings.EqualFold(parsed.Scheme, "ssh") {
			return host
		}
		port := parsed.Port()
		if port == "" || (strings.EqualFold(parsed.Scheme, "http") && port == "80") || (strings.EqualFold(parsed.Scheme, "https") && port == "443") {
			return host
		}
		return net.JoinHostPort(host, port)
	}
	if _, after, ok := strings.Cut(trimmed, "@"); ok {
		trimmed = after
	}
	if host, _, ok := strings.Cut(trimmed, ":"); ok {
		if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
			return strings.ToLower(strings.Trim(host, "[]"))
		}
		return strings.ToLower(host)
	}
	return remoteHostFromURL(trimmed)
}

func normalizeAzureRepositoryPath(host string, segments []string) string {
	if strings.HasSuffix(host, ".visualstudio.com") {
		if len(segments) < 3 || !strings.EqualFold(segments[1], "_git") {
			return ""
		}
		organization := strings.TrimSuffix(host, ".visualstudio.com")
		if organization == "" {
			return ""
		}
		return strings.Join([]string{organization, segments[0], trimGitSuffix(segments[2])}, "/")
	}
	if host == "dev.azure.com" {
		if len(segments) < 4 || !strings.EqualFold(segments[2], "_git") {
			return ""
		}
		return strings.Join([]string{segments[0], segments[1], trimGitSuffix(segments[3])}, "/")
	}
	if host == "ssh.dev.azure.com" {
		if len(segments) < 4 || !strings.EqualFold(segments[0], "v3") {
			return ""
		}
		return strings.Join([]string{segments[1], segments[2], trimGitSuffix(segments[3])}, "/")
	}
	return ""
}

func projectGitHeadRemote(identity *gitremote.RemoteRefIdentity) *types.GitHeadRemote {
	if identity == nil {
		return nil
	}
	projected := &types.GitHeadRemote{
		Provider:             string(identity.Repository.Provider),
		Host:                 identity.Repository.Host,
		Branch:               identity.Ref,
		RepositoryPath:       identity.Repository.RepositoryPath,
		ProviderRepositoryID: identity.Repository.ProviderRepositoryID,
	}
	if identity.Repository.Provider == gitremote.ProviderGitHub {
		parts := splitRemotePath(identity.Repository.RepositoryPath)
		if len(parts) >= 2 {
			projected.Owner = parts[len(parts)-2]
			projected.Repo = parts[len(parts)-1]
		}
	}
	return projected
}

// parseGitHeadRemote is the compatibility adapter for callers that still
// receive a normalized GitHub status projection. All URL parsing now goes
// through the provider-neutral identity resolver above.
func parseGitHeadRemote(remoteURL, branch string) (*types.GitHeadRemote, bool) {
	identity, ok := parseRemoteRepositoryIdentity(remoteURL)
	if !ok || identity.Provider != gitremote.ProviderGitHub {
		return nil, false
	}
	resolved := gitremote.RemoteRefIdentity{Repository: identity, Ref: branch}
	return projectGitHeadRemote(&resolved), true
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

func remoteBranchName(remoteName, remoteRef string) string {
	remoteName = strings.TrimSpace(remoteName)
	remoteRef = strings.TrimSpace(remoteRef)
	for _, prefix := range []string{
		"refs/heads/",
		"refs/remotes/" + remoteName + "/",
	} {
		if branch, ok := strings.CutPrefix(remoteRef, prefix); ok {
			return branch
		}
	}
	return remoteRef
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
