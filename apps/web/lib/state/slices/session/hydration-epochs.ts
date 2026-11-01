import type { SessionSliceState, TaskSessionHydrationEpoch } from "./types";
import type { TaskSession } from "@/lib/types/http";
import type { ContextWindowEntry } from "@/lib/state/slices/session-runtime/types";
import {
  parseContextWindowEntry,
  readContextWindowMetadata,
} from "@/lib/state/slices/session-runtime/context-window";
import { isStaleSessionStateEvent } from "@/lib/session-state";

type SessionHydrationState = Pick<SessionSliceState, "taskSessions" | "taskSessionsByTask">;
type SessionEpochState = Pick<SessionSliceState, "taskSessions">;

const CONTEXT_WINDOW_PAIR = ["context_window", "context_compaction_count"] as const;

type ContextWindowCacheState = {
  contextWindow: { bySessionId: Record<string, ContextWindowEntry> };
};

function metadataRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function hasContextWindowPair(metadata: Record<string, unknown>): boolean {
  return CONTEXT_WINDOW_PAIR.some((key) => Object.prototype.hasOwnProperty.call(metadata, key));
}

function copyContextWindowPair(
  targetMetadata: TaskSession["metadata"],
  sourceMetadata: TaskSession["metadata"],
): TaskSession["metadata"] {
  const target = metadataRecord(targetMetadata);
  const source = metadataRecord(sourceMetadata);
  if (!hasContextWindowPair(target) && !hasContextWindowPair(source)) return targetMetadata;

  const next = { ...target };
  for (const key of CONTEXT_WINDOW_PAIR) {
    delete next[key];
    if (Object.prototype.hasOwnProperty.call(source, key)) next[key] = source[key];
  }
  return next;
}

/** Keep only context metadata from the accepted row while leaving its other metadata intact. */
export function copyAcceptedTaskSessionContextPair(
  target: TaskSession,
  source: TaskSession,
): TaskSession {
  const targetMetadata = metadataRecord(target.metadata);
  const sourceMetadata = metadataRecord(source.metadata);
  const hasDifference = CONTEXT_WINDOW_PAIR.some((key) => {
    const targetHas = Object.prototype.hasOwnProperty.call(targetMetadata, key);
    const sourceHas = Object.prototype.hasOwnProperty.call(sourceMetadata, key);
    return (
      targetHas !== sourceHas || (targetHas && !Object.is(targetMetadata[key], sourceMetadata[key]))
    );
  });
  if (!hasDifference) return target;
  return { ...target, metadata: copyContextWindowPair(target.metadata, source.metadata) };
}

/** Apply a snapshot's context pair only if no later live write or newer row has won. */
export function resolveTaskSessionContextSnapshot(
  existing: TaskSession | undefined,
  incoming: TaskSession,
  currentContextWindowEpoch: number,
  requestEpoch: TaskSessionHydrationEpoch | undefined,
): TaskSession {
  const incomingContext = readContextWindowMetadata(incoming.metadata);
  const contextChangedDuringRequest = hydrationEpochChanged(
    currentContextWindowEpoch,
    requestEpoch?.contextWindow,
  );
  if (!existing) {
    if (!incomingContext.present || !contextChangedDuringRequest) return incoming;
    return { ...incoming, metadata: copyContextWindowPair(incoming.metadata, undefined) };
  }
  const staleRow = isStaleSessionStateEvent(existing, incoming.updated_at);
  if (incomingContext.present && !contextChangedDuringRequest && !staleRow) return incoming;

  const metadata = copyContextWindowPair(incoming.metadata, existing.metadata);
  if (metadata === incoming.metadata && !staleRow) return incoming;
  return {
    ...incoming,
    ...(metadata !== incoming.metadata ? { metadata } : {}),
    ...(staleRow ? { updated_at: existing.updated_at } : {}),
  };
}

/** Publish an explicitly supplied context pair to the runtime cache from its accepted row. */
export function synchronizeAcceptedContextWindow(
  state: ContextWindowCacheState,
  incoming: TaskSession,
  accepted: TaskSession,
): void {
  if (!readContextWindowMetadata(incoming.metadata).present) return;
  const context = readContextWindowMetadata(accepted.metadata);
  if (!context.present) {
    delete state.contextWindow.bySessionId[accepted.id];
    return;
  }

  const entry = parseContextWindowEntry(context.contextWindow, undefined, context.compactionCount);
  if (entry) state.contextWindow.bySessionId[accepted.id] = entry;
  else delete state.contextWindow.bySessionId[accepted.id];
}

/** Apply the existing generation rule used by snapshot hydration. */
export function hydrationEpochChanged(current: number, request: number | undefined): boolean {
  if (request === undefined) return current > 0;
  return current > request;
}

/** Capture the live activity and read-cursor generations for one session. */
export function captureTaskSessionHydrationEpoch(
  state: SessionEpochState,
  sessionId: string,
): TaskSessionHydrationEpoch {
  return {
    activity: state.taskSessions.activityEpochBySession?.[sessionId] ?? 0,
    readCursor: state.taskSessions.readCursorEpochBySession?.[sessionId] ?? 0,
    contextWindow: state.taskSessions.contextWindowEpochBySession?.[sessionId] ?? 0,
    workspaceRecovery: state.taskSessions.workspaceRecoveryEpochBySession?.[sessionId] ?? 0,
  };
}

/** Capture the live activity and read-cursor generations at list request start. */
export function captureTaskSessionHydrationEpochs(
  state: SessionHydrationState,
  taskId: string,
): Readonly<Record<string, TaskSessionHydrationEpoch>> {
  const sessions = state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
  return Object.fromEntries(
    sessions.map((session) => [session.id, captureTaskSessionHydrationEpoch(state, session.id)]),
  );
}
