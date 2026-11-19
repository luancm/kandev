import { useCallback, useContext, useEffect, useRef } from "react";
import {
  TaskSessionNavigationContext,
  type OpenTaskSessionTarget,
} from "./task-session-navigation-context";
export type { OpenTaskSessionTarget } from "./task-session-navigation-context";
import { replaceTaskConversationUrl } from "@/lib/task-conversation-navigation";
import { useAppStoreApi } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { addSessionPanel } from "@/lib/state/dockview-panel-actions";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { clearHiddenSessionPanel } from "../../dockview-hidden-session-panels";
import { markSessionTabUserActivationIntent } from "../../session-tab-activation-intent";
import type { ErrandInspection } from "@/lib/types/errands";
import { readErrandInspection } from "@/lib/state/slices/errands/errand-browser-read-coordinator";
import { OwnedErrandNavigationContext } from "./owned-errand-navigation-context";

type AppStoreApi = ReturnType<typeof useAppStoreApi>;
type OpenSession = (target: OpenTaskSessionTarget) => Promise<boolean>;
type TaskSession = ReturnType<AppStoreApi["getState"]>["taskSessions"]["items"][string];

async function inspectAndOpenErrand(
  store: AppStoreApi,
  { taskId, sessionId }: OpenTaskSessionTarget,
  openOwned: ((inspection: ErrandInspection) => boolean) | null,
  isCurrent: () => boolean,
): Promise<boolean> {
  if (!openOwned) return false;
  const read = readErrandInspection(store, taskId, sessionId);
  try {
    const inspection = await read.promise;
    return (
      inspection.owns_conversation === true &&
      isCurrent() &&
      read.isCurrent() &&
      openOwned(inspection)
    );
  } catch {
    return false;
  } finally {
    read.release();
  }
}

function openOrdinaryTarget(args: {
  target: OpenTaskSessionTarget;
  session: TaskSession | undefined;
  ownerOpen: ((target: OpenTaskSessionTarget) => void) | null;
  taskId: string;
  sessionId: string;
  title: string;
  store: AppStoreApi;
  usesDesktopWorkbench: boolean;
}): boolean {
  const { target, session, ownerOpen, taskId, sessionId, title, store, usesDesktopWorkbench } =
    args;
  if (ownerOpen) {
    ownerOpen(target);
    return true;
  }
  if (!session) return false;
  replaceTaskConversationUrl(taskId, sessionId);
  const dock = useDockviewStore.getState();
  if (usesDesktopWorkbench && dock.api) {
    store.getState().setActiveSession(taskId, sessionId);
    markSessionTabUserActivationIntent(sessionId);
    clearHiddenSessionPanel(dock.api, sessionId);
    addSessionPanel(dock.api, dock.centerGroupId, sessionId, title);
    return true;
  }
  store.getState().setActiveSession(taskId, sessionId);
  return true;
}

/** Select a conversation through its embedded owner or the task workbench, preserving normal tab ownership. */
export function useOpenTaskSession(): OpenSession {
  const ownerOpen = useContext(TaskSessionNavigationContext);
  const openOwned = useContext(OwnedErrandNavigationContext);
  const store = useAppStoreApi();
  const { usesDesktopWorkbench } = useResponsiveBreakpoint();
  const requestAttemptRef = useRef(0);
  const navigationGenerationRef = useRef(0);
  const mountedRef = useRef(false);
  const ownerOpenRef = useRef(ownerOpen);
  const ownedOpenRef = useRef(openOwned);
  if (ownerOpenRef.current !== ownerOpen || ownedOpenRef.current !== openOwned) {
    ownerOpenRef.current = ownerOpen;
    ownedOpenRef.current = openOwned;
    navigationGenerationRef.current++;
  }

  useEffect(() => {
    mountedRef.current = true;
    const unsubscribe = store.subscribe((state, previousState) => {
      if (
        state.tasks.activeTaskId !== previousState.tasks.activeTaskId ||
        state.tasks.activeSessionId !== previousState.tasks.activeSessionId
      ) {
        navigationGenerationRef.current++;
      }
    });
    return () => {
      mountedRef.current = false;
      navigationGenerationRef.current++;
      unsubscribe();
    };
  }, [store]);

  return useCallback(
    async (target: OpenTaskSessionTarget) => {
      const { taskId, sessionId, title } = target;
      const requestAttempt = ++requestAttemptRef.current;
      const navigationGeneration = navigationGenerationRef.current;
      if (target.isErrand) {
        return inspectAndOpenErrand(
          store,
          target,
          ownedOpenRef.current,
          () =>
            mountedRef.current &&
            requestAttempt === requestAttemptRef.current &&
            navigationGeneration === navigationGenerationRef.current,
        );
      }

      const state = store.getState();
      const sessions = state.taskSessionsByTask?.itemsByTaskId?.[taskId] ?? [];
      const session =
        state.taskSessions?.items?.[sessionId] ?? sessions.find((item) => item.id === sessionId);
      if (session && session.task_id !== taskId) return false;
      return openOrdinaryTarget({
        target,
        session,
        ownerOpen: ownerOpenRef.current,
        taskId,
        sessionId,
        title,
        store,
        usesDesktopWorkbench,
      });
    },
    [store, usesDesktopWorkbench],
  );
}
