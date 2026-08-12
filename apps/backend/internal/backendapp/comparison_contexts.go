package backendapp

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/azuredevops"
	"github.com/kandev/kandev/internal/common/gitremote"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// comparisonContextLinkProvider adapts provider stores into the narrow
// provider-neutral observation seam owned by task service. It deliberately
// returns rows grouped by task repository so the task service can enforce
// exact-ref uniqueness and never select by provider row order.
func comparisonContextLinkProvider(gh *github.Service, gl *gitlab.Service, az *azuredevops.Service) taskservice.ComparisonContextLinkProvider {
	return func(ctx context.Context, taskID string) (map[string][]gitremote.LinkedChange, error) {
		out := make(map[string][]gitremote.LinkedChange)
		if gh != nil {
			rowsByTask, err := gh.ListTaskPRs(ctx, []string{taskID})
			if err != nil {
				return nil, fmt.Errorf("list GitHub linked changes: %w", err)
			}
			rows := rowsByTask[taskID]
			for _, row := range rows {
				if row == nil {
					continue
				}
				change, err := githubLinkedChange(row)
				if err != nil {
					return nil, err
				}
				out[row.RepositoryID] = append(out[row.RepositoryID], change)
			}
		}
		if gl != nil {
			rows, err := gl.ListTaskMRsByTask(ctx, taskID)
			if err != nil {
				return nil, fmt.Errorf("list GitLab linked changes: %w", err)
			}
			for _, row := range rows {
				if row == nil {
					continue
				}
				change, err := gitlabLinkedChange(row)
				if err != nil {
					return nil, err
				}
				out[row.RepositoryID] = append(out[row.RepositoryID], change)
			}
		}
		if az != nil {
			rows, err := az.ListTaskPRsByTask(ctx, taskID)
			if err != nil {
				return nil, fmt.Errorf("list Azure linked changes: %w", err)
			}
			for _, row := range rows {
				if row == nil {
					continue
				}
				change, err := azureLinkedChange(row)
				if err != nil {
					return nil, err
				}
				out[row.RepositoryID] = append(out[row.RepositoryID], change)
			}
		}
		return out, nil
	}
}

func githubLinkedChange(row *github.TaskPR) (gitremote.LinkedChange, error) {
	headHost, err := linkedHost(row.HeadHost, row.BaseHost, "github.com")
	if err != nil {
		return gitremote.LinkedChange{}, fmt.Errorf("GitHub linked source %q host: %w", row.ID, err)
	}
	baseHost, err := linkedHost(row.BaseHost, row.HeadHost, "github.com")
	if err != nil {
		return gitremote.LinkedChange{}, fmt.Errorf("GitHub linked base %q host: %w", row.ID, err)
	}
	source := gitremote.RemoteRefIdentity{Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderGitHub, Host: headHost, RepositoryPath: strings.Trim(row.HeadOwner+"/"+row.HeadRepo, "/"), ProviderRepositoryID: int64String(row.HeadRepoID)}, Ref: row.HeadBranch}
	base := gitremote.RemoteRefIdentity{Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderGitHub, Host: baseHost, RepositoryPath: strings.Trim(row.BaseOwner+"/"+row.BaseRepo, "/"), ProviderRepositoryID: int64String(row.BaseRepoID)}, Ref: row.BaseBranch}
	if err := source.Validate(); err != nil {
		return gitremote.LinkedChange{}, fmt.Errorf("GitHub linked source %q: %w", row.ID, err)
	}
	if err := base.Validate(); err != nil {
		return gitremote.LinkedChange{}, fmt.Errorf("GitHub linked base %q: %w", row.ID, err)
	}
	return gitremote.LinkedChange{Source: &source, Base: &base}, nil
}

func gitlabLinkedChange(row *gitlab.TaskMR) (gitremote.LinkedChange, error) {
	sourceHost, err := linkedHost(row.SourceHost, row.TargetHost, row.Host)
	if err != nil {
		return gitremote.LinkedChange{}, fmt.Errorf("GitLab linked source %q host: %w", row.ID, err)
	}
	baseHost, err := linkedHost(row.TargetHost, row.SourceHost, row.Host)
	if err != nil {
		return gitremote.LinkedChange{}, fmt.Errorf("GitLab linked base %q host: %w", row.ID, err)
	}
	source := gitremote.RemoteRefIdentity{Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderGitLab, Host: sourceHost, RepositoryPath: firstNonEmpty(row.SourceProjectPath, row.TargetProjectPath, row.ProjectPath), ProviderRepositoryID: int64String(row.SourceProjectID)}, Ref: row.HeadBranch}
	base := gitremote.RemoteRefIdentity{Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderGitLab, Host: baseHost, RepositoryPath: firstNonEmpty(row.TargetProjectPath, row.SourceProjectPath, row.ProjectPath), ProviderRepositoryID: int64String(row.TargetProjectID)}, Ref: row.BaseBranch}
	if err := source.Validate(); err != nil {
		return gitremote.LinkedChange{}, fmt.Errorf("GitLab linked source %q: %w", row.ID, err)
	}
	if err := base.Validate(); err != nil {
		return gitremote.LinkedChange{}, fmt.Errorf("GitLab linked base %q: %w", row.ID, err)
	}
	return gitremote.LinkedChange{Source: &source, Base: &base}, nil
}

func azureLinkedChange(row *azuredevops.TaskPR) (gitremote.LinkedChange, error) {
	sourceHost, err := azureLinkedHost(row.SourceOrganizationURL, row.OrganizationURL)
	if err != nil {
		return gitremote.LinkedChange{}, fmt.Errorf("Azure linked source %q host: %w", row.ID, err)
	}
	baseHost, err := azureLinkedHost(row.TargetOrganizationURL, row.OrganizationURL)
	if err != nil {
		return gitremote.LinkedChange{}, fmt.Errorf("Azure linked base %q host: %w", row.ID, err)
	}
	sourcePath := strings.Trim(strings.TrimSpace(firstNonEmpty(row.SourceProjectName, row.SourceProjectID))+"/"+strings.TrimSpace(firstNonEmpty(row.SourceRepositoryName, row.SourceRepositoryID)), "/")
	basePath := strings.Trim(strings.TrimSpace(firstNonEmpty(row.TargetProjectName, row.TargetProjectID))+"/"+strings.TrimSpace(firstNonEmpty(row.TargetRepositoryName, row.TargetRepositoryID)), "/")
	source := gitremote.RemoteRefIdentity{Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderAzureRepos, Host: sourceHost, RepositoryPath: sourcePath, ProviderRepositoryID: row.SourceRepositoryID}, Ref: row.SourceBranch}
	base := gitremote.RemoteRefIdentity{Repository: gitremote.RemoteRepositoryIdentity{Provider: gitremote.ProviderAzureRepos, Host: baseHost, RepositoryPath: basePath, ProviderRepositoryID: row.TargetRepositoryID}, Ref: row.TargetBranch}
	if err := source.Validate(); err != nil {
		return gitremote.LinkedChange{}, fmt.Errorf("Azure linked source %q: %w", row.ID, err)
	}
	if err := base.Validate(); err != nil {
		return gitremote.LinkedChange{}, fmt.Errorf("Azure linked base %q: %w", row.ID, err)
	}
	return gitremote.LinkedChange{Source: &source, Base: &base}, nil
}

func linkedHost(values ...string) (string, error) {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		host, err := gitremote.NormalizeHost(value)
		if err != nil {
			return "", err
		}
		return host, nil
	}
	return "", fmt.Errorf("repository host is missing")
}

func azureLinkedHost(values ...string) (string, error) {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		rawHost := value
		if strings.Contains(value, "://") {
			parsed, err := url.Parse(value)
			if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
				return "", fmt.Errorf("repository host is invalid")
			}
			rawHost = parsed.Host
		}
		host, err := gitremote.NormalizeHost(rawHost)
		if err != nil {
			return "", err
		}
		return host, nil
	}
	return "", fmt.Errorf("repository host is missing")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func int64String(value int64) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprintf("%d", value)
}
