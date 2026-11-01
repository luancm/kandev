import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { AppState } from "@/lib/state/store";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";
import {
  captureTaskSessionHydrationEpoch,
  captureTaskSessionHydrationEpochs,
} from "../slices/session/hydration-epochs";

const TASK_ID = toTaskId("task-hydrator-context-window");
const SESSION_ID = toSessionId("session-hydrator-context-window");
const SNAPSHOT_TIME = "2026-10-31T10:00:00.000Z";

function contextWindow(used: number) {
  return {
    size: 200_000,
    used,
    remaining: 200_000 - used,
    efficiency: (used / 200_000) * 100,
    source: "acp",
  };
}

function session(metadata: Record<string, unknown>, updatedAt = SNAPSHOT_TIME): TaskSession {
  return {
    id: SESSION_ID,
    task_id: TASK_ID,
    state: "RUNNING",
    started_at: SNAPSHOT_TIME,
    updated_at: updatedAt,
    metadata,
  } as TaskSession;
}

function hydrateTaskList(
  store: ReturnType<typeof createAppStore>,
  rows: TaskSession[],
  options?: Parameters<AppState["hydrate"]>[1],
): void {
  store.getState().hydrate(
    {
      taskSessionsByTask: {
        itemsByTaskId: { [TASK_ID]: rows },
        loadingByTaskId: { [TASK_ID]: false },
        loadedByTaskId: { [TASK_ID]: true },
      },
    } as unknown as Parameters<AppState["hydrate"]>[0],
    options,
  );
}

describe("route context-window hydration", () => {
  it("keeps a live clear when a full route snapshot resolves afterward", () => {
    const store = createAppStore();
    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [session({ context_window: contextWindow(68_000), context_compaction_count: 2 })],
        {},
      );
    const requestEpochs = captureTaskSessionHydrationEpochs(store.getState(), TASK_ID);
    store.getState().clearContextWindow(SESSION_ID);

    const routeSnapshot = session({
      context_window: contextWindow(68_000),
      context_compaction_count: 2,
      route_field: true,
    });
    store.getState().hydrate(
      {
        taskSessions: { items: { [SESSION_ID]: routeSnapshot } },
        taskSessionsByTask: {
          itemsByTaskId: { [TASK_ID]: [routeSnapshot] },
          loadingByTaskId: { [TASK_ID]: false },
          loadedByTaskId: { [TASK_ID]: true },
        },
      } as unknown as Parameters<AppState["hydrate"]>[0],
      { taskSessionHydrationEpochsAtRequestStart: requestEpochs },
    );

    const state = store.getState();
    expect(state.taskSessions.items[SESSION_ID].metadata).toMatchObject({
      context_window: null,
      route_field: true,
    });
    expect(state.taskSessionsByTask.itemsByTaskId[TASK_ID][0].metadata?.context_window).toBeNull();
    expect(state.contextWindow.bySessionId[SESSION_ID]).toBeUndefined();
    expect(state.taskSessions.contextWindowEpochBySession?.[SESSION_ID]).toBe(1);
  });

  it("keeps canonical, list mirror, and cache aligned for a list-only stale context row", () => {
    const store = createAppStore();
    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [session({ context_window: contextWindow(68_000), context_compaction_count: 2 })],
        {},
      );
    const requestEpochs = captureTaskSessionHydrationEpochs(store.getState(), TASK_ID);
    store.getState().clearContextWindow(SESSION_ID);

    hydrateTaskList(
      store,
      [
        session({
          context_window: contextWindow(68_000),
          context_compaction_count: 2,
          list_field: true,
        }),
      ],
      { taskSessionHydrationEpochsAtRequestStart: requestEpochs },
    );

    const state = store.getState();
    expect(state.taskSessions.items[SESSION_ID].metadata).toMatchObject({ context_window: null });
    expect(state.taskSessionsByTask.itemsByTaskId[TASK_ID][0].metadata).toMatchObject({
      context_window: null,
      list_field: true,
    });
    expect(state.contextWindow.bySessionId[SESSION_ID]).toBeUndefined();
    expect(state.taskSessions.contextWindowEpochBySession?.[SESSION_ID]).toBe(1);
    expect(requestEpochs[SESSION_ID]?.contextWindow).toBe(0);
  });
});

describe("accepted context-window hydration", () => {
  it("publishes a list-only accepted context row to the runtime cache", () => {
    const store = createAppStore();

    hydrateTaskList(store, [
      session({ context_window: contextWindow(82_000), context_compaction_count: 3 }),
    ]);

    expect(store.getState().taskSessions.items[SESSION_ID]).toBeUndefined();
    expect(store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID][0].metadata).toMatchObject({
      context_window: { used: 82_000 },
      context_compaction_count: 3,
    });
    expect(store.getState().contextWindow.bySessionId[SESSION_ID]).toMatchObject({
      used: 82_000,
      compactionCount: 3,
    });
  });

  it("does not seed a list-only row from a snapshot predating a rowless clear", () => {
    const store = createAppStore();
    const requestEpoch = captureTaskSessionHydrationEpoch(store.getState(), SESSION_ID);
    store.getState().clearContextWindow(SESSION_ID);

    hydrateTaskList(
      store,
      [session({ context_window: contextWindow(82_000), context_compaction_count: 3 })],
      { taskSessionHydrationEpochsAtRequestStart: { [SESSION_ID]: requestEpoch } },
    );

    expect(
      store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID][0].metadata,
    ).not.toHaveProperty("context_window");
    expect(store.getState().contextWindow.bySessionId[SESSION_ID]).toBeUndefined();
  });

  it("publishes an accepted full route context row and mirror", () => {
    const store = createAppStore();
    const routeSnapshot = session({
      context_window: contextWindow(82_000),
      context_compaction_count: 3,
    });

    store.getState().hydrate({
      taskSessions: { items: { [SESSION_ID]: routeSnapshot } },
      taskSessionsByTask: {
        itemsByTaskId: { [TASK_ID]: [routeSnapshot] },
        loadingByTaskId: { [TASK_ID]: false },
        loadedByTaskId: { [TASK_ID]: true },
      },
    } as unknown as Parameters<AppState["hydrate"]>[0]);

    expect(store.getState().taskSessions.items[SESSION_ID].metadata?.context_window).toMatchObject({
      used: 82_000,
    });
    expect(
      store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID][0].metadata?.context_window,
    ).toMatchObject({
      used: 82_000,
    });
    expect(store.getState().contextWindow.bySessionId[SESSION_ID]).toMatchObject({
      used: 82_000,
      compactionCount: 3,
    });
  });
});
