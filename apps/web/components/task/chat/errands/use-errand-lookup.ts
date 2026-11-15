import { useAppStore } from "@/components/state-provider";
import { selectErrandBrowserRowBySessionId } from "@/lib/state/slices/errands/selectors";
import type { ErrandBrowserRow } from "@/lib/types/errands";

/** Task of a session, falling back to the active task while the row is not loaded. */
export function useSessionTaskId(sessionId: string | null | undefined): string | null {
  return useAppStore(
    (state) =>
      (sessionId ? state.taskSessions.items[sessionId]?.task_id : undefined) ??
      state.tasks.activeTaskId ??
      null,
  );
}

/** The bounded current errand row; historical results do not trigger a full list read. */
export function useLiveErrand(
  taskId: string | null | undefined,
  errandId: string | null | undefined,
): ErrandBrowserRow | undefined {
  return useAppStore((state) =>
    taskId && errandId ? selectErrandBrowserRowBySessionId(state, taskId, errandId) : undefined,
  );
}
