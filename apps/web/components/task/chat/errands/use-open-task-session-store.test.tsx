import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import {
  makeBrowserErrandRow,
  makeInspection,
} from "@/lib/state/slices/errands/errands.test-helpers";
import { useOpenTaskSession } from "./use-open-task-session";
import { OwnedErrandNavigationContext } from "./owned-errand-navigation-context";

const transport = vi.hoisted(() => ({ inspect: vi.fn(), detail: vi.fn() }));
vi.mock("@/lib/api/domains/errand-api", () => ({ inspectErrand: transport.inspect }));
vi.mock("@/lib/api", () => ({ fetchTaskSession: transport.detail }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ usesDesktopWorkbench: false }),
}));

const CHILD_ID = "errand-1";
const PARENT_ID = "session-parent";
const target = { taskId: "task-1", sessionId: CHILD_ID, title: "Audit", isErrand: true as const };
const inspection = makeInspection();

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((yes) => {
    resolve = yes;
  });
  return { promise, resolve };
}

function mountOpener(withOwnedHost = true) {
  const openOwned = vi.fn(() => true);
  const wrapper = ({ children }: { children: ReactNode }) => (
    <StateProvider>
      <OwnedErrandNavigationContext.Provider value={withOwnedHost ? openOwned : null}>
        {children}
      </OwnedErrandNavigationContext.Provider>
    </StateProvider>
  );
  const { result, unmount } = renderHook(
    () => ({ store: useAppStoreApi(), open: useOpenTaskSession() }),
    { wrapper },
  );
  act(() => {
    result.current.store.getState().setConnectionStatus("connected");
    result.current.store.getState().setActiveSession("task-1", PARENT_ID);
  });
  return { result, openOwned, unmount };
}

beforeEach(() => vi.resetAllMocks());

it("opens an Errand-owned conversation without a TaskSession request or selection", async () => {
  transport.inspect.mockResolvedValue(inspection);
  const { result, openOwned } = mountOpener();
  await act(async () => expect(await result.current.open(target)).toBe(true));
  expect(openOwned).toHaveBeenCalledExactlyOnceWith(inspection);
  expect(transport.detail).not.toHaveBeenCalled();
  expect(result.current.store.getState().tasks.activeSessionId).toBe(PARENT_ID);
  expect(result.current.store.getState().taskSessions.items[CHILD_ID]).toBeUndefined();
});

it("leaves the parent selected when there is no host for the owned conversation", async () => {
  transport.inspect.mockResolvedValue(inspection);
  transport.detail.mockRejectedValue(new Error("no task session"));
  const { result } = mountOpener(false);
  await act(async () => expect(await result.current.open(target)).toBe(false));
  expect(transport.detail).not.toHaveBeenCalled();
  expect(result.current.store.getState().tasks.activeSessionId).toBe(PARENT_ID);
});

// @covers AC-TASKS-ERRAND-HISTORY-001.7 AC-TASKS-ERRAND-HISTORY-001.8
it.each(["scope", "task", "deletion", "navigation", "unmount"] as const)(
  "discards the pending owned inspection after %s invalidation",
  async (invalidation) => {
    const read = deferred<typeof inspection>();
    transport.inspect.mockReturnValue(read.promise);
    const { result, openOwned, unmount } = mountOpener();
    const pending = result.current.open(target);
    await waitFor(() => expect(transport.inspect).toHaveBeenCalled());
    act(() => {
      const state = result.current.store.getState();
      if (invalidation === "scope") state.resetErrandReadScope();
      if (invalidation === "task") state.removeTaskErrands("task-1");
      if (invalidation === "deletion") state.removeErrandBrowserSession("task-1", CHILD_ID);
      if (invalidation === "navigation") state.setActiveSession("task-1", "ordinary");
      if (invalidation === "unmount") unmount();
    });
    read.resolve(inspection);
    await act(async () => expect(await pending).toBe(false));
    expect(openOwned).not.toHaveBeenCalled();
    expect(transport.detail).not.toHaveBeenCalled();
  },
);

it("starts a fresh owned inspection after a scope reset while the old read is pending", async () => {
  const old = deferred<typeof inspection>();
  const fresh = deferred<typeof inspection>();
  transport.inspect.mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise);
  const { result, openOwned } = mountOpener();
  const first = result.current.open(target);
  await waitFor(() => expect(transport.inspect).toHaveBeenCalledTimes(1));
  act(() => result.current.store.getState().resetErrandReadScope());
  const second = result.current.open(target);
  await waitFor(() => expect(transport.inspect).toHaveBeenCalledTimes(2));
  fresh.resolve(inspection);
  await act(async () => expect(await second).toBe(true));
  old.resolve(inspection);
  await act(async () => expect(await first).toBe(false));
  expect(openOwned).toHaveBeenCalledExactlyOnceWith(inspection);
});

it("opens only the latest Errand when two inspections overlap", async () => {
  const firstRead = deferred<typeof inspection>();
  const secondRow = makeBrowserErrandRow({ errand_id: "errand-2" });
  const secondInspection = makeInspection(secondRow);
  transport.inspect.mockReturnValueOnce(firstRead.promise).mockResolvedValueOnce(secondInspection);
  const { result, openOwned } = mountOpener();
  const first = result.current.open(target);
  await waitFor(() => expect(transport.inspect).toHaveBeenCalledOnce());
  await act(async () => {
    expect(await result.current.open({ ...target, sessionId: secondRow.errand_id })).toBe(true);
  });
  firstRead.resolve(inspection);
  await act(async () => expect(await first).toBe(false));
  expect(openOwned).toHaveBeenCalledExactlyOnceWith(secondInspection);
  expect(result.current.store.getState().tasks.activeSessionId).toBe(PARENT_ID);
});
