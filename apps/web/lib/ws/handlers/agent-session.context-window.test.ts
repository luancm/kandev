import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";
import { registerTaskSessionHandlers } from "./agent-session";

const TASK_ID = toTaskId("task-context-window-handler");
const SESSION_ID = toSessionId("session-context-window-handler");
const EARLIER_TIME = "2026-10-31T10:00:00.000Z";
const LATER_TIME = "2026-10-31T10:01:00.000Z";

function makeSession(metadata: Record<string, unknown>, updatedAt = EARLIER_TIME): TaskSession {
  return {
    id: SESSION_ID,
    task_id: TASK_ID,
    state: "RUNNING",
    started_at: EARLIER_TIME,
    updated_at: updatedAt,
    metadata,
  } as TaskSession;
}

function contextMetadata(used: number) {
  return {
    size: 200_000,
    used,
    remaining: 200_000 - used,
    efficiency: (used / 200_000) * 100,
    source: "acp",
  };
}

function sendStateChanged(store: ReturnType<typeof createAppStore>, payload: unknown): void {
  registerTaskSessionHandlers(store)["session.state_changed"]?.({
    id: "context-window-state-change",
    type: "notification",
    action: "session.state_changed",
    timestamp: LATER_TIME,
    payload,
  } as never);
}

describe("session state context-window ingestion", () => {
  it("normalizes both metadata aliases once with metadata precedence", () => {
    const store = createAppStore();
    store
      .getState()
      .setTaskSession(
        makeSession({ context_window: contextMetadata(68_000), context_compaction_count: 2 }),
      );
    let cachePublications = 0;
    store.subscribe((state, previous) => {
      if (
        state.contextWindow.bySessionId[SESSION_ID] !==
        previous.contextWindow.bySessionId[SESSION_ID]
      ) {
        cachePublications += 1;
      }
    });

    sendStateChanged(store, {
      task_id: TASK_ID,
      session_id: SESSION_ID,
      updated_at: LATER_TIME,
      metadata: { context_window: contextMetadata(82_000), context_compaction_count: 3 },
      session_metadata: {
        context_window: contextMetadata(55_000),
        context_compaction_count: 1,
        preserved_session_field: true,
      },
    });

    const state = store.getState();
    expect(state.taskSessions.items[SESSION_ID].metadata).toMatchObject({
      context_window: { used: 82_000 },
      context_compaction_count: 3,
      preserved_session_field: true,
    });
    expect(state.contextWindow.bySessionId[SESSION_ID]).toMatchObject({
      used: 82_000,
      compactionCount: 3,
    });
    expect(state.taskSessions.contextWindowEpochBySession?.[SESSION_ID]).toBe(1);
    expect(state.taskSessionsByTask.itemsByTaskId[TASK_ID][0].metadata).toMatchObject({
      context_window: { used: 82_000 },
      context_compaction_count: 3,
    });
    expect(cachePublications).toBe(1);
  });

  it("accepts an explicit clear from session_metadata when metadata omits context", () => {
    const store = createAppStore();
    store
      .getState()
      .setTaskSession(
        makeSession({ context_window: contextMetadata(68_000), context_compaction_count: 2 }),
      );

    sendStateChanged(store, {
      task_id: TASK_ID,
      session_id: SESSION_ID,
      updated_at: LATER_TIME,
      metadata: { unrelated: true },
      session_metadata: { context_window: null, context_compaction_count: 2 },
    });

    const state = store.getState();
    expect(state.taskSessions.items[SESSION_ID].metadata?.context_window).toBeNull();
    expect(state.contextWindow.bySessionId[SESSION_ID]).toBeUndefined();
    expect(state.taskSessionsByTask.itemsByTaskId[TASK_ID][0].metadata?.context_window).toBeNull();
    expect(state.taskSessions.contextWindowEpochBySession?.[SESSION_ID]).toBe(1);
  });

  it("rejects stale context events before any canonical or cache update", () => {
    const store = createAppStore();
    store
      .getState()
      .setTaskSession(
        makeSession(
          { context_window: contextMetadata(82_000), context_compaction_count: 3 },
          LATER_TIME,
        ),
      );
    const previousMetadata = store.getState().taskSessions.items[SESSION_ID].metadata;
    const previousEntry = store.getState().contextWindow.bySessionId[SESSION_ID];
    const previousEpoch = store.getState().taskSessions.contextWindowEpochBySession?.[SESSION_ID];

    sendStateChanged(store, {
      task_id: TASK_ID,
      session_id: SESSION_ID,
      updated_at: EARLIER_TIME,
      metadata: { context_window: null },
    });

    expect(store.getState().taskSessions.items[SESSION_ID].metadata).toBe(previousMetadata);
    expect(store.getState().contextWindow.bySessionId[SESSION_ID]).toBe(previousEntry);
    expect(store.getState().taskSessions.contextWindowEpochBySession?.[SESSION_ID]).toBe(
      previousEpoch,
    );
  });
});
