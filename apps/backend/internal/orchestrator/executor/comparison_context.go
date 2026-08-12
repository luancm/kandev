package executor

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/common/gitremote"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// comparisonContextsForRepos produces the backend-owned fallback observation
// used at launch and resume. Provider event handlers can replace it with an
// exact linked-change observation; this helper never selects by PR number or
// branch name alone.
func comparisonContextsForRepos(infos []*repoInfo) (map[string]gitremote.ComparisonContext, error) {
	if len(infos) == 0 {
		return nil, nil
	}
	plans := buildRepoBranchPlans(infos)
	contexts := make(map[string]gitremote.ComparisonContext, len(infos))
	for _, info := range infos {
		if info == nil || info.Repository == nil {
			return nil, fmt.Errorf("comparison context repository is incomplete")
		}
		repository, err := comparisonRepositoryIdentity(info.Repository)
		if err != nil {
			return nil, err
		}
		ref := info.BaseBranch
		if ref == "" {
			ref = info.Repository.DefaultBranch
		}
		if ref == "" {
			ref = defaultBaseBranch
		}
		if info.RemoteContribution != nil {
			ref = info.RemoteContribution.BaseBranch
		}
		target := gitremote.RemoteRefIdentity{Repository: repository, Ref: ref}
		context, err := gitremote.NewComparisonContext(target, "", "")
		if err != nil {
			return nil, fmt.Errorf("comparison context for repository %q: %w", info.RepositoryID, err)
		}
		key := ""
		if len(infos) > 1 {
			plan := plans[info]
			key = comparisonContextTrackerKey(info.Repository.Name, plan.pathSlug)
		}
		contexts[key] = context
	}
	return contexts, nil
}

func comparisonContextsForRepoInfo(info *repoInfo) (gitremote.ComparisonContext, bool) {
	if info == nil || info.Repository == nil {
		return gitremote.ComparisonContext{}, false
	}
	repository, err := comparisonRepositoryIdentity(info.Repository)
	if err != nil {
		return gitremote.ComparisonContext{}, false
	}
	ref := info.BaseBranch
	if ref == "" {
		ref = info.Repository.DefaultBranch
	}
	if ref == "" {
		ref = defaultBaseBranch
	}
	if info.RemoteContribution != nil {
		ref = info.RemoteContribution.BaseBranch
	}
	context, err := gitremote.NewComparisonContext(gitremote.RemoteRefIdentity{Repository: repository, Ref: ref}, "", "")
	if err != nil {
		return gitremote.ComparisonContext{}, false
	}
	return context, true
}

func comparisonContextTrackerKey(repoName, pathSlug string) string {
	name := worktree.SanitizeRepoDirName(repoName)
	if name == "" {
		name = repoName
	}
	if pathSlug == "" {
		return name
	}
	return name + "-" + pathSlug
}

func comparisonRepositoryIdentity(repo *models.Repository) (gitremote.RemoteRepositoryIdentity, error) {
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
		var hostErr error
		host, hostErr = gitremote.NormalizeHost(repo.ProviderHost)
		if hostErr != nil {
			return gitremote.RemoteRepositoryIdentity{}, fmt.Errorf("repository %q host: %w", repo.ID, hostErr)
		}
	}
	path := strings.Trim(strings.TrimSpace(repo.ProviderOwner+"/"+repo.ProviderName), "/")
	if host == "" && repo.RemoteURL != "" {
		parsed, err := url.Parse(repo.RemoteURL)
		if err != nil || parsed.User != nil || parsed.Host == "" {
			return gitremote.RemoteRepositoryIdentity{}, fmt.Errorf("repository %q has a credential-bearing or invalid remote URL", repo.ID)
		}
		host, err = gitremote.NormalizeHost(parsed.Host)
		if err != nil {
			return gitremote.RemoteRepositoryIdentity{}, fmt.Errorf("repository %q host: %w", repo.ID, err)
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
		return gitremote.RemoteRepositoryIdentity{}, fmt.Errorf("repository %q identity: %w", repo.ID, err)
	}
	return identity, nil
}
