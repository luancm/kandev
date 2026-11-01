import { useCallback, useContext } from "react";
import {
  TaskSessionNavigationContext,
  type OpenTaskSessionTarget,
} from "./task-session-navigation-context";
export type { OpenTaskSessionTarget } from "./task-session-navigation-context";
import {
  conversationTabSessionId,
  isErrandConversation,
  replaceTaskConversationUrl,
} from "@/lib/task-conversation-navigation";
import { useAppStoreApi } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { addSessionPanel } from "@/lib/state/dockview-panel-actions";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { clearHiddenSessionPanel } from "../../dockview-hidden-session-panels";
import { markSessionTabUserActivationIntent } from "../../session-tab-activation-intent";

function focusParentSurface(hostId: string | null) {
  const dock = useDockviewStore.getState();
  if (!dock.api || !hostId) return;
  clearHiddenSessionPanel(dock.api, hostId);
  dock.api.getPanel?.(`session:${hostId}`)?.api.setActive();
}

/** Select a conversation through its embedded owner or the task workbench, preserving normal tab ownership. */
export function useOpenTaskSession(): (target: OpenTaskSessionTarget) => void {
  const ownerOpen = useContext(TaskSessionNavigationContext);
  const store = useAppStoreApi();
  const { usesDesktopWorkbench } = useResponsiveBreakpoint();
  return useCallback(
    ({ taskId, sessionId, title }: OpenTaskSessionTarget) => {
      if (ownerOpen) {
        ownerOpen({ taskId, sessionId, title });
        return;
      }
      const state = store.getState();
      const sessions = state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
      const errands = state.errands.byTaskId[taskId];
      const target =
        state.taskSessions.items[sessionId] ?? sessions.find((session) => session.id === sessionId);
      if (target && target.task_id !== taskId) return;
      replaceTaskConversationUrl(taskId, sessionId);
      const hostId = conversationTabSessionId(sessionId, sessions, errands);
      const dock = useDockviewStore.getState();
      if (target && isErrandConversation(target, errands)) {
        state.setActiveSession(taskId, sessionId);
        if (usesDesktopWorkbench) focusParentSurface(hostId);
        return;
      }
      if (usesDesktopWorkbench && dock.api) {
        if (target) state.setActiveSession(taskId, sessionId);
        markSessionTabUserActivationIntent(sessionId);
        clearHiddenSessionPanel(dock.api, sessionId);
        addSessionPanel(dock.api, dock.centerGroupId, sessionId, title);
        return;
      }
      store.getState().setActiveSession(taskId, sessionId);
    },
    [store, usesDesktopWorkbench, ownerOpen],
  );
}
