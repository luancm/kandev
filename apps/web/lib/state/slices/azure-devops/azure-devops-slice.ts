import type { StateCreator } from "zustand";
import type { AzureDevOpsSlice, AzureDevOpsSliceState } from "./types";

export const defaultAzureDevOpsState: AzureDevOpsSliceState = {
  azureDevOpsTaskPullRequests: { byTaskId: {} },
  azureDevOpsTaskWorkItems: { byTaskId: {} },
};

type AzureDevOpsStateCreator = StateCreator<
  AzureDevOpsSlice,
  [["zustand/immer", never]],
  [],
  AzureDevOpsSlice
>;
type AzureDevOpsSliceCreator = (set: Parameters<AzureDevOpsStateCreator>[0]) => AzureDevOpsSlice;

function mergeTaskPullRequestIdentity(
  previous: AzureDevOpsSliceState["azureDevOpsTaskPullRequests"]["byTaskId"][string][number],
  incoming: AzureDevOpsSliceState["azureDevOpsTaskPullRequests"]["byTaskId"][string][number],
) {
  return {
    ...incoming,
    sourceOrganizationUrl: incoming.sourceOrganizationUrl ?? previous.sourceOrganizationUrl,
    sourceProjectId: incoming.sourceProjectId ?? previous.sourceProjectId,
    sourceProjectName: incoming.sourceProjectName ?? previous.sourceProjectName,
    sourceRepositoryId: incoming.sourceRepositoryId ?? previous.sourceRepositoryId,
    sourceRepositoryName: incoming.sourceRepositoryName ?? previous.sourceRepositoryName,
    targetOrganizationUrl: incoming.targetOrganizationUrl ?? previous.targetOrganizationUrl,
    targetProjectId: incoming.targetProjectId ?? previous.targetProjectId,
    targetProjectName: incoming.targetProjectName ?? previous.targetProjectName,
    targetRepositoryId: incoming.targetRepositoryId ?? previous.targetRepositoryId,
    targetRepositoryName: incoming.targetRepositoryName ?? previous.targetRepositoryName,
  };
}

export const createAzureDevOpsSlice: AzureDevOpsSliceCreator = (set) => ({
  ...defaultAzureDevOpsState,
  setAzureDevOpsTaskPullRequests: (pullRequests) =>
    set((draft) => {
      const previous = draft.azureDevOpsTaskPullRequests.byTaskId;
      draft.azureDevOpsTaskPullRequests.byTaskId = Object.fromEntries(
        Object.entries(pullRequests).map(([taskId, incoming]) => [
          taskId,
          incoming.map((pullRequest) => {
            const existing = (previous[taskId] ?? []).find(
              (candidate) => candidate.id === pullRequest.id,
            );
            return existing ? mergeTaskPullRequestIdentity(existing, pullRequest) : pullRequest;
          }),
        ]),
      );
    }),
  setAzureDevOpsTaskPullRequest: (taskId, pullRequest) =>
    set((draft) => {
      const existing = draft.azureDevOpsTaskPullRequests.byTaskId[taskId] ?? [];
      const index = existing.findIndex((item) => item.id === pullRequest.id);
      if (index >= 0) {
        existing[index] = mergeTaskPullRequestIdentity(existing[index]!, pullRequest);
      } else existing.push(pullRequest);
      draft.azureDevOpsTaskPullRequests.byTaskId[taskId] = existing;
    }),
  removeAzureDevOpsTaskPullRequest: (taskId, associationId) =>
    set((draft) => {
      const existing = draft.azureDevOpsTaskPullRequests.byTaskId[taskId];
      if (!existing) return;
      const remaining = existing.filter((pullRequest) => pullRequest.id !== associationId);
      if (remaining.length === 0) delete draft.azureDevOpsTaskPullRequests.byTaskId[taskId];
      else draft.azureDevOpsTaskPullRequests.byTaskId[taskId] = remaining;
    }),
  resetAzureDevOpsTaskPullRequests: () =>
    set((draft) => {
      draft.azureDevOpsTaskPullRequests.byTaskId = {};
    }),
  setAzureDevOpsTaskWorkItems: (workItems) =>
    set((draft) => {
      draft.azureDevOpsTaskWorkItems.byTaskId = workItems;
    }),
  setAzureDevOpsTaskWorkItem: (taskId, workItem) =>
    set((draft) => {
      const existing = draft.azureDevOpsTaskWorkItems.byTaskId[taskId] ?? [];
      const index = existing.findIndex((item) => item.id === workItem.id);
      if (index >= 0) existing[index] = workItem;
      else existing.push(workItem);
      draft.azureDevOpsTaskWorkItems.byTaskId[taskId] = existing;
    }),
  resetAzureDevOpsTaskWorkItems: () =>
    set((draft) => {
      draft.azureDevOpsTaskWorkItems.byTaskId = {};
    }),
});
