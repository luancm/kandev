import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
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

function connect(store: ReturnType<typeof useAppStoreApi>) {
  act(() => store.getState().setConnectionStatus("connected"));
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
