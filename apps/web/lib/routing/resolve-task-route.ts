import { isErrandConversation } from "@/lib/task-conversation-navigation";
import { fetchTask, listOrdinaryTaskSessions } from "@/lib/api";
import { inspectErrand } from "@/lib/api/domains/errand-api";
import type { ErrandInspection } from "@/lib/types/errands";

async function resolveRequestedErrandInspection(
  taskId: string,
  sessionId: string,
): Promise<ErrandInspection> {
  const inspection = await inspectErrand(taskId, sessionId);
  if (
    inspection.scope.task_id !== taskId ||
    inspection.scope.child_session_id !== sessionId ||
    inspection.errand.task_id !== taskId ||
    inspection.errand.errand_id !== sessionId ||
    !inspection.child_session_exists
  ) {
    throw new Error("requested task session is unavailable");
  }
  return inspection;
}

export async function resolveTaskRoute(taskId: string, requestedSessionId?: string) {
  const [task, allSessionsResponse] = await Promise.all([
    fetchTask(taskId, { cache: "no-store" }),
    listOrdinaryTaskSessions(taskId, { cache: "no-store" }),
  ]);
  const sessions = allSessionsResponse.sessions ?? [];

  const ownedSessions = sessions.filter(
    (session) => session.task_id === taskId && !isErrandConversation(session),
  );
  const requestedSession = requestedSessionId
    ? ownedSessions.find((session) => session.id === requestedSessionId)
    : undefined;
  let selectedErrandInspection: ErrandInspection | undefined;
  if (requestedSessionId && !requestedSession) {
    selectedErrandInspection = await resolveRequestedErrandInspection(taskId, requestedSessionId);
  }
  const primarySession = task.primary_session_id
    ? ownedSessions.find((session) => session.id === task.primary_session_id)
    : undefined;
  const sessionId = requestedSessionId
    ? (requestedSession?.id ?? selectedErrandInspection?.scope.child_session_id)
    : (primarySession?.id ?? ownedSessions[0]?.id);
  return {
    task,
    sessionId,
    allSessionsResponse: { sessions: ownedSessions, total: ownedSessions.length },
    selectedErrandInspection,
  };
}
