import { useMemo } from "react";
import { useInspectedErrandSession } from "@/hooks/domains/session/use-inspected-errand-session";
import type { TaskSession } from "@/lib/types/http";
import { pickActiveSessionId } from "./session-sort";

/** Explicit child inspection selects a conversation without changing normal tab membership. */
export function usePreviewConversation(
  taskId: string,
  sessionId: string | null,
  ordinarySessions: TaskSession[],
): TaskSession | null {
  const child = useInspectedErrandSession(taskId, sessionId);
  return useMemo(() => {
    if (child) return child;
    const selectedId = pickActiveSessionId(ordinarySessions, sessionId);
    return ordinarySessions.find((session) => session.id === selectedId) ?? null;
  }, [child, ordinarySessions, sessionId]);
}
