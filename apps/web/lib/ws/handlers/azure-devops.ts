import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type {
  AzureDevOpsTaskPullRequestDeletedEvent,
  AzureDevOpsTaskPullRequestUpdatedEvent,
} from "@/lib/types/azure-devops";

export function registerAzureDevOpsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "azure_devops.task_pr.updated": (message) => {
      const pullRequest = message.payload as AzureDevOpsTaskPullRequestUpdatedEvent;
      const activeWorkspaceId = store.getState().workspaces.activeId;
      if (!pullRequest.workspaceId || pullRequest.workspaceId !== activeWorkspaceId) return;
      if (!pullRequest.taskId) return;
      store.getState().setAzureDevOpsTaskPullRequest(pullRequest.taskId, pullRequest);
    },
    "azure_devops.task_pr.deleted": (message) => {
      const deleted = message.payload as AzureDevOpsTaskPullRequestDeletedEvent;
      const activeWorkspaceId = store.getState().workspaces.activeId;
      if (!deleted.workspaceId || deleted.workspaceId !== activeWorkspaceId) return;
      if (!deleted.taskId || !deleted.associationId) return;
      store.getState().removeAzureDevOpsTaskPullRequest(deleted.taskId, deleted.associationId);
    },
  };
}
