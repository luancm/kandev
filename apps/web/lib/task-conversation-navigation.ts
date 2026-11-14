import { isTaskDetailPath } from "./links";
import type { Errand } from "@/lib/types/errands";
import type { TaskSession } from "@/lib/types/http";

type ConversationSession = {
  id: string;
  role?: TaskSession["role"];
  parent_session_id?: string;
};
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
  selectedDetail?: ConversationSession,
): string | null {
  const selected =
    sessions.find((session) => session.id === selectedId) ??
    (selectedDetail?.id === selectedId ? selectedDetail : undefined) ??
    (selectedId && errands[selectedId]
      ? {
          id: selectedId,
          role: "errand" as const,
          parent_session_id: errands[selectedId].parent_session_id,
        }
      : undefined);
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
  selectedDetail?: ConversationSession,
): string | null {
  if (!selectedId) return null;
  const selected =
    sessions.find((session) => session.id === selectedId) ??
    (selectedDetail?.id === selectedId ? selectedDetail : undefined) ??
    (errands[selectedId]
      ? {
          id: selectedId,
          role: "errand" as const,
          parent_session_id: errands[selectedId].parent_session_id,
        }
      : undefined);
  if (!selected) return null;
  if (!isErrandConversation(selected, errands)) return selectedId;
  return errandParentSessionId(selectedId, sessions, errands, selected);
}

/** Only the owning normal tab may show the currently selected child. */
export function panelConversationSessionId(
  hostId: string | null,
  selectedId: string | null,
  sessions: readonly ConversationSession[],
  errands: ErrandParents = {},
  selectedDetail?: ConversationSession,
): string | null {
  const selected =
    sessions.find((session) => session.id === selectedId) ??
    (selectedDetail?.id === selectedId ? selectedDetail : undefined) ??
    (selectedId && errands[selectedId]
      ? {
          id: selectedId,
          role: "errand" as const,
          parent_session_id: errands[selectedId].parent_session_id,
        }
      : undefined);
  if (
    selected &&
    isErrandConversation(selected, errands) &&
    conversationTabSessionId(selectedId, sessions, errands, selected) === hostId
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
