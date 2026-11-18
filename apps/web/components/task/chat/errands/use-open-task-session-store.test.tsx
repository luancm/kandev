import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import {
  makeBrowserErrandRow,
  makeInspection,
} from "@/lib/state/slices/errands/errands.test-helpers";
import { sessionId, taskId } from "@/lib/types/ids";
import { useOpenTaskSession } from "./use-open-task-session";
import { useErrandViewModel } from "./use-errand-view-model";
import { defaultState } from "@/lib/state/default-state";
import { OwnedErrandNavigationContext } from "./owned-errand-navigation-context";

const transport = vi.hoisted(() => ({ inspect: vi.fn(), detail: vi.fn() }));
vi.mock("@/lib/api/domains/errand-api", () => ({ inspectErrand: transport.inspect }));
vi.mock("@/lib/api", () => ({ fetchTaskSession: transport.detail }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ usesDesktopWorkbench: false }),
}));

const wrapper = ({ children }: { children: ReactNode }) => (
  <StateProvider>{children}</StateProvider>
);
const CHILD_ID = "errand-1";
const PARENT_ID = "session-parent";
const target = { taskId: "task-1", sessionId: CHILD_ID, title: "Audit", isErrand: true as const };
const child = {
  id: sessionId(CHILD_ID),
  task_id: taskId("task-1"),
  role: "errand" as const,
  parent_session_id: sessionId(PARENT_ID),
  state: "COMPLETED" as const,
  started_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

beforeEach(() => vi.clearAllMocks());

it("opens an Errand-owned conversation without a TaskSession request or selection", async () => {
  const owned = { ...makeInspection(), child_session_exists: false, owns_conversation: true };
  const openOwned = vi.fn(() => true);
  transport.inspect.mockResolvedValue(owned);
  const host = ({ children }: { children: ReactNode }) => (
    <StateProvider>
      <OwnedErrandNavigationContext.Provider value={openOwned}>
        {children}
      </OwnedErrandNavigationContext.Provider>
    </StateProvider>
  );
  const { result } = renderHook(() => ({ store: useAppStoreApi(), open: useOpenTaskSession() }), {
    wrapper: host,
  });
  act(() => {
    result.current.store.getState().setConnectionStatus("connected");
    result.current.store.getState().setActiveSession("task-1", PARENT_ID);
  });
  await act(async () => expect(await result.current.open(target)).toBe(true));
  expect(openOwned).toHaveBeenCalledExactlyOnceWith(owned);
  expect(transport.detail).not.toHaveBeenCalled();
  expect(result.current.store.getState().tasks.activeSessionId).toBe(PARENT_ID);
  expect(result.current.store.getState().taskSessions.items[CHILD_ID]).toBeUndefined();
});

it("reuses the successful Open inspection when the child view mounts", async () => {
  transport.inspect.mockResolvedValue(makeInspection());
  transport.detail.mockResolvedValue({ session: child });
  const { result, rerender } = renderHook(
    ({ selected }: { selected: string | null }) => ({
      store: useAppStoreApi(),
      open: useOpenTaskSession(),
      view: useErrandViewModel(selected, "task-1"),
    }),
    { wrapper, initialProps: { selected: null as string | null } },
  );
  act(() => {
    result.current.store.getState().setFeatures({ ...defaultState.features, errands: true });
    result.current.store.getState().setConnectionStatus("connected");
    result.current.store.getState().setActiveSession("task-1", PARENT_ID);
  });
  await act(async () => expect(await result.current.open(target)).toBe(true));
  rerender({ selected: CHILD_ID });
  await waitFor(() => expect(result.current.view?.sessionId).toBe(CHILD_ID));
  expect(transport.inspect).toHaveBeenCalledExactlyOnceWith(
    "task-1",
    CHILD_ID,
    expect.any(AbortSignal),
  );
});

// @covers AC-TASKS-ERRAND-HISTORY-001.7 AC-TASKS-ERRAND-HISTORY-001.8
it("releases provisional inspection after detail fails while the opener remains mounted", async () => {
  transport.inspect.mockResolvedValue(makeInspection());
  const detail = deferred<{ session: typeof child }>();
  transport.detail.mockReturnValue(detail.promise);
  const { result } = renderHook(() => ({ store: useAppStoreApi(), open: useOpenTaskSession() }), {
    wrapper,
  });
  act(() => result.current.store.getState().setActiveSession("task-1", PARENT_ID));
  const pending = result.current.open(target);
  await waitFor(() => expect(transport.detail).toHaveBeenCalled());
  expect(result.current.store.getState().errands.detailBySessionId[CHILD_ID]).toBeDefined();
  detail.reject(new Error("failed"));
  await act(async () => expect(await pending).toBe(false));
  expect(result.current.store.getState().errands.detailBySessionId[CHILD_ID]).toBeUndefined();
});

it("does not publish child detail or navigate after the read scope resets during the second await", async () => {
  transport.inspect.mockResolvedValue(makeInspection());
  const detail = deferred<{ session: typeof child }>();
  transport.detail.mockReturnValue(detail.promise);
  const { result } = renderHook(() => ({ store: useAppStoreApi(), open: useOpenTaskSession() }), {
    wrapper,
  });
  act(() => result.current.store.getState().setActiveSession("task-1", PARENT_ID));
  const pending = result.current.open(target);
  await waitFor(() => expect(transport.detail).toHaveBeenCalled());
  act(() => result.current.store.getState().resetErrandReadScope());
  detail.resolve({ session: child });
  await act(async () => expect(await pending).toBe(false));
  expect(result.current.store.getState().taskSessions.items[CHILD_ID]).toBeUndefined();
  expect(result.current.store.getState().tasks.activeSessionId).toBe(PARENT_ID);
});

it.each(["scope", "task", "deletion"] as const)(
  "rejects inspection after %s invalidation before the first await completes",
  async (invalidation) => {
    const inspection = deferred<ReturnType<typeof makeInspection>>();
    transport.inspect.mockReturnValue(inspection.promise);
    const { result } = renderHook(() => ({ store: useAppStoreApi(), open: useOpenTaskSession() }), {
      wrapper,
    });
    act(() => result.current.store.getState().setActiveSession("task-1", PARENT_ID));
    const pending = result.current.open(target);
    await waitFor(() => expect(transport.inspect).toHaveBeenCalled());
    act(() => {
      const state = result.current.store.getState();
      if (invalidation === "scope") state.resetErrandReadScope();
      if (invalidation === "task") state.removeTaskErrands("task-1");
      if (invalidation === "deletion") state.removeErrandBrowserSession("task-1", CHILD_ID);
    });
    inspection.resolve(makeInspection());
    await act(async () => expect(await pending).toBe(false));
    expect(transport.detail).not.toHaveBeenCalled();
    expect(result.current.store.getState().errands.detailBySessionId[CHILD_ID]).toBeUndefined();
    expect(result.current.store.getState().tasks.activeSessionId).toBe(PARENT_ID);
  },
);

it.each(["task", "deletion"] as const)(
  "rejects full detail after %s invalidation during the second await",
  async (invalidation) => {
    transport.inspect.mockResolvedValue(makeInspection());
    const detail = deferred<{ session: typeof child }>();
    transport.detail.mockReturnValue(detail.promise);
    const { result } = renderHook(() => ({ store: useAppStoreApi(), open: useOpenTaskSession() }), {
      wrapper,
    });
    act(() => result.current.store.getState().setActiveSession("task-1", PARENT_ID));
    const pending = result.current.open(target);
    await waitFor(() => expect(transport.detail).toHaveBeenCalled());
    act(() => {
      const state = result.current.store.getState();
      if (invalidation === "task") state.removeTaskErrands("task-1");
      else state.removeErrandBrowserSession("task-1", CHILD_ID);
    });
    detail.resolve({ session: child });
    await act(async () => expect(await pending).toBe(false));
    expect(result.current.store.getState().taskSessions.items[CHILD_ID]).toBeUndefined();
    expect(result.current.store.getState().tasks.activeSessionId).toBe(PARENT_ID);
  },
);

it("preserves an independent inspection owner after a failed opener until its final release", async () => {
  transport.inspect.mockResolvedValue(makeInspection());
  const detail = deferred<{ session: typeof child }>();
  transport.detail.mockReturnValue(detail.promise);
  const { result } = renderHook(() => ({ store: useAppStoreApi(), open: useOpenTaskSession() }), {
    wrapper,
  });
  act(() => result.current.store.getState().setActiveSession("task-1", PARENT_ID));
  const pending = result.current.open(target);
  await waitFor(() => expect(transport.detail).toHaveBeenCalled());
  act(() => result.current.store.getState().retainErrandInspection("other", makeInspection()));
  detail.reject(new Error("failed"));
  await act(async () => expect(await pending).toBe(false));
  expect(result.current.store.getState().errands.detailBySessionId[CHILD_ID]).toBeDefined();
  act(() => result.current.store.getState().releaseErrandInspection("other"));
  expect(result.current.store.getState().errands.detailBySessionId[CHILD_ID]).toBeUndefined();
});

it("releases provisional inspection when external navigation supersedes a pending open", async () => {
  transport.inspect.mockResolvedValue(makeInspection());
  const detail = deferred<{ session: typeof child }>();
  transport.detail.mockReturnValue(detail.promise);
  const { result } = renderHook(() => ({ store: useAppStoreApi(), open: useOpenTaskSession() }), {
    wrapper,
  });
  act(() => result.current.store.getState().setActiveSession("task-1", PARENT_ID));
  const pending = result.current.open(target);
  await waitFor(() => expect(transport.detail).toHaveBeenCalled());
  act(() => result.current.store.getState().setActiveSession("task-1", "ordinary"));
  expect(result.current.store.getState().errands.detailBySessionId[CHILD_ID]).toBeUndefined();
  detail.resolve({ session: child });
  await act(async () => expect(await pending).toBe(false));
});

it("does not retain or fetch detail after ordinary navigation during inspection", async () => {
  const inspection = deferred<ReturnType<typeof makeInspection>>();
  transport.inspect.mockReturnValue(inspection.promise);
  const { result } = renderHook(() => ({ store: useAppStoreApi(), open: useOpenTaskSession() }), {
    wrapper,
  });
  act(() => result.current.store.getState().setActiveSession("task-1", PARENT_ID));
  const pending = result.current.open(target);
  await waitFor(() =>
    expect(transport.inspect).toHaveBeenCalledExactlyOnceWith(
      "task-1",
      CHILD_ID,
      expect.any(AbortSignal),
    ),
  );
  act(() => result.current.store.getState().setActiveSession("task-1", "ordinary"));
  inspection.resolve(makeInspection());
  await act(async () => expect(await pending).toBe(false));
  expect(transport.detail).not.toHaveBeenCalled();
  expect(result.current.store.getState().errands.detailBySessionId[CHILD_ID]).toBeUndefined();
  expect(result.current.store.getState().tasks.activeSessionId).toBe("ordinary");
});

it("starts a fresh inspection after a scope reset even when the old transport has not settled", async () => {
  const old = deferred<ReturnType<typeof makeInspection>>();
  const fresh = deferred<ReturnType<typeof makeInspection>>();
  transport.inspect.mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise);
  transport.detail.mockResolvedValue({ session: child });
  const { result } = renderHook(() => ({ store: useAppStoreApi(), open: useOpenTaskSession() }), {
    wrapper,
  });
  act(() => result.current.store.getState().setActiveSession("task-1", PARENT_ID));
  const first = result.current.open(target);
  await waitFor(() => expect(transport.inspect).toHaveBeenCalledTimes(1));
  act(() => result.current.store.getState().resetErrandReadScope());
  const second = result.current.open(target);
  await waitFor(() => expect(transport.inspect).toHaveBeenCalledTimes(2));
  fresh.resolve(makeInspection());
  await act(async () => expect(await second).toBe(true));
  old.resolve(makeInspection());
  await act(async () => expect(await first).toBe(false));
});

it("rejects a child deleted by a subscriber during full-session publication", async () => {
  transport.inspect.mockResolvedValue(makeInspection());
  transport.detail.mockResolvedValue({ session: child });
  const { result } = renderHook(() => ({ store: useAppStoreApi(), open: useOpenTaskSession() }), {
    wrapper,
  });
  const store = result.current.store;
  act(() => store.getState().setActiveSession("task-1", PARENT_ID));
  const unsubscribe = store.subscribe((state, previous) => {
    if (state.taskSessions.items[CHILD_ID] && !previous.taskSessions.items[CHILD_ID]) {
      state.removeTaskSession("task-1", CHILD_ID);
    }
  });
  await act(async () => expect(await result.current.open(target)).toBe(false));
  unsubscribe();
  expect(store.getState().taskSessions.items[CHILD_ID]).toBeUndefined();
  expect(store.getState().errands.detailBySessionId[CHILD_ID]).toBeUndefined();
  expect(store.getState().tasks.activeSessionId).toBe(PARENT_ID);
});

it("does not rebase a replacement inspection owned by another reader during publication", async () => {
  transport.inspect.mockResolvedValue(makeInspection());
  transport.detail.mockResolvedValue({ session: child });
  const { result } = renderHook(() => ({ store: useAppStoreApi(), open: useOpenTaskSession() }), {
    wrapper,
  });
  const store = result.current.store;
  act(() => store.getState().setActiveSession("task-1", PARENT_ID));
  const replacement = makeInspection(makeBrowserErrandRow({ title: "Replacement" }));
  const unsubscribe = store.subscribe((state, previous) => {
    if (state.taskSessions.items[CHILD_ID] && !previous.taskSessions.items[CHILD_ID]) {
      state.retainErrandInspection("other", replacement);
    }
  });
  await act(async () => expect(await result.current.open(target)).toBe(false));
  unsubscribe();
  expect(store.getState().errands.detailBySessionId[CHILD_ID]).toEqual(replacement);
  expect(store.getState().errands.detailOwnersBySessionId[CHILD_ID]).toEqual({ other: true });
  expect(store.getState().tasks.activeSessionId).toBe(PARENT_ID);
  act(() => store.getState().releaseErrandInspection("other"));
  expect(store.getState().errands.detailBySessionId[CHILD_ID]).toBeUndefined();
});

it("does not absorb an unrelated revision during full-session publication", async () => {
  transport.inspect.mockResolvedValue(makeInspection());
  transport.detail.mockResolvedValue({ session: child });
  const { result } = renderHook(() => ({ store: useAppStoreApi(), open: useOpenTaskSession() }), {
    wrapper,
  });
  const store = result.current.store;
  act(() => store.getState().setActiveSession("task-1", PARENT_ID));
  const unsubscribe = store.subscribe((state, previous) => {
    if (state.taskSessions.items[CHILD_ID] && !previous.taskSessions.items[CHILD_ID]) {
      state.advanceErrandBrowserRevision("task-1");
    }
  });
  await act(async () => expect(await result.current.open(target)).toBe(false));
  unsubscribe();
  expect(store.getState().errands.detailBySessionId[CHILD_ID]).toBeUndefined();
  expect(store.getState().tasks.activeSessionId).toBe(PARENT_ID);
});
