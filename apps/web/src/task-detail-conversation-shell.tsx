import { KanbanTaskShell } from "@/app/tasks/[id]/kanban-task-shell";
import { RetainedManagedConversationTranscript } from "@/components/plugins/retained-managed-conversation-transcript";
import { OwnedErrandConversation } from "@/components/task/chat/errands/owned-errand-conversation";
import { LinkedOwnedErrandContext } from "@/components/task/chat/errands/owned-errand-navigation-context";
import { isDetachedManagedConversation } from "@/lib/plugins/retained-managed-conversation";
import { extractInitialRepositories, extractInitialScripts } from "@/lib/ssr/session-page-state";
import type { deriveTaskDetailRouteView } from "./task-detail-route-view";

export function TaskDetailConversationShell({
  route,
  layout,
  simple,
  mode,
}: {
  route: ReturnType<typeof deriveTaskDetailRouteView>;
  layout?: string | null;
  simple?: string;
  mode?: string;
}) {
  const ownedErrand = route.data?.ownedErrand ?? null;
  if (ownedErrand && !ownedErrand.parent.available) {
    return (
      <div className="flex h-full min-h-0">
        <OwnedErrandConversation
          taskId={ownedErrand.errand.task_id}
          errandId={ownedErrand.errand.errand_id}
        />
      </div>
    );
  }
  if (route.task && isDetachedManagedConversation(route.task)) {
    return (
      <RetainedManagedConversationTranscript task={route.task} sessionId={route.activeSessionId} />
    );
  }
  return (
    <LinkedOwnedErrandContext.Provider value={ownedErrand}>
      <KanbanTaskShell
        task={route.task}
        taskId={route.shellTaskId}
        sessionId={route.activeSessionId}
        initialRepositories={extractInitialRepositories(route.initialState, route.task)}
        initialScripts={extractInitialScripts(route.initialState, route.task)}
        initialTerminals={route.data?.initialTerminals ?? []}
        defaultLayouts={{}}
        initialLayout={layout}
        urlSimple={simple}
        urlMode={mode}
      />
    </LinkedOwnedErrandContext.Provider>
  );
}
