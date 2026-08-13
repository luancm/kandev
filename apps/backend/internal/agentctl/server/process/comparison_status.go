package process

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/gitremote"
)

type comparisonResolution struct {
	status *types.GitComparisonStatus
	ref    string
}

// resolveComparisonStatus consumes the complete backend-owned context and the
// executor-local remote-role resolver. It deliberately has no integration
// branch fallback: a target that cannot be resolved may use only its
// identity-qualified stored anchor, then the current branch tip as the safe
// read-only fallback.
func (wt *WorkspaceTracker) resolveComparisonStatus(
	ctx context.Context,
	branch, headCommit string,
) comparisonResolution {
	status := &types.GitComparisonStatus{State: gitremote.ResolutionUnresolved}
	comparison := wt.ComparisonContext()
	if comparison == nil {
		status.Reason = "comparison context is cleared"
		status.BaseCommit = headCommit
		return comparisonResolution{status: status}
	}

	status.ContextGeneration = comparison.ContextGeneration
	if comparison.Target != nil {
		target := *comparison.Target
		status.Target = &target
	}
	if comparison.Target == nil {
		status.Reason = "comparison target is unavailable"
		status.BaseCommit = comparisonStoredOrHead(comparison.StoredBaseCommit, headCommit)
		return comparisonResolution{status: status}
	}

	roles := wt.ResolveGitRemoteRolesForBranch(ctx, branch, comparison.Target)
	status.State = roles.Comparison.State
	if roles.Comparison.State == gitremote.ResolutionAmbiguous {
		status.Reason = "multiple comparison remotes match the target identity"
		status.BaseCommit = comparisonStoredOrHead(comparison.StoredBaseCommit, headCommit)
		return comparisonResolution{status: status}
	}
	if roles.Comparison.State != gitremote.ResolutionResolved || roles.Comparison.Identity == nil {
		status.State = gitremote.ResolutionUnresolved
		status.Reason = "no configured remote matches the comparison target"
		status.BaseCommit = comparisonStoredOrHead(comparison.StoredBaseCommit, headCommit)
		return comparisonResolution{status: status}
	}

	ref := roles.Comparison.RemoteName + "/" + comparison.Target.Ref
	if !IsSafeGitRef(ref) {
		status.State = gitremote.ResolutionUnresolved
		status.Reason = "comparison remote ref is invalid"
		status.BaseCommit = comparisonStoredOrHead(comparison.StoredBaseCommit, headCommit)
		return comparisonResolution{status: status}
	}
	if _, err := wt.runGitOutput(ctx, "rev-parse", "--verify", ref+"^{commit}"); err != nil {
		status.State = gitremote.ResolutionUnresolved
		status.Reason = "comparison ref is not available locally"
		status.BaseCommit = comparisonStoredOrHead(comparison.StoredBaseCommit, headCommit)
		return comparisonResolution{status: status}
	}

	mergeBaseOutput, err := wt.runGitOutput(ctx, "merge-base", "HEAD", ref)
	mergeBase := strings.TrimSpace(string(mergeBaseOutput))
	if err != nil || !sha1HexPattern.MatchString(mergeBase) {
		status.State = gitremote.ResolutionUnresolved
		status.Reason = "comparison ref has no usable merge-base"
		status.ResolvedRef = ref
		status.BaseCommit = comparisonStoredOrHead(comparison.StoredBaseCommit, headCommit)
		return comparisonResolution{status: status, ref: ref}
	}

	status.State = gitremote.ResolutionResolved
	status.ResolvedRef = ref
	status.Reason = "comparison target resolved"
	// A resolved target is authoritative for this poll. StoredBaseCommit is
	// only a safe fallback while the target is unresolved; preferring it here
	// can make newly landed canonical commits appear as deletions.
	status.BaseCommit = mergeBase
	return comparisonResolution{status: status, ref: ref}
}

func comparisonStoredOrHead(stored, head string) string {
	if sha1HexPattern.MatchString(stored) {
		return stored
	}
	return head
}

func (wt *WorkspaceTracker) applyComparisonStatus(
	update *types.GitStatusUpdate,
	resolution comparisonResolution,
) {
	update.Comparison = resolution.status
	update.BaseCommit = resolution.status.BaseCommit
}

// finalizeComparisonStatus projects the counts computed from the selected
// comparison ref into the nullable structured evidence. An unresolved target
// may retain its identity-qualified base anchor, but its live counts remain
// unknown; the compatibility scalar fields are cleared so a previous
// origin-based snapshot cannot survive a context change.
func (wt *WorkspaceTracker) finalizeComparisonStatus(update *types.GitStatusUpdate) {
	if update.Comparison == nil {
		return
	}
	if update.Comparison.State != gitremote.ResolutionResolved {
		update.Ahead = 0
		update.Behind = 0
		update.BranchAdditions = 0
		update.BranchDeletions = 0
		return
	}

	ahead, behind := update.Ahead, update.Behind
	additions, deletions := update.BranchAdditions, update.BranchDeletions
	update.Comparison.Ahead = &ahead
	update.Comparison.Behind = &behind
	update.Comparison.Additions = &additions
	update.Comparison.Deletions = &deletions
}
