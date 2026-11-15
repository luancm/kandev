import type { AppState } from "../../app-state-types";
import { purgeSessionRuntimeState } from "../session-runtime/session-runtime-slice";

/** Inspection owns cached child data; ordinary conversations and shared environments keep their owners. */
export function evictUnownedErrandConversation(state: AppState, sessionId: string): void {
  if (
    state.taskSessions?.items[sessionId]?.role !== "errand" ||
    state.tasks?.activeSessionId === sessionId ||
    Object.keys(state.errands.detailOwnersBySessionId[sessionId] ?? {}).length > 0
  ) {
    return;
  }
  delete state.taskSessions.items[sessionId];
  delete state.taskSessions.activityEpochBySession?.[sessionId];
  delete state.taskSessions.readCursorEpochBySession?.[sessionId];
  delete state.taskSessions.contextWindowEpochBySession?.[sessionId];
  delete state.taskSessions.workspaceRecoveryEpochBySession?.[sessionId];
  delete state.messages.bySession[sessionId];
  delete state.messages.metaBySession[sessionId];
  delete state.messagePrompts.bySession[sessionId];
  delete state.messagePrompts.metaBySession[sessionId];
  delete state.messagePrompts.authoritativeBySession[sessionId];
  delete state.messagePrompts.observedBySession[sessionId];
  delete state.messagePrompts.deletedIdsBySession[sessionId];
  delete state.messagePrompts.generationBySession?.[sessionId];
  delete state.turns.bySession[sessionId];
  delete state.turns.activeBySession[sessionId];
  delete state.turns.windowCoverageBySession?.[sessionId];
  delete state.turns.loadedBySession[sessionId];
  delete state.turns.reconcileEpochBySession[sessionId];
  delete state.turns.settledBoundaryBySession[sessionId];
  delete state.turnChanges.bySession[sessionId];
  delete state.turnChanges.nextOffsetBySession[sessionId];
  delete state.turnChanges.loadedBySession[sessionId];
  delete state.pendingActionProjectionsBySessionId[sessionId];
  delete state.queue.bySessionId[sessionId];
  delete state.queue.metaBySessionId[sessionId];
  delete state.queue.activeOperationBySessionId[sessionId];
  purgeSessionRuntimeState(state, sessionId);
}
