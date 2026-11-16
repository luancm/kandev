import { act, cleanup, render } from "@testing-library/react";
import { createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { createAppStore } from "@/lib/state/store";
import type { AppState } from "@/lib/state/store";
import type { ContextWindowEntry } from "@/lib/state/slices/session-runtime/types";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";
import type { StoreApi } from "zustand";
import { useSessionContextWindow } from "@/hooks/domains/session/use-session-context-window";
import {
  captureTaskSessionHydrationEpoch,
  captureTaskSessionHydrationEpochs,
} from "./hydration-epochs";
import { registerTaskSessionHandlers } from "@/lib/ws/handlers/agent-session";
import { registerSessionModelsHandlers } from "@/lib/ws/handlers/session-models";

const transport = vi.hoisted(() => ({ unsubscribe: vi.fn(), subscribeSession: vi.fn() }));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ subscribeSession: transport.subscribeSession }),
}));

const TASK_ID = toTaskId("task-context-window");
const CHILD_ID = toSessionId("child-context-window");
const OTHER_CHILD_ID = toSessionId("other-child-context-window");
const SNAPSHOT_TIME = "2026-10-31T10:00:00.000Z";
const LATER_TIME = "2026-10-31T10:01:00.000Z";

function makeSession(
  sessionId: string,
  metadata: Record<string, unknown>,
  updatedAt = SNAPSHOT_TIME,
): TaskSession {
  return {
    id: toSessionId(sessionId),
    task_id: TASK_ID,
    state: "RUNNING",
    started_at: SNAPSHOT_TIME,
    updated_at: updatedAt,
    metadata,
  } as TaskSession;
}

function makeOtherSession(
  metadata: Record<string, unknown>,
  updatedAt = SNAPSHOT_TIME,
): TaskSession {
  return { ...makeSession(OTHER_CHILD_ID, metadata), updated_at: updatedAt };
}

function windowMetadata(used: number) {
  return {
    size: 200_000,
    used,
    remaining: 200_000 - used,
    efficiency: (used / 200_000) * 100,
    source: "acp",
  };
}

function StoreCapture({ capture }: { capture: (store: StoreApi<AppState>) => void }) {
  capture(useAppStoreApi());
  return null;
}

function ContextWindowProbe({
  capture,
}: {
  capture: (value: ContextWindowEntry | undefined) => void;
}) {
  capture(useSessionContextWindow(CHILD_ID, false));
  return null;
}

function renderContextWindowProbe(
  storeCapture: (store: StoreApi<AppState>) => void,
  probeCapture: (value: ContextWindowEntry | undefined) => void,
  showProbe = true,
) {
  return render(
    createElement(
      StateProvider,
      null,
      createElement(StoreCapture, { capture: storeCapture }),
      showProbe ? createElement(ContextWindowProbe, { capture: probeCapture }) : null,
    ),
  );
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function sendContextStateChanged(store: StoreApi<AppState>, payload: unknown): void {
  registerTaskSessionHandlers(store)["session.state_changed"]?.({
    id: "context-window-live-event",
    type: "notification",
    action: "session.state_changed",
    timestamp: LATER_TIME,
    payload,
  } as never);
}

afterEach(() => {
  cleanup();
  transport.unsubscribe.mockClear();
  transport.subscribeSession.mockClear();
});

describe("task-session context-window snapshots", () => {
  it("authoritative reconnect snapshot clears populated child context", () => {
    const store = createAppStore();
    store.getState().setTaskSessionsForTask(
      TASK_ID,
      [
        makeSession(CHILD_ID, {
          context_window: windowMetadata(68_000),
          context_compaction_count: 2,
        }),
        makeOtherSession({ context_window: windowMetadata(51_000), context_compaction_count: 1 }),
      ],
      {},
    );
    store.getState().setContextWindow(CHILD_ID, {
      size: 200_000,
      used: 68_000,
      remaining: 132_000,
      efficiency: 34,
      compactionCount: 2,
      source: "acp",
    });
    store.getState().setContextWindow(OTHER_CHILD_ID, {
      size: 200_000,
      used: 51_000,
      remaining: 149_000,
      efficiency: 25.5,
      compactionCount: 1,
      source: "acp",
    });
    const requestEpochs = captureTaskSessionHydrationEpochs(store.getState(), TASK_ID);

    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [
          makeSession(CHILD_ID, { context_window: null }),
          makeOtherSession({ context_window: windowMetadata(51_000), context_compaction_count: 1 }),
        ],
        requestEpochs,
      );

    const state = store.getState();
    expect(state.contextWindow.bySessionId[CHILD_ID]).toBeUndefined();
    expect(state.contextWindow.bySessionId[OTHER_CHILD_ID]?.used).toBe(51_000);
    expect(state.taskSessions.items[CHILD_ID].metadata?.context_window).toBeNull();
    expect(state.taskSessionsByTask.itemsByTaskId[TASK_ID][0].metadata?.context_window).toBeNull();
  });
});

describe("accepted task-session context snapshots", () => {
  it("accepted snapshot replaces cached child sample", () => {
    let store: StoreApi<AppState> | undefined;
    let observed: ContextWindowEntry | undefined;
    renderContextWindowProbe(
      (value) => {
        store = value;
      },
      (value) => {
        observed = value;
      },
    );
    const appStore = store;
    expect(appStore).toBeDefined();
    act(() => {
      appStore!.getState().setTaskSessionsForTask(
        TASK_ID,
        [
          makeSession(CHILD_ID, {
            context_window: windowMetadata(68_000),
            context_compaction_count: 2,
          }),
          makeOtherSession({ context_window: windowMetadata(39_000), context_compaction_count: 1 }),
        ],
        {},
      );
      appStore!.getState().setContextWindow(CHILD_ID, {
        size: 200_000,
        used: 68_000,
        remaining: 132_000,
        efficiency: 34,
        compactionCount: 2,
        source: "acp",
      });
      appStore!.getState().setContextWindow(OTHER_CHILD_ID, {
        size: 200_000,
        used: 39_000,
        remaining: 161_000,
        efficiency: 19.5,
        compactionCount: 1,
        source: "acp",
      });
    });
    const requestEpochs = captureTaskSessionHydrationEpochs(appStore!.getState(), TASK_ID);
    expect(observed?.used).toBe(68_000);

    act(() =>
      appStore!.getState().setTaskSessionsForTask(
        TASK_ID,
        [
          makeSession(CHILD_ID, {
            context_window: windowMetadata(82_000),
            context_compaction_count: 3,
          }),
          makeSession(OTHER_CHILD_ID, {
            context_window: windowMetadata(44_000),
            context_compaction_count: 1,
          }),
        ],
        requestEpochs,
      ),
    );

    expect(observed?.used).toBe(82_000);
    expect(appStore!.getState().contextWindow.bySessionId[CHILD_ID]).toMatchObject({
      used: 82_000,
      compactionCount: 3,
    });
    expect(appStore!.getState().contextWindow.bySessionId[OTHER_CHILD_ID]).toMatchObject({
      used: 44_000,
      compactionCount: 1,
    });
    expect(appStore!.getState().taskSessions.contextWindowEpochBySession).toEqual({
      [CHILD_ID]: 1,
      [OTHER_CHILD_ID]: 1,
    });
  });
});

describe("deferred context snapshots versus live events", () => {
  it("keeps a live clear when a measured request resolves afterward", async () => {
    const store = createAppStore();
    store.getState().setTaskSessionsForTask(
      TASK_ID,
      [
        makeSession(CHILD_ID, {
          context_window: windowMetadata(68_000),
          context_compaction_count: 2,
        }),
      ],
      {},
    );
    const requestEpochs = captureTaskSessionHydrationEpochs(store.getState(), TASK_ID);
    const response = deferred<TaskSession[]>();
    const pendingRefresh = response.promise.then((sessions) =>
      store.getState().setTaskSessionsForTask(TASK_ID, sessions, requestEpochs),
    );

    sendContextStateChanged(store, {
      task_id: TASK_ID,
      session_id: CHILD_ID,
      updated_at: SNAPSHOT_TIME,
      metadata: { context_window: null, context_compaction_count: 2 },
    });
    response.resolve([
      makeSession(CHILD_ID, {
        context_window: windowMetadata(68_000),
        context_compaction_count: 99,
      }),
    ]);
    await pendingRefresh;

    const state = store.getState();
    expect(state.taskSessions.items[CHILD_ID].metadata).toMatchObject({
      context_window: null,
      context_compaction_count: 2,
    });
    expect(state.contextWindow.bySessionId[CHILD_ID]).toBeUndefined();
    expect(state.taskSessionsByTask.itemsByTaskId[TASK_ID][0].metadata).toMatchObject({
      context_window: null,
      context_compaction_count: 2,
    });
    expect(state.taskSessions.contextWindowEpochBySession?.[CHILD_ID]).toBe(1);
  });

  it("keeps a live sample when an older explicit null resolves afterward", async () => {
    const store = createAppStore();
    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [makeSession(CHILD_ID, { context_window: null, context_compaction_count: 2 })],
        {},
      );
    const requestEpochs = captureTaskSessionHydrationEpochs(store.getState(), TASK_ID);
    const response = deferred<TaskSession[]>();
    const pendingRefresh = response.promise.then((sessions) =>
      store.getState().setTaskSessionsForTask(TASK_ID, sessions, requestEpochs),
    );

    sendContextStateChanged(store, {
      task_id: TASK_ID,
      session_id: CHILD_ID,
      updated_at: SNAPSHOT_TIME,
      metadata: { context_window: windowMetadata(82_000), context_compaction_count: 3 },
    });
    response.resolve([
      makeSession(CHILD_ID, { context_window: null, context_compaction_count: 2 }),
    ]);
    await pendingRefresh;

    const state = store.getState();
    expect(state.taskSessions.items[CHILD_ID].metadata).toMatchObject({
      context_window: { used: 82_000 },
      context_compaction_count: 3,
    });
    expect(state.contextWindow.bySessionId[CHILD_ID]).toMatchObject({
      used: 82_000,
      compactionCount: 3,
    });
    expect(
      state.taskSessionsByTask.itemsByTaskId[TASK_ID][0].metadata?.context_window,
    ).toMatchObject({
      used: 82_000,
    });
  });
});

describe("context snapshot row ordering", () => {
  it("uses row timestamp ordering while same-epoch snapshots remain acceptable", () => {
    const store = createAppStore();
    store.getState().setTaskSessionsForTask(
      TASK_ID,
      [
        makeSession(CHILD_ID, {
          context_window: windowMetadata(68_000),
          context_compaction_count: 2,
        }),
      ],
      {},
    );
    const requestEpochs = captureTaskSessionHydrationEpochs(store.getState(), TASK_ID);

    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [
          makeSession(
            CHILD_ID,
            { context_window: windowMetadata(82_000), context_compaction_count: 3, accepted: true },
            LATER_TIME,
          ),
        ],
        requestEpochs,
      );
    store.getState().setTaskSessionsForTask(
      TASK_ID,
      [
        makeSession(CHILD_ID, {
          context_window: windowMetadata(75_000),
          context_compaction_count: 4,
          stale_row: true,
        }),
      ],
      requestEpochs,
    );

    expect(store.getState().taskSessions.items[CHILD_ID].metadata).toMatchObject({
      context_window: { used: 82_000 },
      context_compaction_count: 3,
      stale_row: true,
    });
    expect(store.getState().contextWindow.bySessionId[CHILD_ID]).toMatchObject({
      used: 82_000,
      compactionCount: 3,
    });
    expect(store.getState().taskSessions.contextWindowEpochBySession?.[CHILD_ID]).toBeUndefined();

    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [
          makeSession(
            CHILD_ID,
            { context_window: windowMetadata(80_000), context_compaction_count: 4 },
            "2026-10-31T10:00:30.000Z",
          ),
        ],
        requestEpochs,
      );
    expect(store.getState().contextWindow.bySessionId[CHILD_ID]).toMatchObject({
      used: 82_000,
      compactionCount: 3,
    });
    expect(store.getState().taskSessions.items[CHILD_ID].updated_at).toBe(LATER_TIME);

    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [
          makeSession(
            CHILD_ID,
            { context_window: windowMetadata(91_000), context_compaction_count: 5 },
            "2026-10-31T10:02:00.000Z",
          ),
        ],
        requestEpochs,
      );
    expect(store.getState().contextWindow.bySessionId[CHILD_ID]).toMatchObject({
      used: 91_000,
      compactionCount: 5,
    });
    expect(store.getState().taskSessions.contextWindowEpochBySession?.[CHILD_ID]).toBeUndefined();
  });
});

describe("context invalidation lifecycle", () => {
  it("retains an absent context pair after a live invalidation without a session row", () => {
    const store = createAppStore();
    const requestEpoch = captureTaskSessionHydrationEpoch(store.getState(), CHILD_ID);
    store.getState().clearContextWindow(CHILD_ID);
    store.getState().setTaskSession(makeSession(CHILD_ID, { unrelated_live_field: true }));
    store.getState().setTaskSession(
      makeSession(CHILD_ID, {
        context_window: windowMetadata(68_000),
        context_compaction_count: 2,
      }),
      requestEpoch,
    );

    const state = store.getState();
    expect(Object.hasOwn(state.taskSessions.items[CHILD_ID].metadata ?? {}, "context_window")).toBe(
      false,
    );
    expect(state.taskSessions.items[CHILD_ID].metadata?.unrelated_live_field).toBe(true);
    expect(state.contextWindow.bySessionId[CHILD_ID]).toBeUndefined();
  });

  it("does not seed context from a stale snapshot when the session row is still absent", () => {
    const store = createAppStore();
    const requestEpoch = captureTaskSessionHydrationEpoch(store.getState(), CHILD_ID);
    store.getState().clearContextWindow(CHILD_ID);

    store.getState().setTaskSessionsForTask(
      TASK_ID,
      [
        makeSession(CHILD_ID, {
          context_window: windowMetadata(68_000),
          context_compaction_count: 2,
        }),
      ],
      { [CHILD_ID]: requestEpoch },
    );

    const state = store.getState();
    expect(Object.hasOwn(state.taskSessions.items[CHILD_ID].metadata ?? {}, "context_window")).toBe(
      false,
    );
    expect(state.contextWindow.bySessionId[CHILD_ID]).toBeUndefined();
    expect(state.taskSessionsByTask.itemsByTaskId[TASK_ID][0].metadata).not.toHaveProperty(
      "context_window",
    );
  });

  it("increments the live epoch once and purges it with the session", () => {
    const store = createAppStore();
    store
      .getState()
      .setTaskSession(makeSession(CHILD_ID, { context_window: windowMetadata(68_000) }));
    expect(store.getState().taskSessions.contextWindowEpochBySession?.[CHILD_ID]).toBeUndefined();

    store.getState().setContextWindow(CHILD_ID, {
      size: 200_000,
      used: 82_000,
      remaining: 118_000,
      efficiency: 41,
      compactionCount: 3,
      source: "api",
    });
    store.getState().clearContextWindow(CHILD_ID);
    store.getState().setContextWindow(OTHER_CHILD_ID, {
      size: 200_000,
      used: 51_000,
      remaining: 149_000,
      efficiency: 25.5,
      compactionCount: 1,
      source: "acp",
    });

    expect(store.getState().taskSessions.contextWindowEpochBySession).toEqual({
      [CHILD_ID]: 2,
      [OTHER_CHILD_ID]: 1,
    });
    store.getState().removeTaskSession(TASK_ID, CHILD_ID);
    expect(store.getState().taskSessions.contextWindowEpochBySession).toEqual({
      [OTHER_CHILD_ID]: 1,
    });
  });
});

describe("omitted task-session context metadata", () => {
  it("omitted context leaves reading unchanged", () => {
    const store = createAppStore();
    store.getState().setTaskSessionsForTask(
      TASK_ID,
      [
        makeSession(CHILD_ID, {
          context_window: windowMetadata(68_000),
          context_compaction_count: 2,
        }),
      ],
      {},
    );
    const previous = store.getState().contextWindow.bySessionId[CHILD_ID];

    store
      .getState()
      .setTaskSessionsForTask(TASK_ID, [makeSession(CHILD_ID, { unrelated: "metadata" })], {});

    expect(store.getState().contextWindow.bySessionId[CHILD_ID]).toBe(previous);
    expect(store.getState().contextWindow.bySessionId[CHILD_ID]?.used).toBe(68_000);

    const empty = createAppStore();
    empty
      .getState()
      .setTaskSessionsForTask(TASK_ID, [makeSession(CHILD_ID, { unrelated: "metadata" })], {});
    expect(empty.getState().contextWindow.bySessionId[CHILD_ID]).toBeUndefined();
  });
});

describe("context-window row lifecycle", () => {
  it("retained clear cannot rehydrate on row remount", () => {
    let store: StoreApi<AppState> | undefined;
    let observed: ContextWindowEntry | undefined;
    const view = renderContextWindowProbe(
      (value) => {
        store = value;
      },
      (value) => {
        observed = value;
      },
    );
    const appStore = store;
    expect(appStore).toBeDefined();

    act(() => {
      appStore!.getState().setTaskSession(
        makeSession(CHILD_ID, {
          context_window: windowMetadata(68_000),
          context_compaction_count: 2,
        }),
      );
    });
    expect(observed?.used).toBe(68_000);
    expect(
      appStore!.getState().taskSessions.contextWindowEpochBySession?.[CHILD_ID],
    ).toBeUndefined();
    appStore!.getState().setSessionModels(CHILD_ID, {
      currentModelId: "previous-model",
      models: [],
      configOptions: [],
    });

    act(() =>
      appStore!.getState().setContextWindow(CHILD_ID, {
        size: 200_000,
        used: 82_000,
        remaining: 118_000,
        efficiency: 41,
        compactionCount: 3,
        source: "api",
      }),
    );
    expect(observed?.used).toBe(82_000);
    expect(appStore!.getState().taskSessions.contextWindowEpochBySession?.[CHILD_ID]).toBe(1);
    expect(appStore!.getState().taskSessions.items[CHILD_ID].metadata).toMatchObject({
      context_window: { used: 82_000, source: "api" },
      context_compaction_count: 3,
    });

    act(() =>
      registerSessionModelsHandlers(appStore!)["session.models_updated"]?.({
        id: "context-window-model-change",
        type: "notification",
        action: "session.models_updated",
        payload: {
          task_id: TASK_ID,
          session_id: CHILD_ID,
          agent_id: "agent-context-window",
          current_model_id: "replacement-model",
          models: [],
          config_options: [],
          config_options_settled: true,
          timestamp: LATER_TIME,
        },
      } as never),
    );
    expect(observed).toBeUndefined();
    expect(appStore!.getState().taskSessions.contextWindowEpochBySession?.[CHILD_ID]).toBe(2);

    view.rerender(
      createElement(
        StateProvider,
        null,
        createElement(StoreCapture, {
          capture: (value) => {
            store = value;
          },
        }),
        null,
      ),
    );
    observed = undefined;
    view.rerender(
      createElement(
        StateProvider,
        null,
        createElement(StoreCapture, {
          capture: (value) => {
            store = value;
          },
        }),
        createElement(ContextWindowProbe, {
          capture: (value) => {
            observed = value;
          },
        }),
      ),
    );

    expect(observed).toBeUndefined();
    expect(appStore!.getState().taskSessions.items[CHILD_ID].metadata?.context_window).toBeNull();
    expect(transport.subscribeSession).not.toHaveBeenCalled();
  });
});
