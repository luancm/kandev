export type ReplaySettlementBoundary = {
  targetSessionId?: string;
};

export type ReplaySettlementObservation = {
  state: string;
  /** True only after the isolated backend restart has completed readiness. */
  backendRestarted: boolean;
};

const SETTLED_SESSION_STATES = new Set([
  "IDLE",
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "FAILED",
  "CANCELLED",
]);

/**
 * A settled state is only a replay observation after the causal restart
 * boundary. Without that boundary, an already-idle target would be a false
 * positive for queued replay work.
 */
export function hasCausalReplaySettlement({
  state,
  backendRestarted,
}: ReplaySettlementObservation): boolean {
  return backendRestarted && SETTLED_SESSION_STATES.has(state);
}

export function replaySettlementSessionId(
  boundary: ReplaySettlementBoundary,
  sourceSessionId: string,
): string {
  return boundary.targetSessionId ?? sourceSessionId;
}
