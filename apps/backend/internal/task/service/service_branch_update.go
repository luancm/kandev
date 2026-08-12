package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/gitremote"
	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// UpdateRepositoryBaseBranchRequest carries the parameters for the
// changes-panel "Compare against" picker. Mutates exactly one
// task_repositories row.
type UpdateRepositoryBaseBranchRequest struct {
	TaskID           string
	TaskRepositoryID string
	BaseBranch       string
}

// ErrTaskRepositoryNotFound is returned when the supplied task_repository_id
// has no row, or it belongs to a different task than the caller claimed.
var ErrTaskRepositoryNotFound = errors.New("task repository not found")

// UpdateRepositoryBaseBranch changes the base_branch on a single
// task_repositories row, publishes task.updated so connected clients refresh,
// and pushes the new per-repo map to the live agentctl instance (if any) so
// the changes panel updates its BaseCommit / Ahead / Behind without waiting
// for a session restart.
//
// The DB write is the source of truth; a failed push is logged at warn but
// does NOT roll the DB back — at next session launch the persisted map
// rebuilds trackers correctly. Callers that need stronger guarantees can
// re-issue the request.
//
// Validation:
//   - TaskID, TaskRepositoryID, BaseBranch all required.
//   - BaseBranch is trimmed; whitespace-only is rejected.
//   - The TaskRepository row must belong to the supplied TaskID — guards
//     against a caller pointing at someone else's task_repository_id.
//
// Returns the updated TaskRepository on success.
func (s *Service) UpdateRepositoryBaseBranch(ctx context.Context, req UpdateRepositoryBaseBranchRequest) (*models.TaskRepository, error) {
	baseBranch, err := validateUpdateRepositoryBaseBranchRequest(req)
	if err != nil {
		return nil, err
	}
	// Ahead of every repository read. The WS action names task_id so the gateway
	// backstop already covers that transport, but this method is also reachable
	// over HTTP and MCP, and a backstop is not a substitute for a service guard.
	if err := s.authorizeTaskID(ctx, req.TaskID); err != nil {
		return nil, err
	}
	taskRepo, err := s.loadTaskRepositoryForUpdate(ctx, req.TaskID, req.TaskRepositoryID)
	if err != nil {
		return nil, err
	}
	if taskRepo.BaseBranch == baseBranch {
		return taskRepo, nil
	}

	taskRepo.BaseBranch = baseBranch
	if err := s.taskRepos.UpdateTaskRepository(ctx, taskRepo); err != nil {
		return nil, fmt.Errorf("update task repository: %w", err)
	}

	// Detach from the caller's ctx for post-commit fan-out: the DB row is
	// already persisted, so if the HTTP / WS request gets cancelled mid-
	// response we must still clear session.base_commit_sha and push to
	// agentctl — otherwise the persisted task_repositories row would
	// disagree with the cached session SHA + live tracker map until the
	// next session launch.
	s.applyBaseBranchSideEffects(context.WithoutCancel(ctx), req.TaskID, taskRepo.RepositoryID, baseBranch)
	return taskRepo, nil
}

// validateUpdateRepositoryBaseBranchRequest checks required fields and
// returns the trimmed + sanitized base_branch. Pulled out so the main
// service method stays under the cyclomatic-complexity lint cap.
func validateUpdateRepositoryBaseBranchRequest(req UpdateRepositoryBaseBranchRequest) (string, error) {
	if req.TaskID == "" {
		return "", fmt.Errorf("task_id is required")
	}
	if req.TaskRepositoryID == "" {
		return "", fmt.Errorf("task_repository_id is required")
	}
	baseBranch := strings.TrimSpace(req.BaseBranch)
	if baseBranch == "" {
		return "", fmt.Errorf("base_branch is required")
	}
	// Reject values that would be unsafe to splice into a `git` argument
	// list downstream (the picker payload is user-controlled and reaches
	// `exec.Command("git", …, baseBranch)` via the agentctl workspace
	// tracker). Mirrors process.IsSafeGitRef in the agentctl side; kept
	// independent here so the service stays self-contained.
	if !isSafeBaseBranchRef(baseBranch) {
		return "", fmt.Errorf("base_branch contains characters not allowed in a git ref name")
	}
	return baseBranch, nil
}

// loadTaskRepositoryForUpdate fetches the row and validates it belongs to
// the supplied task. Folds the repo-tier "not found" string error into
// ErrTaskRepositoryNotFound for stable caller-side classification.
func (s *Service) loadTaskRepositoryForUpdate(ctx context.Context, taskID, taskRepositoryID string) (*models.TaskRepository, error) {
	taskRepo, err := s.taskRepos.GetTaskRepository(ctx, taskRepositoryID)
	if err != nil {
		if strings.Contains(err.Error(), "task repository not found") {
			return nil, ErrTaskRepositoryNotFound
		}
		return nil, fmt.Errorf("get task repository: %w", err)
	}
	if taskRepo == nil || taskRepo.TaskID != taskID {
		return nil, ErrTaskRepositoryNotFound
	}
	return taskRepo, nil
}

// applyBaseBranchSideEffects runs the post-write fan-out that keeps the
// commits panel, cumulative diff, WS-driven UIs, and the running agentctl
// instance aligned with the new base. Each step is best-effort: failures
// here don't roll back the DB because the persisted task_repositories
// row is the authoritative source for the next session launch.
func (s *Service) applyBaseBranchSideEffects(ctx context.Context, taskID, repositoryID, baseBranch string) {
	if _, err := s.sessions.ResetTaskSessionBasesForRepository(ctx, taskID, repositoryID, baseBranch); err != nil {
		s.logger.Warn("UpdateRepositoryBaseBranch: failed to reset session bases",
			zap.String("task_id", taskID),
			zap.String("repository_id", repositoryID),
			zap.Error(err))
	}
	if task, err := s.tasks.GetTask(ctx, taskID); err == nil && task != nil {
		s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	}
	if s.baseBranchPusher != nil {
		branches, mapErr := s.collectTaskBaseBranches(ctx, taskID)
		if mapErr != nil {
			s.logger.Warn("UpdateRepositoryBaseBranch: failed to collect base branches for live push",
				zap.String("task_id", taskID),
				zap.Error(mapErr))
		} else if len(branches) > 0 {
			// Empty map = task currently has no recorded base_branches. Pushing
			// nil to agentctl would call Manager.UpdateBaseBranches(nil) and wipe
			// every tracker's override, including ones the caller didn't touch.
			// Skip the push instead — the DB row we just updated is the source of
			// truth for the next session launch.
			s.baseBranchPusher.PushBaseBranchesForTask(ctx, taskID, branches)
		}
	}
	if s.comparisonContextPusher != nil {
		if contexts, err := s.TaskComparisonContexts(ctx, taskID); err != nil {
			s.logger.Warn("UpdateRepositoryBaseBranch: failed to collect comparison contexts for live push", zap.String("task_id", taskID), zap.Error(err))
		} else if contexts != nil {
			s.comparisonContextPusher.PushComparisonContextsForTask(ctx, taskID, contexts)
		}
	}
}

// isSafeBaseBranchRef delegates to the shared
// `securityutil.IsValidBranchName` allowlist so the service-tier rejection
// rules track agentctl's exactly. Without this, a value like
// `feature/@2024` would pass here, persist to task_repositories, then get
// dropped by the agentctl-side sanitiser at push time — silently wiping
// every tracker's override. Sharing one allowlist keeps the DB write and
// the live push agreeing on what counts as a valid ref name.
//
// `origin/<name>` refs are split before validation because the underlying
// regex disallows "/" as the first character.
func isSafeBaseBranchRef(ref string) bool {
	if rest, ok := strings.CutPrefix(ref, "origin/"); ok {
		return securityutil.IsValidBranchName(rest)
	}
	return securityutil.IsValidBranchName(ref)
}

// TaskBaseBranches exposes the stored per-repo base-branch map so the agent
// runtime can seed a workspace at agentctl-ready time. Wired as
// lifecycle.BaseBranchProvider; the launch path builds the same shape from the
// LaunchRequest, and this is the DB-backed equivalent for every other path.
func (s *Service) TaskBaseBranches(ctx context.Context, taskID string) (map[string]string, error) {
	if taskID == "" {
		return nil, nil
	}
	return s.collectTaskBaseBranches(ctx, taskID)
}

// TaskComparisonContexts hydrates the attached-repository fallback context for
// every task worktree. Provider event handlers may layer exact linked-change
// evidence above this value; this method intentionally never guesses from a
// PR number or branch name alone.
func (s *Service) TaskComparisonContexts(ctx context.Context, taskID string) (map[string]gitremote.ComparisonContext, error) {
	if taskID == "" {
		return nil, nil
	}
	taskRepos, err := s.taskRepos.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list task repositories: %w", err)
	}
	repos, err := s.resolveBaseBranchRepositories(ctx, taskRepos)
	if err != nil {
		return nil, err
	}
	inputs := make([]worktree.BranchIdentityInput, len(taskRepos))
	for i, tr := range taskRepos {
		defaultBranch := ""
		if repos[i] != nil {
			defaultBranch = repos[i].DefaultBranch
		}
		inputs[i] = worktree.BranchIdentityInput{RepositoryID: tr.RepositoryID, BaseBranch: tr.BaseBranch, CheckoutBranch: tr.CheckoutBranch, DefaultBranch: defaultBranch, PRNumber: taskRepositoryPRNumber(tr.Metadata), Position: tr.Position}
	}
	plans := worktree.BuildBranchIdentityPlans(inputs)
	contexts := make(map[string]gitremote.ComparisonContext, len(taskRepos))
	for i, tr := range taskRepos {
		repo := repos[i]
		if repo == nil {
			return nil, fmt.Errorf("repository %s is unavailable while hydrating comparison context", tr.RepositoryID)
		}
		ref := tr.BaseBranch
		if ref == "" {
			ref = repo.DefaultBranch
		}
		if ref == "" {
			return nil, fmt.Errorf("repository %s has no selected base ref", tr.RepositoryID)
		}
		identity, err := repositoryRemoteIdentity(repo)
		if err != nil {
			return nil, fmt.Errorf("repository %s comparison identity: %w", tr.RepositoryID, err)
		}
		target := gitremote.RemoteRefIdentity{Repository: identity, Ref: ref}
		if binding, present, loadErr := models.LoadRemoteContribution(tr.Metadata); loadErr != nil {
			return nil, fmt.Errorf("repository %s remote contribution: %w", tr.RepositoryID, loadErr)
		} else if present {
			// A contribution binding is authoritative for the target ref while
			// its attached repository remains the authorization anchor.
			target.Ref = binding.BaseBranch
		}
		context, err := gitremote.NewComparisonContext(target, "", "")
		if err != nil {
			return nil, fmt.Errorf("repository %s comparison context: %w", tr.RepositoryID, err)
		}
		key := baseBranchTrackerKey(repo.Name, plans[i].PathSlug)
		contexts[key] = context
	}
	if s.comparisonContextLinkProvider != nil {
		links, linkErr := s.comparisonContextLinkProvider(ctx, taskID)
		if linkErr != nil {
			return nil, fmt.Errorf("hydrate linked comparison changes: %w", linkErr)
		}
		for i, tr := range taskRepos {
			candidates := links[tr.RepositoryID]
			if len(candidates) == 0 && len(taskRepos) == 1 {
				// Provider rows written before task-repository scoping have an
				// empty RepositoryID. They are safe to consider only for a
				// single-repository task; multi-repository rows must remain
				// unresolved rather than being assigned by list order.
				candidates = links[""]
			}
			if len(candidates) == 0 {
				continue
			}
			// The task repository's stored contribution identity, when present,
			// is the durable action-head evidence. Otherwise a checkout of the
			// attached repository supplies the same-repository action identity.
			// A fork source without either exact identity remains unresolved;
			// provider rows are never selected by branch name alone.
			var actionHead *gitremote.RemoteRefIdentity
			if binding, present, loadErr := models.LoadRemoteContribution(tr.Metadata); loadErr != nil {
				return nil, fmt.Errorf("repository %s remote contribution: %w", tr.RepositoryID, loadErr)
			} else if present {
				identity, identityErr := remoteContributionSourceIdentity(binding)
				if identityErr != nil {
					return nil, fmt.Errorf("repository %s action-head identity: %w", tr.RepositoryID, identityErr)
				}
				actionHead = &identity
			} else if tr.CheckoutBranch != "" {
				identity := gitremote.RemoteRefIdentity{Repository: contexts[baseBranchTrackerKey(repos[i].Name, plans[i].PathSlug)].Target.Repository, Ref: tr.CheckoutBranch}
				actionHead = &identity
			}
			selected := gitremote.SelectComparisonContext(gitremote.ComparisonContextInput{
				ActionHead:        actionHead,
				LinkedChanges:     candidates,
				ContextGeneration: "",
			})
			if selected.State != gitremote.ResolutionResolved {
				return nil, fmt.Errorf("repository %s linked comparison context unresolved: %s", tr.RepositoryID, selected.Reason)
			}
			key := baseBranchTrackerKey(repos[i].Name, plans[i].PathSlug)
			contexts[key] = selected.Context
			if len(taskRepos) == 1 {
				contexts[""] = selected.Context.Clone()
			}
		}
	}
	if len(taskRepos) == 1 {
		if context, ok := contexts[baseBranchTrackerKey(repos[0].Name, plans[0].PathSlug)]; ok {
			contexts[""] = context.Clone()
		}
	}
	return contexts, nil
}

func repositoryRemoteIdentity(repo *models.Repository) (gitremote.RemoteRepositoryIdentity, error) {
	provider := gitremote.Provider(repo.Provider)
	if provider == "" && repo.ProviderOwner != "" && repo.ProviderName != "" {
		provider = gitremote.ProviderGitHub
	}
	switch strings.ToLower(string(provider)) {
	case "github":
		provider = gitremote.ProviderGitHub
	case "gitlab":
		provider = gitremote.ProviderGitLab
	case "azure", "azuredevops", "azure_repos":
		provider = gitremote.ProviderAzureRepos
	case "":
		provider = gitremote.ProviderGeneric
	default:
		return gitremote.RemoteRepositoryIdentity{}, fmt.Errorf("unsupported repository provider %q", repo.Provider)
	}
	host := ""
	if strings.TrimSpace(repo.ProviderHost) != "" {
		var err error
		host, err = gitremote.NormalizeHost(repo.ProviderHost)
		if err != nil {
			return gitremote.RemoteRepositoryIdentity{}, err
		}
	}
	path := strings.Trim(strings.TrimSpace(repo.ProviderOwner+"/"+repo.ProviderName), "/")
	if host == "" && repo.RemoteURL != "" {
		parsed, err := url.Parse(repo.RemoteURL)
		if err != nil || parsed.User != nil || parsed.Host == "" {
			return gitremote.RemoteRepositoryIdentity{}, fmt.Errorf("remote URL is not credential-free")
		}
		host, err = gitremote.NormalizeHost(parsed.Host)
		if err != nil {
			return gitremote.RemoteRepositoryIdentity{}, err
		}
		if path == "" {
			path = strings.Trim(parsed.Path, "/")
		}
	}
	if host == "" {
		host = "local"
	}
	if path == "" {
		path = repo.Name
	}
	identity := gitremote.RemoteRepositoryIdentity{Provider: provider, Host: host, RepositoryPath: path, ProviderRepositoryID: repo.ProviderRepoID}
	if err := identity.Validate(); err != nil {
		return gitremote.RemoteRepositoryIdentity{}, err
	}
	return identity, nil
}

func remoteContributionSourceIdentity(binding models.RemoteContribution) (gitremote.RemoteRefIdentity, error) {
	provider := gitremote.Provider(binding.Provider)
	switch strings.ToLower(string(provider)) {
	case models.RemoteContributionProviderGitHub:
		provider = gitremote.ProviderGitHub
	case models.RemoteContributionProviderGitLab:
		provider = gitremote.ProviderGitLab
	default:
		return gitremote.RemoteRefIdentity{}, fmt.Errorf("unsupported contribution provider %q", binding.Provider)
	}
	host, err := gitremote.NormalizeHost(binding.SourceRepository.Host)
	if err != nil {
		return gitremote.RemoteRefIdentity{}, err
	}
	identity := gitremote.RemoteRefIdentity{Repository: gitremote.RemoteRepositoryIdentity{
		Provider:             provider,
		Host:                 host,
		RepositoryPath:       binding.SourceRepository.Path,
		ProviderRepositoryID: binding.SourceRepository.ProviderID,
	}, Ref: binding.HeadBranch}
	if err := identity.Validate(); err != nil {
		return gitremote.RemoteRefIdentity{}, err
	}
	return identity, nil
}

// collectTaskBaseBranches builds the per-worktree {trackerName → base_branch}
// map the agentctl WorkspaceTracker reads. Mirrors
// lifecycle.baseBranchMetadataKey but at update time the LaunchRequest is gone,
// so we hydrate from the DB: list task_repositories, resolve each Repository to
// recover its Name, and derive the same branch-identity path slug the launch
// path derives, so the key equals the worktree directory basename — which is
// what the tracker reports as its repositoryName.
//
// Keying by the bare repository name is NOT sufficient. A task may attach the
// same repository on several branches (task_repositories is unique on
// task+repo+base+checkout), and those siblings live in
// `{RepoName}-{BranchSlug}` directories. Collapsing them under `{RepoName}`
// leaves every sibling tracker without a key, and because SetBaseBranches
// *replaces* the stored map, pushing that collapsed map actively overwrites the
// correctly-keyed map the launch path already seeded — sending the siblings
// back to the origin/main fallback.
func (s *Service) collectTaskBaseBranches(ctx context.Context, taskID string) (map[string]string, error) {
	taskRepos, err := s.taskRepos.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list task repositories: %w", err)
	}
	repos, err := s.resolveBaseBranchRepositories(ctx, taskRepos)
	if err != nil {
		return nil, err
	}
	// Plans are computed over *every* row, including rows without a base
	// branch: BuildBranchIdentityPlans groups by repository and picks which
	// member of a group keeps the flat legacy path. Filtering first would
	// change that choice and desync these keys from the launch path's.
	inputs := make([]worktree.BranchIdentityInput, len(taskRepos))
	for i, tr := range taskRepos {
		defaultBranch := ""
		if repos[i] != nil {
			defaultBranch = repos[i].DefaultBranch
		}
		inputs[i] = worktree.BranchIdentityInput{
			RepositoryID:   tr.RepositoryID,
			BaseBranch:     tr.BaseBranch,
			CheckoutBranch: tr.CheckoutBranch,
			DefaultBranch:  defaultBranch,
			PRNumber:       taskRepositoryPRNumber(tr.Metadata),
			Position:       tr.Position,
		}
	}
	plans := worktree.BuildBranchIdentityPlans(inputs)

	out := make(map[string]string, len(taskRepos)+1)
	for i, tr := range taskRepos {
		if tr.BaseBranch == "" {
			continue
		}
		out[baseBranchTrackerKey(repos[i].Name, plans[i].PathSlug)] = tr.BaseBranch
	}
	// Single-repo legacy fallback: when only one row, duplicate under the
	// empty key so the root WorkspaceTracker (repositoryName == "") picks it
	// up too — matches the synthesis lifecycle.collectBaseBranches performs
	// from req.RepoSpecs().
	if len(taskRepos) == 1 && taskRepos[0].BaseBranch != "" {
		if _, ok := out[""]; !ok {
			out[""] = taskRepos[0].BaseBranch
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// resolveBaseBranchRepositories resolves the Repository entity for each row,
// positionally aligned with taskRepos so the branch-identity plans can be
// indexed alongside.
//
// A row that records a base branch but cannot resolve its repository makes the
// map INCOMPLETE, not merely smaller. agentctl's SetBaseBranches replaces the
// stored map wholesale, so pushing a partial map silently drops the base branch
// of every repository that failed to resolve — and the caller's len(map) > 0
// guard cannot detect it, because the map is non-empty. Fail instead: callers
// already skip the push on error, leaving the previously-correct map in place.
//
// A row *without* a base branch contributes no key, so an unresolvable one is
// tolerated (nil entry) rather than failing the whole map; it still occupies
// its index so the plans stay aligned.
func (s *Service) resolveBaseBranchRepositories(
	ctx context.Context,
	taskRepos []*models.TaskRepository,
) ([]*models.Repository, error) {
	repos := make([]*models.Repository, len(taskRepos))
	for i, tr := range taskRepos {
		repo, err := s.repoEntities.GetRepository(ctx, tr.RepositoryID)
		if err != nil {
			if tr.BaseBranch == "" {
				continue
			}
			return nil, fmt.Errorf("resolve repository %s for base-branch map: %w", tr.RepositoryID, err)
		}
		if repo == nil || repo.Name == "" {
			if tr.BaseBranch == "" {
				continue
			}
			return nil, fmt.Errorf("repository %s for base-branch map is missing or unnamed", tr.RepositoryID)
		}
		repos[i] = repo
	}
	return repos, nil
}

// baseBranchTrackerKey builds the map key a WorkspaceTracker looks itself up
// under: the worktree directory basename. Mirrors
// lifecycle.baseBranchMetadataKey exactly — same sanitiser, same raw-name
// fallback, same `-`-joined path slug — so the DB-hydrated map and the
// launch-time map agree on every key.
func baseBranchTrackerKey(repoName, pathSlug string) string {
	key := worktree.SanitizeRepoDirName(repoName)
	if key == "" {
		key = repoName
	}
	if pathSlug == "" {
		return key
	}
	return key + "-" + pathSlug
}
