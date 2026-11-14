import { useCallback, useEffect, useRef } from "react";
import type { useAppStoreApi } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";

type AppStoreApi = ReturnType<typeof useAppStoreApi>;

export type LayoutApplyOwner = {
  taskId: string | null;
  sessionId: string | null;
  environmentId: string | null;
  isCurrent: () => boolean;
};

export function useLayoutApplyOwnership(
  appStore: AppStoreApi,
  renderedTaskId: string | null,
): () => LayoutApplyOwner | null {
  const attemptRef = useRef(0);
  const generationRef = useRef(0);
  const mountedRef = useRef(false);

  useEffect(() => {
    mountedRef.current = true;
    const unsubscribeAppStore = appStore.subscribe((state, previousState) => {
      if (
        state.tasks.activeTaskId !== previousState.tasks.activeTaskId ||
        state.tasks.activeSessionId !== previousState.tasks.activeSessionId
      ) {
        generationRef.current++;
      }
    });
    const unsubscribeDockviewStore = useDockviewStore.subscribe((state, previousState) => {
      if (
        state.currentLayoutEnvId !== previousState.currentLayoutEnvId ||
        state.api !== previousState.api
      ) {
        generationRef.current++;
      }
    });
    return () => {
      mountedRef.current = false;
      generationRef.current++;
      attemptRef.current++;
      unsubscribeAppStore();
      unsubscribeDockviewStore();
    };
  }, [appStore]);

  return useCallback(() => {
    const attempt = ++attemptRef.current;
    const generation = generationRef.current;
    const appState = appStore.getState();
    const taskId = appState.tasks.activeTaskId;
    if (taskId !== renderedTaskId) return null;

    const sessionId = appState.tasks.activeSessionId;
    const dockview = useDockviewStore.getState();
    return {
      taskId,
      sessionId,
      environmentId: dockview.currentLayoutEnvId,
      isCurrent: () => {
        const currentApp = appStore.getState();
        const currentDockview = useDockviewStore.getState();
        return (
          mountedRef.current &&
          attemptRef.current === attempt &&
          generationRef.current === generation &&
          currentApp.tasks.activeTaskId === taskId &&
          currentApp.tasks.activeSessionId === sessionId &&
          currentDockview.currentLayoutEnvId === dockview.currentLayoutEnvId &&
          currentDockview.api === dockview.api
        );
      },
    };
  }, [appStore, renderedTaskId]);
}
