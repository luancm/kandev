package runtime

import (
	"slices"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

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
	if id, ok := info.Metadata["executor_id"].(string); ok {
		request.Metadata["executor_id"] = id
	}
	if container != nil {
		request.WorkspacePath = container.WorkspacePath
		request.Metadata[lifecycle.MetadataKeyContainerID] = container.ContainerID
		request.Metadata[lifecycle.MetadataKeyContainerControlAuthSecret] = container.ContainerControlAuthTokenSecretID
		request.Metadata[lifecycle.MetadataKeyBootstrapNonceSecret] = container.ContainerBootstrapNonceSecretID
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
