import { isTaskDetailPath } from "./links";
import type { Errand } from "@/lib/types/errands";
import type { TaskSession } from "@/lib/types/http";

type ConversationSession = Pick<TaskSession, "id" | "role" | "parent_session_id">;
type ErrandParents = Record<string, Pick<Errand, "parent_session_id">>;

export function isErrandConversation(
  session: ConversationSession,
  errands: ErrandParents = {},
): boolean {
  return session.role === "errand" || Boolean(errands[session.id]);
}

/** Only a known normal parent may retain the conversation underneath a child. */
export function errandParentSessionId(
  selectedId: string | null,
  sessions: readonly ConversationSession[],
  errands: ErrandParents = {},
): string | null {
  const selected = sessions.find((session) => session.id === selectedId);
  if (!selected || !isErrandConversation(selected, errands)) return null;
  const parentId = selected.parent_session_id || errands[selected.id]?.parent_session_id;
  const parent = sessions.find((session) => session.id === parentId);
  return parent && !isErrandConversation(parent, errands) ? parent.id : null;
}

/** A child is selected as a conversation, while its normal parent owns the tab. */
export function conversationTabSessionId(
  selectedId: string | null,
  sessions: readonly ConversationSession[],
  errands: ErrandParents = {},
): string | null {
  if (!selectedId) return null;
  const selected = sessions.find((session) => session.id === selectedId);
  if (!selected || !isErrandConversation(selected, errands)) return selectedId;
  const normal = sessions.filter((session) => !isErrandConversation(session, errands));
  return errandParentSessionId(selectedId, sessions, errands) ?? normal[0]?.id ?? null;
}

/** Only the owning normal tab may show the currently selected child. */
export function panelConversationSessionId(
  hostId: string | null,
  selectedId: string | null,
  sessions: readonly ConversationSession[],
  errands: ErrandParents = {},
): string | null {
  const selected = sessions.find((session) => session.id === selectedId);
  if (
    selected &&
    isErrandConversation(selected, errands) &&
    conversationTabSessionId(selectedId, sessions, errands) === hostId
  ) {
    return selectedId;
  }
  return hostId;
}

/** Keep an explicit conversation choice reloadable without replacing the mounted task view. */
export function replaceTaskConversationUrl(taskId: string, sessionId: string): void {
  if (typeof window === "undefined" || !isTaskDetailPath(window.location.pathname, taskId)) return;
  const url = new URL(window.location.href);
  if (url.searchParams.get("sessionId") === sessionId) return;
  url.searchParams.set("sessionId", sessionId);
  window.history.replaceState(window.history.state, "", `${url.pathname}${url.search}${url.hash}`);
}
