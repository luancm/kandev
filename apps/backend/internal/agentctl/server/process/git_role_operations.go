package process

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/gitremote"
	"go.uber.org/zap"
)

// GitMutationExpectation is the caller's observation of the role it is about
// to mutate. It is deliberately credential-free and is checked again while
// GitOperator's operation lock is held. Empty fields retain compatibility
// with older clients; when a comparison context is delivered, the role and
// identity are still resolved from the current checkout rather than guessed.
type GitMutationExpectation struct {
	RemoteRolesGeneration        string
	ExpectedTarget               *gitremote.RemoteRefIdentity
	ExpectedObservationState     gitremote.ObservationState
	ExpectedRemoteHeadCommit     string
	ExpectedComparisonGeneration string
	ExpectedSource               *gitremote.RemoteRefIdentity
	ExpectedBase                 *gitremote.RemoteRefIdentity
}

type gitProviderRoute struct {
	Source           *gitremote.RemoteRefIdentity
	Base             *gitremote.RemoteRefIdentity
	SourceRemoteName string
	BaseRemoteName   string
}

var errGitRemoteRoleStale = errors.New("git remote role observation is stale; refresh git status and retry")

const comparisonRebaseOperation = "rebase"

func (g *GitOperator) roleRoutingActive(expected GitMutationExpectation) bool {
	return g.workspaceTracker != nil && (g.workspaceTracker.HasComparisonContext() || expected.RemoteRolesGeneration != "")
}

func (g *GitOperator) resolvePullTarget(ctx context.Context, branch string, expected GitMutationExpectation) (string, string, error) {
	if g.roleRoutingActive(expected) {
		role, _, err := g.resolveRoleForMutation(ctx, gitremote.TrackingUpstreamRole, expected, true)
		if err != nil {
			return "", "", err
		}
		return role.RemoteName, role.Identity.Ref, nil
	}
	if g.remoteContribution != nil {
		if err := g.validateContributionRemote(ctx); err != nil {
			return "", "", err
		}
		return g.remoteContribution.ContributionRemoteName(), g.remoteContribution.HeadBranch, nil
	}
	if g.getUpstreamRef(ctx) == "" {
		if defaultBranch := g.getDefaultRemoteBranch(ctx); defaultBranch != "" {
			branch = defaultBranch
		}
	}
	return "origin", branch, nil
}

func (g *GitOperator) resolvePushTarget(ctx context.Context, branch string, setUpstream bool, expected GitMutationExpectation) (string, string, bool, error) {
	shouldSetUpstream := setUpstream || g.getUpstreamRef(ctx) == ""
	if g.roleRoutingActive(expected) {
		role, observation, err := g.resolveRoleForMutation(ctx, gitremote.ActionHeadRole, expected, false)
		if err != nil {
			return "", "", false, err
		}
		if observation.State == gitremote.ObservationUnknown {
			return "", "", false, fmt.Errorf("action_head remote observation is unknown; refresh git status before pushing")
		}
		return role.RemoteName, "HEAD:refs/heads/" + role.Identity.Ref, setUpstream, nil
	}
	if g.remoteContribution != nil {
		return g.remoteContribution.ContributionRemoteName(), "HEAD:refs/heads/" + g.remoteContribution.HeadBranch, setUpstream, nil
	}
	return "origin", branch, shouldSetUpstream, nil
}

func (g *GitOperator) runComparisonMutation(ctx context.Context, operation string, expected GitMutationExpectation) (*GitOperationResult, bool) {
	if !g.roleRoutingActive(expected) {
		return nil, false
	}
	target, generation, delivered := g.deliveredComparisonTarget()
	if !delivered {
		if expected.RemoteRolesGeneration == "" {
			return nil, false
		}
		return &GitOperationResult{Operation: operation, Error: "comparison target was not delivered; refresh comparison status before " + comparisonOperationGerund(operation)}, true
	}
	role, observation, err := g.resolveRoleForMutation(ctx, gitremote.ComparisonTargetRole, expected, false)
	if err != nil {
		return &GitOperationResult{Operation: operation, Error: err.Error()}, true
	}
	if err := g.validateComparisonExpectation(expected, generation, target); err != nil {
		return &GitOperationResult{Operation: operation, Error: err.Error()}, true
	}
	if observation.State != gitremote.ObservationPresent {
		return &GitOperationResult{
			Operation: operation,
			Error:     fmt.Sprintf("comparison_target remote observation is %s; refresh comparison status before %s", observation.State, comparisonOperationGerund(operation)),
		}, true
	}
	fetchOutput, fetchErr := g.runGitCommand(ctx, "fetch", role.RemoteName, role.Identity.Ref)
	if fetchErr != nil {
		return &GitOperationResult{
			Operation: operation,
			Output:    fetchOutput,
			Error:     fmt.Sprintf("failed to fetch comparison target: %s", fetchErr.Error()),
		}, true
	}
	operationOutput, operationErr := g.runGitCommand(ctx, operation, role.RemoteName+"/"+role.Identity.Ref)
	result := &GitOperationResult{Operation: operation, Output: fetchOutput + operationOutput}
	if operationErr != nil {
		result.Error = operationErr.Error()
		result.ConflictFiles = g.parseConflictFiles(operationOutput)
		if operation == comparisonRebaseOperation && len(result.ConflictFiles) > 0 {
			if _, abortErr := g.runGitCommand(ctx, comparisonRebaseOperation, "--abort"); abortErr != nil {
				g.logger.Warn("failed to abort rebase", zap.Error(abortErr))
			}
		}
		return result, true
	}
	result.Success = true
	return result, true
}

func comparisonOperationGerund(operation string) string {
	if operation == comparisonRebaseOperation {
		return "rebasing"
	}
	return "merging"
}

func (g *GitOperator) pushPRBranch(ctx context.Context, route *gitProviderRoute) (string, error) {
	args := []string{"push", "--set-upstream", "origin", "HEAD"}
	if route != nil {
		args = []string{"push", "--set-upstream", route.SourceRemoteName, "HEAD:refs/heads/" + route.Source.Ref}
	}
	return g.runGitCommand(ctx, args...)
}

func (g *GitOperator) resolvePRBranchAndRemote(ctx context.Context, branch, remoteURL string) (string, string, error) {
	if branch == "" {
		var err error
		branch, err = g.getCurrentBranch(ctx)
		if err != nil {
			return "", "", fmt.Errorf("failed to get current branch: %s", err.Error())
		}
	}
	if remoteURL == "" {
		var err error
		remoteURL, err = g.getOriginRemoteURL(ctx)
		if err != nil {
			return "", "", err
		}
	}
	return branch, remoteURL, nil
}

func (g *GitOperator) resolvePRRoute(ctx context.Context, expected GitMutationExpectation) (*gitProviderRoute, string, string, string, error) {
	if !g.roleRoutingActive(expected) {
		return nil, "", "", "", nil
	}
	target, generation, delivered := g.deliveredComparisonTarget()
	if !delivered {
		if expected.RemoteRolesGeneration == "" {
			return nil, "", "", "", nil
		}
		return nil, "", "", "", errors.New("comparison target was not delivered; refresh comparison status before creating a change request")
	}
	if target == nil {
		return nil, "", "", "", errors.New("comparison target is unresolved; refresh comparison status before creating a change request")
	}
	roles, err := g.roleSnapshot(ctx)
	if err != nil {
		return nil, "", "", "", err
	}
	if err := g.validatePRRoleSnapshot(roles, target, generation, expected); err != nil {
		return nil, "", "", "", err
	}
	observation := g.workspaceTracker.ObserveGitRemoteRef(ctx, roles.ActionHead)
	if err := validateExpectedObservation(observation, expected); err != nil {
		return nil, "", "", "", err
	}
	if observation.State != gitremote.ObservationPresent {
		return nil, "", "", "", errors.New("action_head remote observation is unknown or absent; refresh git status before creating a change request")
	}
	actionURL, err := g.remoteURLForRole(ctx, roles.ActionHead.RemoteName, true)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("failed to resolve action_head remote: %v", err)
	}
	comparisonURL, err := g.remoteURLForRole(ctx, roles.Comparison.RemoteName, false)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("failed to resolve comparison_target remote: %v", err)
	}
	source := *roles.ActionHead.Identity
	base := *roles.Comparison.Identity
	return &gitProviderRoute{
		Source:           &source,
		Base:             &base,
		SourceRemoteName: roles.ActionHead.RemoteName,
		BaseRemoteName:   roles.Comparison.RemoteName,
	}, actionURL, comparisonURL, source.Ref, nil
}

func (g *GitOperator) validatePRRoleSnapshot(roles GitRemoteRoles, target *gitremote.RemoteRefIdentity, generation string, expected GitMutationExpectation) error {
	if err := validateExpectedGeneration(roles, expected); err != nil {
		return err
	}
	comparisonExpected := expected
	// Create PR carries the source and base independently. During the additive
	// wire rollout the generic target may still be the source (the frontend
	// uses it for Push/Create PR), so the explicit base is authoritative for
	// comparison validation when present.
	if expected.ExpectedBase != nil {
		comparisonExpected.ExpectedTarget = expected.ExpectedBase
	} else if expected.ExpectedSource != nil {
		comparisonExpected.ExpectedTarget = nil
	}
	if err := g.validateComparisonExpectation(comparisonExpected, generation, target); err != nil {
		return err
	}
	if roles.ActionHead.State != gitremote.ResolutionResolved || roles.ActionHead.Identity == nil {
		return errors.New("action_head remote role is unresolved; refresh git status before creating a change request")
	}
	if roles.Comparison.State != gitremote.ResolutionResolved || roles.Comparison.Identity == nil {
		return errors.New("comparison_target remote role is unresolved; configure the delivered target remote before creating a change request")
	}
	if err := validateExpectedIdentity(roles.ActionHead.Identity, expected.ExpectedSource); err != nil {
		return err
	}
	return validateExpectedIdentity(roles.Comparison.Identity, expected.ExpectedBase)
}

func (g *GitOperator) preparePRProvider(remoteURL, targetRemoteURL string, route *gitProviderRoute) (prProvider, *gitLabRepoInfo, error) {
	provider := g.detectPRProvider(remoteURL)
	if route != nil && (route.Source == nil || route.Base == nil ||
		route.Source.Repository.Provider != route.Base.Repository.Provider ||
		!strings.EqualFold(route.Source.Repository.Host, route.Base.Repository.Host)) {
		return "", nil, errors.New("source and comparison target must use the same provider host")
	}
	if route != nil && provider != prProvider(route.Source.Repository.Provider) {
		return "", nil, errors.New("action remote provider does not match the source identity")
	}
	if provider == prProviderAzureRepos {
		if _, err := parseAzureRepoInfo(remoteURL); err != nil {
			return "", nil, err
		}
		if route != nil {
			if _, err := parseAzureRepoInfo(targetRemoteURL); err != nil {
				return "", nil, err
			}
		}
		return provider, nil, nil
	}
	if provider == prProviderGitHub {
		return provider, nil, nil
	}
	if provider == prProviderGitLab {
		gitlabURL := remoteURL
		if route != nil {
			gitlabURL = targetRemoteURL
		}
		info, err := parseGitLabRepoInfo(gitlabURL, g.environmentValue(gitLabHostEnv))
		return provider, info, err
	}
	return "", nil, fmt.Errorf("unsupported git remote for PR creation: %s (GitHub, GitLab, and Azure Repos are supported)", redactRemoteURL(remoteURL))
}

func (g *GitOperator) updateRemoteContributionForPR(ctx context.Context, result *PRCreateResult, title, body string, route *gitProviderRoute) bool {
	if route != nil || g.remoteContribution == nil {
		return false
	}
	if err := g.validateContributionRemote(ctx); err != nil {
		result.Error = err.Error()
		return true
	}
	branch, err := g.getCurrentBranch(ctx)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get current branch: %s", err.Error())
		return true
	}
	output, err := g.runGitCommand(ctx, "push", g.remoteContribution.ContributionRemoteName(), "HEAD:refs/heads/"+g.remoteContribution.HeadBranch)
	if err != nil {
		result.Error = fmt.Sprintf("failed to push contribution branch: %s", g.sanitizePRFailure(output, title, body))
		result.Output = g.sanitizeGitPushOutput(output)
		return true
	}
	result.Success = true
	result.BranchPushed = true
	result.PRURL = g.remoteContribution.CanonicalURL
	result.Provider = g.remoteContribution.Provider
	result.Output = g.sanitizeGitPushOutput(output)
	g.logger.Info("updated existing remote contribution", zap.String("branch", branch), zap.String("provider", result.Provider))
	return true
}

func (g *GitOperator) deliveredComparisonTarget() (*gitremote.RemoteRefIdentity, string, bool) {
	if g.workspaceTracker == nil || !g.workspaceTracker.HasComparisonContext() {
		return nil, "", false
	}
	comparison := g.workspaceTracker.ComparisonContext()
	if comparison == nil {
		return nil, "", true
	}
	return comparison.Target, comparison.ContextGeneration, true
}

func (g *GitOperator) roleSnapshot(ctx context.Context) (GitRemoteRoles, error) {
	if g.workspaceTracker == nil {
		return GitRemoteRoles{}, nil
	}
	target, _, _ := g.deliveredComparisonTarget()
	branch, err := g.getCurrentBranch(ctx)
	if err != nil {
		return GitRemoteRoles{}, err
	}
	return g.workspaceTracker.ResolveGitRemoteRolesForBranch(ctx, branch, target), nil
}

func observationForRole(ctx context.Context, tracker *WorkspaceTracker, role GitRemoteRole) gitremote.RemoteRefObservation {
	if tracker == nil {
		return role.Observation
	}
	// Mutations are serialized by GitOperator's operation lock. Refresh the
	// exact remote ref at that boundary so an expected present head cannot be
	// reused after the provider moved it, and an expected absent ref cannot be
	// mistaken for a first-push destination after it appeared.
	return tracker.ObserveGitRemoteRef(ctx, role)
}

func validateExpectedGeneration(roles GitRemoteRoles, expected GitMutationExpectation) error {
	if expected.RemoteRolesGeneration != "" && expected.RemoteRolesGeneration != roles.Generation {
		return errGitRemoteRoleStale
	}
	return nil
}

func validateExpectedIdentity(identity *gitremote.RemoteRefIdentity, expected *gitremote.RemoteRefIdentity) error {
	if expected == nil {
		return nil
	}
	if identity == nil || !identity.Equal(*expected) {
		return errGitRemoteRoleStale
	}
	return nil
}

func validateExpectedObservation(observation gitremote.RemoteRefObservation, expected GitMutationExpectation) error {
	if expected.ExpectedObservationState != "" && observation.State != expected.ExpectedObservationState {
		return errGitRemoteRoleStale
	}
	if expected.ExpectedRemoteHeadCommit != "" && observation.RemoteHeadCommit != expected.ExpectedRemoteHeadCommit {
		return errGitRemoteRoleStale
	}
	return nil
}

func (g *GitOperator) resolveRoleForMutation(ctx context.Context, role gitremote.RemoteRole, expected GitMutationExpectation, requirePresent bool) (GitRemoteRole, gitremote.RemoteRefObservation, error) {
	roles, err := g.roleSnapshot(ctx)
	if err != nil {
		return GitRemoteRole{}, gitremote.RemoteRefObservation{}, err
	}
	if g.workspaceTracker == nil {
		return GitRemoteRole{}, gitremote.RemoteRefObservation{}, nil
	}
	if err := validateExpectedGeneration(roles, expected); err != nil {
		return GitRemoteRole{}, gitremote.RemoteRefObservation{}, err
	}
	selected := roles.ActionHead
	switch role {
	case gitremote.TrackingUpstreamRole:
		selected = roles.TrackingUpstream
	case gitremote.ComparisonTargetRole:
		selected = roles.Comparison
	}
	if selected.State != gitremote.ResolutionResolved || selected.Identity == nil || selected.RemoteName == "" {
		return GitRemoteRole{}, gitremote.RemoteRefObservation{}, fmt.Errorf("%s remote role is unresolved; refresh git status and configure an exact remote", role)
	}
	if err := validateExpectedIdentity(selected.Identity, expected.ExpectedTarget); err != nil {
		return GitRemoteRole{}, gitremote.RemoteRefObservation{}, err
	}
	observation := observationForRole(ctx, g.workspaceTracker, selected)
	if expected.ExpectedObservationState != "" || expected.ExpectedRemoteHeadCommit != "" {
		if err := validateExpectedObservation(observation, expected); err != nil {
			return GitRemoteRole{}, gitremote.RemoteRefObservation{}, err
		}
	}
	if requirePresent && observation.State != gitremote.ObservationPresent {
		return GitRemoteRole{}, gitremote.RemoteRefObservation{}, fmt.Errorf("%s remote observation is %s; refresh git status before mutating", role, observation.State)
	}
	return selected, observation, nil
}

func (g *GitOperator) validateComparisonExpectation(expected GitMutationExpectation, generation string, target *gitremote.RemoteRefIdentity) error {
	if expected.ExpectedComparisonGeneration != "" && expected.ExpectedComparisonGeneration != generation {
		return errGitRemoteRoleStale
	}
	return validateExpectedIdentity(target, expected.ExpectedTarget)
}

func (g *GitOperator) remoteURLForRole(ctx context.Context, remoteName string, push bool) (string, error) {
	args := []string{"remote", "get-url"}
	if push {
		args = append(args, "--push")
	}
	args = append(args, remoteName)
	output, err := g.runGitCommand(ctx, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}
