import { createElement, useEffect, useMemo, type ReactNode } from "react";
import { act, cleanup, render, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import {
  ERRAND_PARENT_ID,
  ERRAND_TASK_ID,
  makeBrowserErrandRow,
  makeBrowserSummary,
  makeHistoryPage,
} from "@/lib/state/slices/errands/errands.test-helpers";

const api = vi.hoisted(() => ({
  getErrandBrowserSummary: vi.fn(),
  getErrandHistoryPage: vi.fn(),
  listErrands: vi.fn(),
}));
vi.mock("@/lib/api/domains/errand-api", () => api);

import { ErrandHistoryOwner } from "@/lib/state/slices/errands/errand-browser-read-coordinator";
import { useTaskErrands } from "./use-task-errands";

const TASK_ID = ERRAND_TASK_ID;
const PARENT_ID = ERRAND_PARENT_ID;
const SECOND_PARENT_ID = "errand-parent-2";

function useSharedOwnerParentConsumers() {
  const store = useAppStoreApi();
  const owner = useMemo(() => new ErrandHistoryOwner(store), [store]);
  return {
    store,
    first: useTaskErrands(TASK_ID, {
      parentSessionId: PARENT_ID,
      historyOwner: owner,
    }),
    second: useTaskErrands(TASK_ID, {
      parentSessionId: SECOND_PARENT_ID,
      historyOwner: owner,
    }),
  };
}

function mockPendingParentHistoryReads() {
  const firstPage = deferred<ReturnType<typeof makeHistoryPage>>();
  const secondPage = deferred<ReturnType<typeof makeHistoryPage>>();
  let firstSignal: AbortSignal | undefined;
  api.getErrandHistoryPage.mockImplementation(
    (_taskId: string, parentSessionId: string, _options: unknown, signal: AbortSignal) => {
      if (parentSessionId === PARENT_ID) {
        firstSignal = signal;
        return firstPage.promise;
      }
      return secondPage.promise;
    },
  );
  return { firstPage, secondPage, firstSignal: () => firstSignal };
}

function historyPageForParent(parentSessionId: string, rowId: string) {
  const row = makeBrowserErrandRow({ errand_id: rowId, parent_session_id: parentSessionId });
  return {
    row,
    page: {
      ...makeHistoryPage([row]),
      scope: { task_id: TASK_ID, parent_session_id: parentSessionId },
    },
  };
}

function wrapperFor(errandsEnabled: boolean) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(StateProvider, {
      initialState: { features: { ...defaultState.features, errands: errandsEnabled } },
      children,
    });
  };
}

function useConsumer(taskId: string | null = TASK_ID) {
  const store = useAppStoreApi();
  const owner = useMemo(() => new ErrandHistoryOwner(store), [store]);
  return {
    store,
    value: useTaskErrands(taskId, { parentSessionId: PARENT_ID, historyOwner: owner }),
  };
}

function connect(store: ReturnType<typeof useAppStoreApi>) {
  act(() => store.getState().setConnectionStatus("connected"));
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  api.getErrandBrowserSummary.mockResolvedValue(makeBrowserSummary());
  api.getErrandHistoryPage.mockResolvedValue(makeHistoryPage([]));
});

it("coalesces summary reads and does not request finished history on mount", async () => {
  function useTwoConsumers() {
    const store = useAppStoreApi();
    const first = useMemo(() => new ErrandHistoryOwner(store), [store]);
    const second = useMemo(() => new ErrandHistoryOwner(store), [store]);
    return {
      store,
      first: useTaskErrands(TASK_ID, { parentSessionId: PARENT_ID, historyOwner: first }),
      second: useTaskErrands(TASK_ID, { parentSessionId: PARENT_ID, historyOwner: second }),
    };
  }
  const { result } = renderHook(useTwoConsumers, { wrapper: wrapperFor(true) });
  connect(result.current.store);

  await waitFor(() => expect(result.current.first.isSummaryLoaded).toBe(true));
  expect(api.getErrandBrowserSummary).toHaveBeenCalledExactlyOnceWith(
    TASK_ID,
    expect.any(AbortSignal),
  );
  expect(api.getErrandHistoryPage).not.toHaveBeenCalled();
  expect(api.listErrands).not.toHaveBeenCalled();
  expect(result.current.first.outstanding).toEqual([]);
  expect(result.current.second.counts).toEqual(makeBrowserSummary().counts);
});

it("does not load or expose browser errands while the feature is off", () => {
  const { result } = renderHook(() => useConsumer(), { wrapper: wrapperFor(false) });
  connect(result.current.store);

  expect(api.getErrandBrowserSummary).not.toHaveBeenCalled();
  expect(api.getErrandHistoryPage).not.toHaveBeenCalled();
  expect(result.current.value.outstanding).toEqual([]);
  expect(result.current.value.isSummaryLoaded).toBe(false);
});

it("loads the first history page only after its disclosure opens", async () => {
  const historical = makeBrowserErrandRow({
    errand_id: "history-1",
    status: "done",
    created_at: "2026-10-26T10:00:00Z",
  });
  api.getErrandHistoryPage.mockResolvedValue(makeHistoryPage([historical], { total: 1 }));
  const { result } = renderHook(() => useConsumer(), { wrapper: wrapperFor(true) });
  connect(result.current.store);
  await waitFor(() => expect(result.current.value.isSummaryLoaded).toBe(true));

  expect(api.getErrandHistoryPage).not.toHaveBeenCalled();
  act(() => result.current.value.openHistory());
  await waitFor(() => expect(result.current.value.historyRows).toEqual([historical]));

  expect(api.getErrandHistoryPage).toHaveBeenCalledExactlyOnceWith(
    TASK_ID,
    PARENT_ID,
    { limit: 25 },
    expect.any(AbortSignal),
  );
});

it("refreshes only the summary after reconnect and leaves open history stale", async () => {
  api.getErrandBrowserSummary.mockResolvedValue(makeBrowserSummary([], { finished: 1 }));
  api.getErrandHistoryPage.mockResolvedValue(makeHistoryPage([makeBrowserErrandRow()]));
  const { result } = renderHook(() => useConsumer(), { wrapper: wrapperFor(true) });
  connect(result.current.store);
  await waitFor(() => expect(result.current.value.isSummaryLoaded).toBe(true));
  act(() => result.current.value.openHistory());
  await waitFor(() => expect(result.current.value.historyRows).toHaveLength(1));

  act(() => result.current.store.getState().setConnectionStatus("disconnected"));
  act(() => result.current.store.getState().setConnectionStatus("connected"));
  await waitFor(() => expect(api.getErrandBrowserSummary).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(result.current.value.history?.stale).toBe(true));

  expect(api.getErrandHistoryPage).toHaveBeenCalledTimes(1);
});

it("replaces an A read after an A-to-B-to-A switch and rejects its late response", async () => {
  const oldA = deferred<ReturnType<typeof makeBrowserSummary>>();
  const taskB = deferred<ReturnType<typeof makeBrowserSummary>>();
  const freshA = deferred<ReturnType<typeof makeBrowserSummary>>();
  api.getErrandBrowserSummary
    .mockReturnValueOnce(oldA.promise)
    .mockReturnValueOnce(taskB.promise)
    .mockReturnValueOnce(freshA.promise);
  const { result, rerender } = renderHook(({ taskId }) => useConsumer(taskId), {
    initialProps: { taskId: TASK_ID as string | null },
    wrapper: wrapperFor(true),
  });
  connect(result.current.store);
  await waitFor(() => expect(api.getErrandBrowserSummary).toHaveBeenCalledTimes(1));
  rerender({ taskId: "task-b" });
  await waitFor(() => expect(api.getErrandBrowserSummary).toHaveBeenCalledTimes(2));
  rerender({ taskId: TASK_ID });
  await waitFor(() => expect(api.getErrandBrowserSummary).toHaveBeenCalledTimes(3));

  freshA.resolve(
    makeBrowserSummary([makeBrowserErrandRow({ errand_id: "fresh-a", outstanding: true })], {
      outstanding: 1,
    }),
  );
  await waitFor(() => expect(result.current.value.outstanding[0]?.errand_id).toBe("fresh-a"));
  oldA.resolve(
    makeBrowserSummary([makeBrowserErrandRow({ errand_id: "old-a", outstanding: true })], {
      outstanding: 1,
    }),
  );
  taskB.resolve(
    makeBrowserSummary(
      [makeBrowserErrandRow({ task_id: "task-b", errand_id: "b", outstanding: true })],
      { outstanding: 1 },
    ),
  );
  await act(async () => Promise.resolve());

  expect(result.current.value.outstanding.map((row) => row.errand_id)).toEqual(["fresh-a"]);
});

it("retries a failed summary explicitly", async () => {
  api.getErrandBrowserSummary
    .mockRejectedValueOnce(new Error("read failed"))
    .mockResolvedValueOnce(makeBrowserSummary());
  const { result } = renderHook(() => useConsumer(), { wrapper: wrapperFor(true) });
  connect(result.current.store);
  await waitFor(() => expect(result.current.value.summaryStatus).toBe("error"));
  act(() => result.current.value.retrySummary());
  await waitFor(() => expect(result.current.value.isSummaryLoaded).toBe(true));
  expect(api.getErrandBrowserSummary).toHaveBeenCalledTimes(2);
  expect(api.listErrands).not.toHaveBeenCalled();
});
describe("shared history read ownership", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getErrandBrowserSummary.mockResolvedValue(makeBrowserSummary());
  });

  it("keeps a coalesced history request alive when its initiating panel unmounts", async () => {
    const pending = deferred<ReturnType<typeof makeHistoryPage>>();
    let signal: AbortSignal | undefined;
    api.getErrandHistoryPage.mockImplementation(
      (_taskId: string, _parent: string, _options: unknown, nextSignal: AbortSignal) => {
        signal = nextSignal;
        return pending.promise;
      },
    );
    let firstValue: ReturnType<typeof useTaskErrands> | undefined;
    let secondValue: ReturnType<typeof useTaskErrands> | undefined;
    let store: ReturnType<typeof useAppStoreApi> | undefined;
    function Consumer({
      owner,
      onValue,
    }: {
      owner: string;
      onValue: (value: ReturnType<typeof useTaskErrands>) => void;
    }) {
      const store = useAppStoreApi();
      const historyOwner = useMemo(() => new ErrandHistoryOwner(store), [store, owner]);
      const value = useTaskErrands(TASK_ID, { parentSessionId: PARENT_ID, historyOwner });
      useEffect(() => onValue(value), [onValue, value]);
      return null;
    }
    function Consumers({ showFirst }: { showFirst: boolean }) {
      store = useAppStoreApi();
      return createElement(
        "div",
        null,
        showFirst &&
          createElement(Consumer, {
            owner: "panel-first",
            onValue: (value) => {
              firstValue = value;
            },
          }),
        createElement(Consumer, {
          owner: "panel-second",
          onValue: (value) => {
            secondValue = value;
          },
        }),
      );
    }
    const renderTree = (showFirst: boolean) =>
      render(
        createElement(StateProvider, {
          initialState: { features: { ...defaultState.features, errands: true } },
          children: createElement(Consumers, { showFirst }),
        }),
      );
    const view = renderTree(true);
    connect(store!);
    await waitFor(() => expect(api.getErrandBrowserSummary).toHaveBeenCalledTimes(1));
    act(() => {
      firstValue?.openHistory();
      secondValue?.openHistory();
    });
    await waitFor(() => expect(api.getErrandHistoryPage).toHaveBeenCalledTimes(1));
    view.rerender(
      createElement(StateProvider, {
        initialState: { features: { ...defaultState.features, errands: true } },
        children: createElement(Consumers, { showFirst: false }),
      }),
    );

    expect(signal?.aborted).toBe(false);
    pending.resolve(makeHistoryPage([makeBrowserErrandRow({ errand_id: "shared-history" })]));
    await waitFor(() =>
      expect(secondValue?.historyRows.map((row) => row.errand_id)).toEqual(["shared-history"]),
    );
    view.unmount();
  });

  it("releases the previous parent page and scopes a shared owner to its active group", async () => {
    const pending = mockPendingParentHistoryReads();
    const { result } = renderHook(useSharedOwnerParentConsumers, { wrapper: wrapperFor(true) });
    connect(result.current.store);
    await waitFor(() => expect(result.current.first.isSummaryLoaded).toBe(true));

    act(() => result.current.first.openHistory());
    await waitFor(() => expect(api.getErrandHistoryPage).toHaveBeenCalledTimes(1));
    act(() => result.current.second.openHistory());
    await waitFor(() => expect(api.getErrandHistoryPage).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(pending.firstSignal()?.aborted).toBe(true));

    const active = historyPageForParent(SECOND_PARENT_ID, "second-parent-row");
    pending.secondPage.resolve(active.page);
    await waitFor(() => expect(result.current.second.historyRows).toEqual([active.row]));

    pending.firstPage.resolve(
      makeHistoryPage([makeBrowserErrandRow({ errand_id: "late-first-parent-row" })]),
    );
    await act(async () => Promise.resolve());

    expect(result.current.first.history).toBeUndefined();
    expect(result.current.first.historyRows).toEqual([]);
    expect(result.current.second.historyRows).toEqual([active.row]);
  });
});

// @covers AC-TASKS-ERRAND-HISTORY-001.6
it("drops loaded history when a new authority is denied access", async () => {
  api.getErrandHistoryPage.mockResolvedValue(makeHistoryPage([makeBrowserErrandRow()]));
  const { result } = renderHook(() => useConsumer(), { wrapper: wrapperFor(true) });
  connect(result.current.store);
  await waitFor(() => expect(result.current.value.isSummaryLoaded).toBe(true));
  act(() => result.current.value.openHistory());
  await waitFor(() => expect(result.current.value.historyRows).toHaveLength(1));
  api.getErrandBrowserSummary.mockRejectedValue(new Error("Forbidden"));
  act(() => result.current.store.setState({ workspaceContextGeneration: 1 }));
  await waitFor(() => expect(result.current.value.summaryStatus).toBe("error"));
  expect(result.current.value.historyRows).toEqual([]);
  expect(result.current.value.history).toBeUndefined();
  expect(result.current.value.summary).toBeUndefined();
  expect(api.getErrandHistoryPage).toHaveBeenCalledTimes(1);
});
