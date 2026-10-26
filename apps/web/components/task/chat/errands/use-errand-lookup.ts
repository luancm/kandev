import { useAppStore } from "@/components/state-provider";
import { selectErrandBySessionId } from "@/lib/state/slices/errands/selectors";
import type { Errand } from "@/lib/types/errands";

/** Task of a session, falling back to the active task while the row is not loaded. */
export function useSessionTaskId(sessionId: string | null | undefined): string | null {
  return useAppStore(
    (state) =>
      (sessionId ? state.taskSessions.items[sessionId]?.task_id : undefined) ??
      state.tasks.activeTaskId ??
      null,
  );
}

/** The live errand row; present only while the errands flag delivered it. */
export function useLiveErrand(
  taskId: string | null | undefined,
  errandId: string | null | undefined,
): Errand | undefined {
  return useAppStore((state) =>
    taskId && errandId ? selectErrandBySessionId(state, taskId, errandId) : undefined,
  );
}
