import { createElement, type ReactNode } from "react";
import { act, cleanup, render, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { makeErrand } from "@/lib/state/slices/errands/errands.test-helpers";
import type { Errand } from "@/lib/types/errands";

const api = vi.hoisted(() => ({ listErrands: vi.fn() }));
vi.mock("@/lib/api/domains/errand-api", () => api);

import { useTaskErrands } from "./use-task-errands";

const TASK_ID = "task-1";

function wrapperFor(errandsEnabled: boolean) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(StateProvider, {
      initialState: { features: { ...defaultState.features, errands: errandsEnabled } },
      children,
    });
  };
}

function useConsumers() {
  return {
    store: useAppStoreApi(),
    first: useTaskErrands(TASK_ID),
    second: useTaskErrands(TASK_ID),
  };
}

type ErrandsConsumerProbeProps = {
  onStore?: (store: ReturnType<typeof useAppStoreApi>) => void;
  onValue?: (value: ReturnType<typeof useTaskErrands>) => void;
};

function ErrandsConsumerProbe({ onStore, onValue }: ErrandsConsumerProbeProps) {
  const store = useAppStoreApi();
  const value = useTaskErrands(TASK_ID);
  onStore?.(store);
  onValue?.(value);
  return null;
}

function connect(store: ReturnType<typeof useAppStoreApi>) {
  act(() => store.getState().setConnectionStatus("connected"));
}

function authFor(userId: string) {
  return {
    mode: "enabled" as const,
    authenticated: true,
    user: {
      id: userId,
      email: `${userId}@example.test`,
      display_name: userId,
      role: "user",
      status: "active",
    },
  };
}

afterEach(cleanup);

describe("useTaskErrands", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listErrands.mockResolvedValue([
      makeErrand({ errand_id: "b", created_at: "2026-10-26T10:00:02Z", status: "done" }),
      makeErrand({ errand_id: "a", created_at: "2026-10-26T10:00:01Z" }),
    ]);
  });

  it("loads errand.list once for every consumer of a task", async () => {
    const { result } = renderHook(useConsumers, { wrapper: wrapperFor(true) });
    connect(result.current.store);

    await waitFor(() => expect(result.current.first.isLoaded).toBe(true));
    expect(api.listErrands).toHaveBeenCalledExactlyOnceWith(TASK_ID);
    expect(result.current.first.errands.map((errand) => errand.errand_id)).toEqual(["a", "b"]);
    expect(result.current.first.liveCount).toBe(1);
    expect(Object.keys(result.current.first.errandsByParent)).toEqual(["session-parent"]);
    expect(result.current.second.errands).toBe(result.current.first.errands);
  });

  it("neither loads nor exposes errands while the flag is off", async () => {
    const { result } = renderHook(useConsumers, { wrapper: wrapperFor(false) });
    act(() => result.current.store.getState().upsertErrand(makeErrand()));
    connect(result.current.store);

    expect(api.listErrands).not.toHaveBeenCalled();
    expect(result.current.first).toEqual({
      errands: [],
      errandsByParent: {},
      liveCount: 0,
      isLoaded: false,
    });
  });

  it("refreshes the task's errands after a reconnect", async () => {
    const { result } = renderHook(useConsumers, { wrapper: wrapperFor(true) });
    connect(result.current.store);
    await waitFor(() => expect(result.current.first.isLoaded).toBe(true));

    api.listErrands.mockResolvedValue([makeErrand({ errand_id: "a", status: "cancelled" })]);
    act(() => result.current.store.getState().setConnectionStatus("disconnected"));
    connect(result.current.store);

    await waitFor(() => expect(result.current.first.errands).toHaveLength(1));
    expect(api.listErrands).toHaveBeenCalledTimes(2);
    expect(result.current.first.liveCount).toBe(0);
  });

  it("keeps an errand that started live while the list request was in flight", async () => {
    let resolveList: (errands: Errand[]) => void = () => {};
    api.listErrands.mockReturnValue(
      new Promise<Errand[]>((resolve) => {
        resolveList = resolve;
      }),
    );
    const { result } = renderHook(useConsumers, { wrapper: wrapperFor(true) });
    connect(result.current.store);
    expect(api.listErrands).toHaveBeenCalledTimes(1);

    act(() => result.current.store.getState().upsertErrand(makeErrand({ errand_id: "live" })));
    await act(async () => resolveList([makeErrand({ errand_id: "listed" })]));

    expect(result.current.first.errands.map((errand) => errand.errand_id).sort()).toEqual([
      "listed",
      "live",
    ]);
  });

  it("stays unloaded after a failed load so the next reconnect retries", async () => {
    api.listErrands.mockRejectedValueOnce(new Error("feature_disabled"));
    const { result } = renderHook(useConsumers, { wrapper: wrapperFor(true) });
    connect(result.current.store);
    await waitFor(() => expect(api.listErrands).toHaveBeenCalledTimes(1));

    expect(result.current.first.isLoaded).toBe(false);
    act(() => result.current.store.getState().setConnectionStatus("disconnected"));
    connect(result.current.store);
    await waitFor(() => expect(result.current.first.isLoaded).toBe(true));
  });
});

function deferredList() {
  let resolve!: (rows: Errand[]) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<Errand[]>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function useSelectedTask(taskId: string | null) {
  return { store: useAppStoreApi(), value: useTaskErrands(taskId) };
}

// @covers AC-TASKS-ERRANDS-002.1, AC-TASKS-ERRANDS-010.4
// @covers AC-TASKS-ERRANDS-010.7, AC-TASKS-ERRANDS-010.11
describe("task errand snapshot ownership", () => {
  beforeEach(() => {
    api.listErrands.mockReset();
    api.listErrands.mockResolvedValue([makeErrand()]);
  });

  it("refreshes cached A after reconnect while B was selected", async () => {
    api.listErrands
      .mockResolvedValueOnce([makeErrand({ errand_id: "a-before" })])
      .mockResolvedValueOnce([makeErrand({ task_id: "task-b", errand_id: "b-before" })])
      .mockResolvedValueOnce([makeErrand({ task_id: "task-b", errand_id: "b-after" })])
      .mockResolvedValueOnce([makeErrand({ errand_id: "a-after" })]);
    const hook = renderHook(useSelectedTask, { initialProps: TASK_ID, wrapper: wrapperFor(true) });
    connect(hook.result.current.store);
    await waitFor(() =>
      expect(hook.result.current.value.errands.map((row) => row.errand_id)).toEqual(["a-before"]),
    );
    hook.rerender("task-b");
    await waitFor(() => expect(api.listErrands).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(hook.result.current.value.errands.map((row) => row.errand_id)).toEqual(["b-before"]),
    );
    act(() => {
      hook.result.current.store.getState().setConnectionStatus("disconnected");
      hook.result.current.store.getState().setConnectionStatus("connected");
    });
    await waitFor(() => expect(api.listErrands).toHaveBeenCalledTimes(3));
    await waitFor(() =>
      expect(hook.result.current.value.errands.map((row) => row.errand_id)).toEqual(["b-after"]),
    );
    hook.rerender(TASK_ID);
    await waitFor(() =>
      expect(hook.result.current.value.errands.map((row) => row.errand_id)).toEqual(["a-after"]),
    );
    expect(api.listErrands).toHaveBeenCalledTimes(4);
  });

  it("starts a replacement read at reconnect and rejects the previous transport response", async () => {
    const old = deferredList();
    const fresh = deferredList();
    api.listErrands.mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise);
    const { result } = renderHook(useConsumers, { wrapper: wrapperFor(true) });
    connect(result.current.store);
    act(() => {
      result.current.store.getState().setConnectionStatus("disconnected");
      result.current.store.getState().setConnectionStatus("connected");
    });
    expect(api.listErrands).toHaveBeenCalledTimes(2);
    await act(async () => fresh.resolve([makeErrand({ errand_id: "fresh" })]));
    await act(async () => old.resolve([makeErrand({ errand_id: "obsolete" })]));
    expect(result.current.first.errands.map((row) => row.errand_id)).toEqual(["fresh"]);
    const state = result.current.store.getState();
    expect(state.errands.loadedConnectionGenerationByTaskId[TASK_ID]).toBe(
      state.connection.generation,
    );
    expect(state.errands.loadedReadScopeByTaskId[TASK_ID]).toBeDefined();
  });

  it("does not stamp or publish a response after disconnect", async () => {
    const request = deferredList();
    api.listErrands.mockReturnValueOnce(request.promise);
    const { result } = renderHook(useConsumers, { wrapper: wrapperFor(true) });
    connect(result.current.store);
    act(() => result.current.store.getState().setConnectionStatus("disconnected"));
    await act(async () => request.resolve([makeErrand()]));
    expect(result.current.first.isLoaded).toBe(false);
    expect(result.current.first.errands).toEqual([]);
    connect(result.current.store);
    await waitFor(() => expect(result.current.first.isLoaded).toBe(true));
  });

  it("prevents a task reset from reviving an admitted snapshot", async () => {
    const request = deferredList();
    api.listErrands.mockReturnValueOnce(request.promise);
    const { result } = renderHook(useConsumers, { wrapper: wrapperFor(true) });
    connect(result.current.store);
    act(() => result.current.store.getState().removeTaskErrands(TASK_ID));
    await act(async () => request.resolve([makeErrand()]));
    expect(result.current.first.isLoaded).toBe(false);
    expect(result.current.first.errands).toEqual([]);
  });
});

describe("task errand admitted reads", () => {
  beforeEach(() => {
    api.listErrands.mockReset();
  });

  it("does not publish a deferred response after feature disable", async () => {
    const request = deferredList();
    api.listErrands.mockReturnValueOnce(request.promise);
    const { result } = renderHook(useConsumers, { wrapper: wrapperFor(true) });
    connect(result.current.store);
    act(() =>
      result.current.store.getState().setFeatures({ ...defaultState.features, errands: false }),
    );
    await act(async () => request.resolve([makeErrand()]));
    expect(result.current.first.errands).toEqual([]);
    expect(result.current.store.getState().errands.byTaskId[TASK_ID]).toBeUndefined();
  });

  it("rejects an old scope response before another consumer renders", async () => {
    const request = deferredList();
    const next = deferredList();
    api.listErrands.mockReturnValueOnce(request.promise).mockReturnValueOnce(next.promise);
    const { result } = renderHook(useConsumers, { wrapper: wrapperFor(true) });
    connect(result.current.store);
    await act(async () => {
      result.current.store.getState().setActiveWorkspace("workspace-b");
      request.resolve([makeErrand({ errand_id: "old-scope" })]);
    });
    expect(result.current.first.errands).toEqual([]);
    expect(api.listErrands).toHaveBeenCalledTimes(2);
    await act(async () => next.resolve([makeErrand({ errand_id: "new-scope" })]));
    expect(result.current.first.errands.map((row) => row.errand_id)).toEqual(["new-scope"]);
  });

  it("keeps stores independent and admits completion after consumer task switch", async () => {
    const request = deferredList();
    const other = deferredList();
    const nextTask = deferredList();
    api.listErrands
      .mockReturnValueOnce(request.promise)
      .mockReturnValueOnce(other.promise)
      .mockReturnValueOnce(nextTask.promise);
    const first = renderHook(useSelectedTask, {
      initialProps: TASK_ID as string | null,
      wrapper: wrapperFor(true),
    });
    const second = renderHook(useSelectedTask, {
      initialProps: TASK_ID as string | null,
      wrapper: wrapperFor(true),
    });
    connect(first.result.current.store);
    connect(second.result.current.store);
    first.rerender("task-b");
    expect(api.listErrands).toHaveBeenCalledTimes(3);
    await act(async () => request.resolve([makeErrand({ errand_id: "first-store" })]));
    expect(first.result.current.store.getState().errands.loadedByTaskId[TASK_ID]).toBe(true);
    expect(first.result.current.value.errands).toEqual([]);
    await act(async () =>
      nextTask.resolve([makeErrand({ task_id: "task-b", errand_id: "task-b-row" })]),
    );
    await act(async () => other.resolve([makeErrand({ errand_id: "second-store" })]));
    expect(first.result.current.value.errands[0].errand_id).toBe("task-b-row");
    expect(
      first.result.current.store.getState().errands.byTaskId[TASK_ID]?.["first-store"],
    ).toBeDefined();
    expect(second.result.current.value.errands[0].errand_id).toBe("second-store");
  });

  it("does not let an obsolete finally remove a pending replacement", async () => {
    const old = deferredList();
    const fresh = deferredList();
    api.listErrands.mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise);
    const hook = renderHook(useSelectedTask, {
      initialProps: TASK_ID as string | null,
      wrapper: wrapperFor(true),
    });
    connect(hook.result.current.store);
    act(() => {
      hook.result.current.store.getState().setConnectionStatus("disconnected");
      hook.result.current.store.getState().setConnectionStatus("connected");
    });
    await act(async () => old.reject(new Error("old transport failed")));
    hook.rerender(null);
    hook.rerender(TASK_ID);
    expect(api.listErrands).toHaveBeenCalledTimes(2);
    await act(async () => fresh.resolve([makeErrand()]));
    expect(hook.result.current.value.isLoaded).toBe(true);
  });
});

describe("task errand scope transitions", () => {
  beforeEach(() => {
    api.listErrands.mockReset();
    api.listErrands.mockResolvedValue([makeErrand()]);
  });

  it("refreshes a cached task after reconnect with all consumers unmounted", async () => {
    api.listErrands
      .mockResolvedValueOnce([makeErrand({ errand_id: "before-reconnect" })])
      .mockResolvedValueOnce([makeErrand({ errand_id: "after-reconnect" })]);
    let store!: ReturnType<typeof useAppStoreApi>;
    function Harness({ showConsumer }: { showConsumer: boolean }) {
      return createElement(StateProvider, {
        initialState: { features: { ...defaultState.features, errands: true } },
        children: showConsumer
          ? createElement(ErrandsConsumerProbe, { onStore: (current) => (store = current) })
          : null,
      });
    }
    const view = render(createElement(Harness, { showConsumer: true }));
    connect(store);
    await waitFor(() =>
      expect(store.getState().errands.byTaskId[TASK_ID]?.["before-reconnect"]).toBeDefined(),
    );
    view.rerender(createElement(Harness, { showConsumer: false }));
    act(() => {
      store.getState().setConnectionStatus("disconnected");
      store.getState().setConnectionStatus("connected");
    });
    view.rerender(createElement(Harness, { showConsumer: true }));
    await waitFor(() => expect(api.listErrands).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(store.getState().errands.byTaskId[TASK_ID]?.["after-reconnect"]).toBeDefined(),
    );
  });

  it("rejects a deferred response after the authenticated read scope changes", async () => {
    const old = deferredList();
    const fresh = deferredList();
    api.listErrands.mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise);
    const hook = renderHook(useSelectedTask, {
      initialProps: TASK_ID as string | null,
      wrapper: wrapperFor(true),
    });
    connect(hook.result.current.store);
    expect(api.listErrands).toHaveBeenCalledTimes(1);
    act(() => hook.result.current.store.getState().setAuthState(authFor("user-b")));
    await waitFor(() => expect(api.listErrands).toHaveBeenCalledTimes(2));
    await act(async () => fresh.resolve([makeErrand({ errand_id: "current-user" })]));
    await act(async () => old.resolve([makeErrand({ errand_id: "previous-user" })]));
    expect(
      Object.keys(hook.result.current.store.getState().errands.byTaskId[TASK_ID] ?? {}),
    ).toEqual(["current-user"]);
  });
});

it("publishes a shared read when the initiating consumer unmounts", async () => {
  api.listErrands.mockReset();
  const request = deferredList();
  api.listErrands.mockReturnValueOnce(request.promise);
  let store!: ReturnType<typeof useAppStoreApi>;
  let remaining!: ReturnType<typeof useTaskErrands>;
  function Harness({ showInitiator }: { showInitiator: boolean }) {
    return createElement(StateProvider, {
      initialState: { features: { ...defaultState.features, errands: true } },
      children: [
        showInitiator
          ? createElement(ErrandsConsumerProbe, {
              key: "first",
              onStore: (current) => (store = current),
            })
          : null,
        createElement(ErrandsConsumerProbe, {
          key: "remaining",
          onValue: (current) => (remaining = current),
        }),
      ],
    });
  }
  const view = render(createElement(Harness, { showInitiator: true }));
  connect(store);
  view.rerender(createElement(Harness, { showInitiator: false }));
  await act(async () => request.resolve([makeErrand({ status: "done" })]));
  expect(remaining.isLoaded).toBe(true);
  expect(remaining.errands[0].status).toBe("done");
  expect(api.listErrands).toHaveBeenCalledTimes(1);
});

it("retains cached data after refresh failure and repairs it on next connect", async () => {
  api.listErrands.mockReset();
  api.listErrands.mockResolvedValueOnce([makeErrand()]);
  const { result } = renderHook(useConsumers, { wrapper: wrapperFor(true) });
  connect(result.current.store);
  await waitFor(() => expect(result.current.first.isLoaded).toBe(true));
  const priorGeneration = result.current.store.getState().connection.generation;
  const refresh = deferredList();
  api.listErrands.mockReturnValueOnce(refresh.promise);
  act(() => {
    result.current.store.getState().setConnectionStatus("disconnected");
    result.current.store.getState().setConnectionStatus("connected");
  });
  await act(async () => refresh.reject(new Error("unavailable")));
  expect(result.current.first.errands).toHaveLength(1);
  expect(result.current.store.getState().errands.loadedConnectionGenerationByTaskId[TASK_ID]).toBe(
    priorGeneration,
  );
  api.listErrands.mockResolvedValueOnce([]);
  act(() => {
    result.current.store.getState().setConnectionStatus("disconnected");
    result.current.store.getState().setConnectionStatus("connected");
  });
  await waitFor(() => expect(result.current.first.errands).toEqual([]));
  expect(api.listErrands).toHaveBeenCalledTimes(3);
});
