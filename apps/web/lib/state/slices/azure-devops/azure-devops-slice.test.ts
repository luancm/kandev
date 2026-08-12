import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { describe, expect, it } from "vitest";
import { createAzureDevOpsSlice } from "./azure-devops-slice";
import type { AzureDevOpsSlice } from "./types";
import type { AzureDevOpsTaskPullRequest } from "@/lib/types/azure-devops";

function taskPullRequest(
  overrides: Partial<AzureDevOpsTaskPullRequest> = {},
): AzureDevOpsTaskPullRequest {
  return {
    id: "link-1",
    taskId: "task-1",
    repositoryId: "repo-1",
    organizationUrl: "https://dev.azure.com/acme",
    projectId: "project-1",
    azureRepositoryId: "azure-repo-1",
    pullRequestId: 42,
    pullRequestUrl: "https://dev.azure.com/acme/project/_git/repo/pullrequest/42",
    title: "Ship integration",
    sourceBranch: "refs/heads/feature",
    targetBranch: "refs/heads/main",
    authorId: "user-1",
    authorName: "Alice",
    status: "active",
    isDraft: false,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function makeStore() {
  return create<AzureDevOpsSlice>()(immer((set) => createAzureDevOpsSlice(set)));
}

describe("Azure DevOps task PR slice", () => {
  it("preserves source and target identities when a partial refresh omits them", () => {
    const store = makeStore();
    const complete = taskPullRequest({
      sourceOrganizationUrl: "https://dev.azure.com/fork",
      sourceProjectId: "source-project",
      sourceProjectName: "Source project",
      sourceRepositoryId: "source-repo",
      sourceRepositoryName: "source-repository",
      targetOrganizationUrl: "https://dev.azure.com/base",
      targetProjectId: "target-project",
      targetProjectName: "Target project",
      targetRepositoryId: "target-repo",
      targetRepositoryName: "target-repository",
    });
    const partial = taskPullRequest({ title: "Refreshed title" });

    store.getState().setAzureDevOpsTaskPullRequest("task-1", complete);
    store.getState().setAzureDevOpsTaskPullRequest("task-1", partial);

    expect(store.getState().azureDevOpsTaskPullRequests.byTaskId["task-1"]?.[0]).toMatchObject({
      title: "Refreshed title",
      sourceOrganizationUrl: "https://dev.azure.com/fork",
      sourceProjectId: "source-project",
      sourceProjectName: "Source project",
      sourceRepositoryId: "source-repo",
      sourceRepositoryName: "source-repository",
      targetOrganizationUrl: "https://dev.azure.com/base",
      targetProjectId: "target-project",
      targetProjectName: "Target project",
      targetRepositoryId: "target-repo",
      targetRepositoryName: "target-repository",
    });
  });

  it("replaces workspace task associations as one snapshot", () => {
    const store = makeStore();
    store.getState().setAzureDevOpsTaskPullRequests({
      "task-1": [taskPullRequest()],
      "task-2": [taskPullRequest({ id: "link-2", taskId: "task-2", pullRequestId: 7 })],
    });

    expect(Object.keys(store.getState().azureDevOpsTaskPullRequests.byTaskId)).toEqual([
      "task-1",
      "task-2",
    ]);
  });

  it("upserts by persisted association id without changing other tasks", () => {
    const store = makeStore();
    store.getState().setAzureDevOpsTaskPullRequests({
      "task-1": [taskPullRequest()],
      "task-2": [taskPullRequest({ id: "link-2", taskId: "task-2" })],
    });
    store
      .getState()
      .setAzureDevOpsTaskPullRequest(
        "task-1",
        taskPullRequest({ title: "Updated", reviewState: "approved" }),
      );

    expect(store.getState().azureDevOpsTaskPullRequests.byTaskId["task-1"]).toHaveLength(1);
    expect(store.getState().azureDevOpsTaskPullRequests.byTaskId["task-1"]?.[0]?.title).toBe(
      "Updated",
    );
    expect(store.getState().azureDevOpsTaskPullRequests.byTaskId["task-2"]).toHaveLength(1);
  });

  it("removes only the selected task PR association", () => {
    const store = makeStore();
    store.getState().setAzureDevOpsTaskPullRequests({
      "task-1": [taskPullRequest(), taskPullRequest({ id: "link-2", pullRequestId: 7 })],
      "task-2": [taskPullRequest({ id: "link-3", taskId: "task-2" })],
    });

    store.getState().removeAzureDevOpsTaskPullRequest("task-1", "link-1");

    expect(
      store.getState().azureDevOpsTaskPullRequests.byTaskId["task-1"]?.map((pr) => pr.id),
    ).toEqual(["link-2"]);
    expect(store.getState().azureDevOpsTaskPullRequests.byTaskId["task-2"]).toHaveLength(1);
  });

  it("resets all Azure task associations", () => {
    const store = makeStore();
    store.getState().setAzureDevOpsTaskPullRequests({ "task-1": [taskPullRequest()] });
    store.getState().resetAzureDevOpsTaskPullRequests();
    expect(store.getState().azureDevOpsTaskPullRequests.byTaskId).toEqual({});
  });
});
