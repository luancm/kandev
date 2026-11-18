package runtime

import (
	"slices"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

// SSHWorkspaceExecutorConfig copies the stable connection and profile options
// from a saved executor, excluding instance routing and control metadata.
func SSHWorkspaceExecutorConfig(config map[string]string) map[string]string {
	filtered := lifecycle.FilterSSHWorkspaceFallbackConfig(config)
	result := make(map[string]string, len(filtered))
	for key := range filtered {
		result[key] = config[key]
	}
	return result
}

// ErrandWorkspaceLaunch projects the workspace attachment independently of
// the parent's execution and conversation.
func ErrandWorkspaceLaunch(info *WorkspaceInfo, container *models.TaskEnvironment) *LaunchRequest {
	request := &LaunchRequest{
		WorkspaceID: info.WorkspaceID, WorkspacePath: info.WorkspacePath, WorkspaceLayout: info.WorkspaceLayout,
		WorkspaceFolders: slices.Clone(info.WorkspaceFolders), DetachedWorkspaceRoots: slices.Clone(info.DetachedWorkspaceRoots),
		TaskDirName: info.TaskDirName, WorkspaceReuseRequired: true, TaskEnvironmentID: info.TaskEnvironmentID,
		ExecutorType: info.ExecutorType, ManagedRuntimeOwnerUserID: info.OwnerUserID,
		Metadata: map[string]any{lifecycle.MetadataKeyExecutorProfileID: info.ExecutorProfileID},
	}
	executorID, _ := info.Metadata["executor_id"].(string)
	if container != nil {
		request.WorkspacePath = container.WorkspacePath
		executorID = container.ExecutorID
		request.Metadata[lifecycle.MetadataKeyExecutorProfileID] = container.ExecutorProfileID
		request.Metadata[lifecycle.MetadataKeyContainerID] = container.ContainerID
		request.Metadata[lifecycle.MetadataKeyContainerControlAuthSecret] = container.ContainerControlAuthTokenSecretID
		request.Metadata[lifecycle.MetadataKeyBootstrapNonceSecret] = container.ContainerBootstrapNonceSecretID
	}
	if executorID != "" {
		request.Metadata["executor_id"] = executorID
	}
	for _, repo := range info.WorkspaceRepositories {
		request.Repositories = append(request.Repositories, lifecycle.RepoLaunchSpec{
			RepositoryID: repo.RepositoryID, TaskRepositoryID: repo.TaskRepositoryID,
			RepositoryPath: repo.RepositoryPath, RepoName: repo.RepoName, WorktreeID: repo.WorktreeID,
			BaseBranch: repo.BaseBranch, CheckoutBranch: repo.WorktreeBranch, DefaultBranch: repo.DefaultBranch,
			WorkspaceRelativePath: repo.WorkspaceRelativePath, BranchSlug: repo.BranchSlug,
			BranchIdentitySlug: repo.BranchIdentitySlug,
		})
	}
	if len(request.Repositories) != 0 {
		primary := request.Repositories[0]
		request.RepositoryID, request.RepositoryPath = primary.RepositoryID, primary.RepositoryPath
		request.WorktreeID, request.RepoName = primary.WorktreeID, primary.RepoName
		request.BaseBranch, request.CheckoutBranch = primary.BaseBranch, primary.CheckoutBranch
	}
	return request
}
