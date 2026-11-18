import type { SessionSliceState } from "./types";

/** Remove cached conversation data for a deleted task session. */
export function clearConversationCache(state: SessionSliceState, sessionId: string): void {
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
}
