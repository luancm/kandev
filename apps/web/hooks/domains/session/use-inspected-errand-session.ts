import { useAppStore } from "@/components/state-provider";

/** Child details are eligible for explicit selection only within their owning task. */
export function useInspectedErrandSession(taskId: string, sessionId: string | null) {
  return useAppStore((state) => {
    const candidate = sessionId ? state.taskSessions?.items[sessionId] : undefined;
    return candidate?.task_id === taskId && candidate.role === "errand" ? candidate : null;
  });
}
