import type { AppState } from "../../app-state-types";
import { purgeSessionRuntimeState } from "../session-runtime/session-runtime-slice";
import { clearConversationCache } from "../session/conversation-cache";

/** Inspection owns cached child data; ordinary conversations and shared environments keep their owners. */
export function evictUnownedErrandConversation(state: AppState, sessionId: string): void {
  if (
    state.taskSessions?.items[sessionId]?.role !== "errand" ||
    state.tasks?.activeSessionId === sessionId ||
    Object.keys(state.errands.detailOwnersBySessionId[sessionId] ?? {}).length > 0
  ) {
    return;
  }
  clearConversationCache(state, sessionId);
  delete state.messagePrompts.generationBySession?.[sessionId];
  purgeSessionRuntimeState(state, sessionId);
}
