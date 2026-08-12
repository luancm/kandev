import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { AzureDevOpsTaskPullRequest } from "@/lib/types/azure-devops";
import { registerAzureDevOpsHandlers } from "./azure-devops";

const WORKSPACE_A = "workspace-a";

function makeStore(activeWorkspaceId: string | null) {
  const setAzureDevOpsTaskPullRequest = vi.fn();
  const removeAzureDevOpsTaskPullRequest = vi.fn();
  const state = {
    workspaces: { activeId: activeWorkspaceId },
    setAzureDevOpsTaskPullRequest,
    removeAzureDevOpsTaskPullRequest,
  } as unknown as AppState;
  return {
    store: { getState: () => state } as StoreApi<AppState>,
    setAzureDevOpsTaskPullRequest,
    removeAzureDevOpsTaskPullRequest,
  };
}

function taskPullRequest(
  workspaceId: string,
): AzureDevOpsTaskPullRequest & { workspaceId: string } {
  return {
    workspaceId,
    id: "link-1",
    taskId: "task-1",
    repositoryId: "repo-1",
    organizationUrl: "https://dev.azure.com/acme",
    projectId: "project-1",
    azureRepositoryId: "repo-1",
    sourceRepositoryId: "fork-repo",
    targetRepositoryId: "base-repo",
    pullRequestId: 42,
    pullRequestUrl: "https://dev.azure.com/acme/project/_git/repo/pullrequest/42",
    title: "Test PR",
    sourceBranch: "refs/heads/fork",
    targetBranch: "refs/heads/main",
    authorId: "user-1",
    authorName: "Alice",
    status: "active",
    isDraft: false,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

describe("Azure DevOps WebSocket handlers", () => {
  it("upserts a task PR for the active workspace", () => {
    const { store, setAzureDevOpsTaskPullRequest } = makeStore(WORKSPACE_A);
    const handler = registerAzureDevOpsHandlers(store)["azure_devops.task_pr.updated"]!;
    const pr = taskPullRequest(WORKSPACE_A);

    handler({ type: "notification", action: "azure_devops.task_pr.updated", payload: pr });

    expect(setAzureDevOpsTaskPullRequest).toHaveBeenCalledWith("task-1", pr);
  });

  it("removes a detached task PR for the active workspace", () => {
    const { store, removeAzureDevOpsTaskPullRequest } = makeStore(WORKSPACE_A);
    const handler = registerAzureDevOpsHandlers(store)["azure_devops.task_pr.deleted"]!;

    handler({
      type: "notification",
      action: "azure_devops.task_pr.deleted",
      payload: {
        workspaceId: WORKSPACE_A,
        taskId: "task-1",
        associationId: "link-1",
      },
    });

    expect(removeAzureDevOpsTaskPullRequest).toHaveBeenCalledWith("task-1", "link-1");
  });

  it("ignores task PR events from another workspace", () => {
    const { store, setAzureDevOpsTaskPullRequest, removeAzureDevOpsTaskPullRequest } =
      makeStore("workspace-b");
    const update = registerAzureDevOpsHandlers(store)["azure_devops.task_pr.updated"]!;
    const deletion = registerAzureDevOpsHandlers(store)["azure_devops.task_pr.deleted"]!;

    update({
      type: "notification",
      action: "azure_devops.task_pr.updated",
      payload: taskPullRequest(WORKSPACE_A),
    });
    deletion({
      type: "notification",
      action: "azure_devops.task_pr.deleted",
      payload: {
        workspaceId: WORKSPACE_A,
        taskId: "task-1",
        associationId: "link-1",
      },
    });

    expect(setAzureDevOpsTaskPullRequest).not.toHaveBeenCalled();
    expect(removeAzureDevOpsTaskPullRequest).not.toHaveBeenCalled();
  });
});
