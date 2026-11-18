import type { ErrandInspection } from "@/lib/types/errands";
import type { Task, TaskSession } from "@/lib/types/http";

export function requestedOwnedErrand(
  taskId: string,
  requestedId: string | undefined,
  inspection?: ErrandInspection,
): ErrandInspection | undefined {
  if (
    !requestedId ||
    inspection?.owns_conversation !== true ||
    inspection.scope.task_id !== taskId ||
    inspection.scope.child_session_id !== requestedId ||
    inspection.errand.task_id !== taskId ||
    inspection.errand.errand_id !== requestedId
  ) {
    return undefined;
  }
  const parent = inspection.parent;
  if (
    parent.available &&
    (!parent.id || parent.id !== inspection.errand.parent_session_id || parent.role === "errand")
  ) {
    return undefined;
  }
  return inspection;
}

export function resolveTaskSessionId(
  task: Task,
  sessions: TaskSession[],
  requestedSessionId?: string,
  selectedErrandInspection?: ErrandInspection,
): string | null {
  const ownedSessions = sessions.filter((session) => session.task_id === task.id);
  if (requestedSessionId) {
    if (ownedSessions.some((session) => session.id === requestedSessionId))
      return requestedSessionId;
    if (inspectionMatchesRequestedChild(task.id, requestedSessionId, selectedErrandInspection)) {
      return requestedSessionId;
    }
    return null;
  }
  const primarySession = task.primary_session_id
    ? ownedSessions.find((session) => session.id === task.primary_session_id)
    : undefined;
  return primarySession?.id ?? ownedSessions[0]?.id ?? null;
}

export function matchingActiveSession(
  session: TaskSession | null,
  task: Task,
  sessionId: string,
  expectedErrandParentSessionId?: string,
): TaskSession | null {
  if (!session || session.id !== sessionId || session.task_id !== task.id) return null;
  if (
    expectedErrandParentSessionId &&
    (session.role !== "errand" || session.parent_session_id !== expectedErrandParentSessionId)
  ) {
    return null;
  }
  return session;
}

function inspectionMatchesRequestedChild(
  taskId: string,
  requestedSessionId: string,
  inspection?: ErrandInspection,
): boolean {
  return Boolean(
    inspection?.scope.task_id === taskId &&
    inspection.scope.child_session_id === requestedSessionId &&
    inspection.errand.task_id === taskId &&
    inspection.errand.errand_id === requestedSessionId &&
    inspection.child_session_exists,
  );
}
