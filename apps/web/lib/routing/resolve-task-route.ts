import { isErrandConversation } from "@/lib/task-conversation-navigation";
import { fetchTask, listTaskSessions } from "@/lib/api";

export async function resolveTaskRoute(taskId: string, requestedSessionId?: string) {
  const [task, allSessionsResponse] = await Promise.all([
    fetchTask(taskId, { cache: "no-store" }),
    listTaskSessions(taskId, { cache: "no-store" }),
  ]);
  const sessions = allSessionsResponse.sessions ?? [];

  const ownedSessions = sessions.filter((session) => session.task_id === taskId);
  const requestedSession = requestedSessionId
    ? ownedSessions.find((session) => session.id === requestedSessionId)
    : undefined;
  const normalSessions = ownedSessions.filter((session) => !isErrandConversation(session));
  const primarySession = task.primary_session_id
    ? normalSessions.find((session) => session.id === task.primary_session_id)
    : undefined;
  const sessionId = requestedSession?.id ?? primarySession?.id ?? normalSessions[0]?.id;
  return {
    task,
    sessionId,
    allSessionsResponse: { sessions: ownedSessions, total: ownedSessions.length },
  };
}
