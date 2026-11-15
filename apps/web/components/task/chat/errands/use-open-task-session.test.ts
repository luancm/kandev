import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

type NavigationState = { tasks: { activeTaskId: string | null; activeSessionId: string | null } };
type NavigationListener = (state: NavigationState, previousState: NavigationState) => void;

const mocks = vi.hoisted(() => ({
  usesDesktopWorkbench: true,
  api: { id: "dockview-api" } as unknown,
  addSessionPanel: vi.fn(),
  inspectErrand: vi.fn(),
  fetchTaskSession: vi.fn(),
  setTaskSession: vi.fn(),
  clearHiddenSessionPanel: vi.fn(),
  markIntent: vi.fn(),
  setActiveSession: vi.fn(),
  setPrimary: vi.fn(),
  focusedPanels: [] as string[],
  appListeners: new Set<NavigationListener>(),
  taskId: "task-1",
  otherTaskId: "task-2",
  parentId: "parent-1",
  errandId: "errand-1",
  secondErrandId: "errand-2",
  ordinaryId: "ordinary-1",
  otherSessionId: "session-b",
  appState: {
    tasks: { activeTaskId: null, activeSessionId: null },
  } as { tasks: { activeTaskId: string | null; activeSessionId: string | null } },
  taskSessions: {
    items: {} as Record<
      string,
      { id: string; task_id: string; role?: string; parent_session_id?: string }
    >,
  },
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ usesDesktopWorkbench: mocks.usesDesktopWorkbench }),
}));
vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: { getState: () => ({ api: mocks.api, centerGroupId: "center" }) },
}));
vi.mock("@/lib/state/dockview-panel-actions", () => ({
  addSessionPanel: mocks.addSessionPanel,
}));
vi.mock("@/lib/api/domains/errand-api", () => ({ inspectErrand: mocks.inspectErrand }));
vi.mock("@/lib/api", () => ({ fetchTaskSession: mocks.fetchTaskSession }));
vi.mock("../../dockview-hidden-session-panels", () => ({
  clearHiddenSessionPanel: mocks.clearHiddenSessionPanel,
}));
vi.mock("../../session-tab-activation-intent", () => ({
  markSessionTabUserActivationIntent: mocks.markIntent,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({
      ...mocks.appState,
      setActiveSession: mocks.setActiveSession,
      setPrimary: mocks.setPrimary,
      setTaskSession: mocks.setTaskSession,
      retainErrandInspection: vi.fn(),
      releaseErrandInspection: vi.fn(),
      taskSessions: mocks.taskSessions,
      taskSessionsByTask: {
        itemsByTaskId: { [mocks.taskId]: Object.values(mocks.taskSessions.items) },
      },
      errands: {
        byTaskId: {},
        browserRevisionByTaskId: {},
        resetEpochByTaskId: {},
      },
      connection: { generation: 1, status: "connected" },
    }),
    subscribe: (listener: NavigationListener) => {
      mocks.appListeners.add(listener);
      return () => mocks.appListeners.delete(listener);
    },
  }),
}));

import { useOpenTaskSession } from "./use-open-task-session";
import { TaskSessionNavigationContext } from "./task-session-navigation-context";

const TARGET = {
  taskId: mocks.taskId,
  sessionId: mocks.errandId,
  title: "API audit",
  isErrand: true as const,
};

function appStateSnapshot() {
  return {
    ...mocks.appState,
    setActiveSession: mocks.setActiveSession,
    setPrimary: mocks.setPrimary,
    setTaskSession: mocks.setTaskSession,
    retainErrandInspection: vi.fn(),
    releaseErrandInspection: vi.fn(),
    taskSessions: mocks.taskSessions,
    taskSessionsByTask: {
      itemsByTaskId: { [mocks.taskId]: Object.values(mocks.taskSessions.items) },
    },
    errands: { byTaskId: {} },
  };
}

function setSelection(taskId: string, sessionId: string | null) {
  const previous = appStateSnapshot();
  mocks.appState.tasks = { activeTaskId: taskId, activeSessionId: sessionId };
  const next = appStateSnapshot();
  mocks.appListeners.forEach((listener) => listener(next, previous));
}

function inspectionFor(taskId: string, sessionId: string) {
  return {
    scope: { task_id: taskId, child_session_id: sessionId },
    errand: { task_id: taskId, errand_id: sessionId, parent_session_id: mocks.parentId },
    child_session_exists: true,
    parent: { task_id: taskId, task_environment_id: "environment-1" },
  };
}

function childResponse(taskId: string, sessionId: string) {
  return {
    session: {
      id: sessionId,
      task_id: taskId,
      role: "errand",
      parent_session_id: mocks.parentId,
    },
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.appListeners.clear();
  mocks.taskSessions.items = {
    [mocks.parentId]: { id: mocks.parentId, task_id: mocks.taskId },
    [mocks.errandId]: {
      id: mocks.errandId,
      task_id: mocks.taskId,
      role: "errand",
      parent_session_id: mocks.parentId,
    },
  };
  mocks.appState.tasks = { activeTaskId: mocks.taskId, activeSessionId: mocks.parentId };
  mocks.focusedPanels = [];
  mocks.usesDesktopWorkbench = true;
  mocks.api = {
    id: "dockview-api",
    getPanel: (id: string) => ({ api: { setActive: () => mocks.focusedPanels.push(id) } }),
  };
  mocks.inspectErrand.mockImplementation(async (taskId: string, sessionId: string) =>
    inspectionFor(taskId, sessionId),
  );
  mocks.fetchTaskSession.mockImplementation(async (sessionId: string) =>
    childResponse(mocks.taskId, sessionId),
  );
  mocks.setActiveSession.mockImplementation(setSelection);
  window.history.replaceState({}, "", `/t/${mocks.taskId}?sessionId=${mocks.parentId}`);
});

afterEach(() => {
  window.history.replaceState({}, "", "/");
});

describe("useOpenTaskSession", () => {
  it("opens a child conversation in its parent surface without adding a child tab", async () => {
    const { result } = renderHook(() => useOpenTaskSession());
    await act(async () => result.current(TARGET));
    expect(mocks.setActiveSession).toHaveBeenCalledWith(mocks.taskId, mocks.errandId);
    expect(mocks.addSessionPanel).not.toHaveBeenCalledWith(
      expect.anything(),
      expect.anything(),
      mocks.errandId,
      expect.anything(),
    );
    expect(mocks.setPrimary).not.toHaveBeenCalled();
  });

  it("makes the session active on phone and tablet layouts", async () => {
    mocks.usesDesktopWorkbench = false;
    const { result } = renderHook(() => useOpenTaskSession());

    await act(async () => result.current(TARGET));

    expect(mocks.setActiveSession).toHaveBeenCalledWith(mocks.taskId, mocks.errandId);
    expect(mocks.addSessionPanel).not.toHaveBeenCalled();
  });

  it("falls back to the active session while the workbench has no dockview API", async () => {
    mocks.api = null;
    const { result } = renderHook(() => useOpenTaskSession());

    await act(async () => result.current(TARGET));

    expect(mocks.setActiveSession).toHaveBeenCalledWith(mocks.taskId, mocks.errandId);
    expect(mocks.addSessionPanel).not.toHaveBeenCalled();
  });
});

it("preserves the selected child URL for reload without changing task route options", async () => {
  window.history.replaceState(
    { guard: "retained" },
    "",
    "/t/task-1?sessionId=parent-1&layout=wide#turn",
  );
  const { result } = renderHook(() => useOpenTaskSession());
  await act(async () => result.current(TARGET));
  expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(mocks.errandId);
  expect(new URL(window.location.href).searchParams.get("layout")).toBe("wide");
  expect(window.location.hash).toBe("#turn");
  expect(window.history.state).toEqual({ guard: "retained" });
  window.history.replaceState({}, "", "/");
});

it("selects the parent once when returning from a child on phone", async () => {
  mocks.usesDesktopWorkbench = false;
  const { result } = renderHook(() => useOpenTaskSession());
  await act(async () =>
    result.current({ taskId: mocks.taskId, sessionId: mocks.parentId, title: "Main" }),
  );
  expect(mocks.setActiveSession).toHaveBeenCalledExactlyOnceWith(mocks.taskId, mocks.parentId);
});

it("opens an uncached child by inspecting one row and fetching only its detail", async () => {
  mocks.taskSessions.items = {
    [mocks.parentId]: { id: mocks.parentId, task_id: mocks.taskId },
    [mocks.ordinaryId]: { id: mocks.ordinaryId, task_id: mocks.taskId },
  };
  const { result } = renderHook(() => useOpenTaskSession());

  await result.current(TARGET);

  expect(mocks.inspectErrand).toHaveBeenCalledExactlyOnceWith(
    mocks.taskId,
    mocks.errandId,
    expect.any(AbortSignal),
  );
  expect(mocks.fetchTaskSession).toHaveBeenCalledExactlyOnceWith(mocks.errandId, {
    cache: "no-store",
  });
  expect(mocks.setTaskSession).toHaveBeenCalledExactlyOnceWith({
    id: mocks.errandId,
    task_id: mocks.taskId,
    role: "errand",
    parent_session_id: mocks.parentId,
  });
  expect(mocks.setActiveSession).toHaveBeenCalledExactlyOnceWith(mocks.taskId, mocks.errandId);
  expect(mocks.addSessionPanel).not.toHaveBeenCalled();
});

it("discards an inspect response after an ordinary conversation is selected", async () => {
  mocks.taskSessions.items = {
    [mocks.parentId]: { id: mocks.parentId, task_id: mocks.taskId },
    [mocks.ordinaryId]: { id: mocks.ordinaryId, task_id: mocks.taskId },
  };
  const inspection = deferred<ReturnType<typeof inspectionFor>>();
  mocks.inspectErrand.mockReturnValueOnce(inspection.promise);
  const { result } = renderHook(() => useOpenTaskSession());

  const pending = result.current(TARGET);
  await waitFor(() => expect(mocks.inspectErrand).toHaveBeenCalledOnce());
  await act(async () => {
    await result.current({ taskId: mocks.taskId, sessionId: mocks.ordinaryId, title: "Ordinary" });
  });
  inspection.resolve(inspectionFor(mocks.taskId, mocks.errandId));
  await act(async () => pending);

  expect(mocks.fetchTaskSession).not.toHaveBeenCalled();
  expect(mocks.setTaskSession).not.toHaveBeenCalled();
  expect(mocks.appState.tasks).toEqual({
    activeTaskId: mocks.taskId,
    activeSessionId: mocks.ordinaryId,
  });
  expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(mocks.ordinaryId);
  expect(mocks.focusedPanels).not.toContain("session:parent-1");
});

it("discards delayed child detail after ordinary navigation without restoring its URL or focus", async () => {
  mocks.taskSessions.items = {
    [mocks.parentId]: { id: mocks.parentId, task_id: mocks.taskId },
    [mocks.ordinaryId]: { id: mocks.ordinaryId, task_id: mocks.taskId },
  };
  const detail = deferred<ReturnType<typeof childResponse>>();
  mocks.fetchTaskSession.mockReturnValueOnce(detail.promise);
  const { result } = renderHook(() => useOpenTaskSession());

  const pending = result.current(TARGET);
  await waitFor(() => expect(mocks.fetchTaskSession).toHaveBeenCalledOnce());
  await act(async () => {
    await result.current({ taskId: mocks.taskId, sessionId: mocks.ordinaryId, title: "Ordinary" });
  });
  detail.resolve(childResponse(mocks.taskId, mocks.errandId));
  await act(async () => pending);

  expect(mocks.setTaskSession).not.toHaveBeenCalled();
  expect(mocks.appState.tasks).toEqual({
    activeTaskId: mocks.taskId,
    activeSessionId: mocks.ordinaryId,
  });
  expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(mocks.ordinaryId);
  expect(mocks.focusedPanels).not.toContain("session:parent-1");
});

it("keeps only the replacement child open when two uncached opens overlap", async () => {
  mocks.taskSessions.items = {
    [mocks.parentId]: { id: mocks.parentId, task_id: mocks.taskId },
    [mocks.ordinaryId]: { id: mocks.ordinaryId, task_id: mocks.taskId },
  };
  const firstInspection = deferred<ReturnType<typeof inspectionFor>>();
  const secondInspection = deferred<ReturnType<typeof inspectionFor>>();
  mocks.inspectErrand
    .mockReturnValueOnce(firstInspection.promise)
    .mockReturnValueOnce(secondInspection.promise);
  const { result } = renderHook(() => useOpenTaskSession());

  const first = result.current(TARGET);
  const second = result.current({
    taskId: mocks.taskId,
    sessionId: mocks.secondErrandId,
    title: "Second",
    isErrand: true,
  });
  firstInspection.resolve(inspectionFor(mocks.taskId, mocks.errandId));
  await act(async () => first);
  expect(mocks.fetchTaskSession).not.toHaveBeenCalled();

  secondInspection.resolve(inspectionFor(mocks.taskId, mocks.secondErrandId));
  await act(async () => second);

  expect(mocks.setTaskSession).toHaveBeenCalledExactlyOnceWith(
    childResponse(mocks.taskId, mocks.secondErrandId).session,
  );
  expect(mocks.appState.tasks).toEqual({
    activeTaskId: mocks.taskId,
    activeSessionId: mocks.secondErrandId,
  });
  expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(mocks.secondErrandId);
  expect(mocks.focusedPanels.filter((id) => id === "session:parent-1")).toHaveLength(1);
});

it("rejects an uncached child response after task selection leaves and returns to the same ids", async () => {
  mocks.taskSessions.items = {
    [mocks.parentId]: { id: mocks.parentId, task_id: mocks.taskId },
  };
  const inspection = deferred<ReturnType<typeof inspectionFor>>();
  mocks.inspectErrand.mockReturnValueOnce(inspection.promise);
  const { result } = renderHook(() => useOpenTaskSession());

  const pending = result.current(TARGET);
  setSelection(mocks.otherTaskId, mocks.otherSessionId);
  setSelection(mocks.taskId, mocks.parentId);
  inspection.resolve(inspectionFor(mocks.taskId, mocks.errandId));
  await act(async () => pending);

  expect(mocks.fetchTaskSession).not.toHaveBeenCalled();
  expect(mocks.setTaskSession).not.toHaveBeenCalled();
  expect(mocks.appState.tasks).toEqual({
    activeTaskId: mocks.taskId,
    activeSessionId: mocks.parentId,
  });
  expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(mocks.parentId);
  expect(mocks.focusedPanels).toEqual([]);
});

it("does not publish a delayed child response after the opener unmounts", async () => {
  mocks.taskSessions.items = {
    [mocks.parentId]: { id: mocks.parentId, task_id: mocks.taskId },
  };
  const inspection = deferred<ReturnType<typeof inspectionFor>>();
  mocks.inspectErrand.mockReturnValueOnce(inspection.promise);
  const { result, unmount } = renderHook(() => useOpenTaskSession());

  const pending = result.current(TARGET);
  unmount();
  inspection.resolve(inspectionFor(mocks.taskId, mocks.errandId));
  await pending;

  expect(mocks.fetchTaskSession).not.toHaveBeenCalled();
  expect(mocks.setTaskSession).not.toHaveBeenCalled();
  expect(mocks.setActiveSession).not.toHaveBeenCalled();
  expect(mocks.focusedPanels).toEqual([]);
});

it("does not let an older inspect replace a cached child opened later", async () => {
  mocks.taskSessions.items = {
    [mocks.parentId]: { id: mocks.parentId, task_id: mocks.taskId },
  };
  const inspection = deferred<ReturnType<typeof inspectionFor>>();
  mocks.inspectErrand.mockReturnValueOnce(inspection.promise);
  const { result } = renderHook(() => useOpenTaskSession());

  const pending = result.current(TARGET);
  mocks.taskSessions.items[mocks.errandId] = {
    id: mocks.errandId,
    task_id: mocks.taskId,
    role: "errand",
    parent_session_id: mocks.parentId,
  };
  const laterPending = result.current(TARGET);
  inspection.resolve(inspectionFor(mocks.taskId, mocks.errandId));
  await act(async () => Promise.all([pending, laterPending]));

  expect(mocks.fetchTaskSession).not.toHaveBeenCalled();
  expect(mocks.setTaskSession).not.toHaveBeenCalled();
  expect(mocks.appState.tasks).toEqual({
    activeTaskId: mocks.taskId,
    activeSessionId: mocks.errandId,
  });
  expect(new URL(window.location.href).searchParams.get("sessionId")).toBe(mocks.errandId);
});

it("invalidates an earlier child read when navigation moves to its embedded owner", async () => {
  mocks.taskSessions.items = {
    [mocks.parentId]: { id: mocks.parentId, task_id: mocks.taskId },
  };
  const inspection = deferred<ReturnType<typeof inspectionFor>>();
  mocks.inspectErrand.mockReturnValueOnce(inspection.promise);
  let owner: ((target: { taskId: string; sessionId: string; title: string }) => void) | null = null;
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(TaskSessionNavigationContext.Provider, { value: owner }, children);
  const { result, rerender } = renderHook(() => useOpenTaskSession(), { wrapper });

  expect(typeof result.current).toBe("function");

  const pending = result.current(TARGET);
  const ownerOpen = vi.fn();
  owner = ownerOpen;
  rerender();
  const ownerPending = result.current(TARGET);
  inspection.resolve(inspectionFor(mocks.taskId, mocks.errandId));
  await act(async () => Promise.all([pending, ownerPending]));

  expect(ownerOpen).toHaveBeenCalledExactlyOnceWith(
    expect.objectContaining({
      taskId: mocks.taskId,
      sessionId: mocks.errandId,
      isErrand: true,
      inspection: expect.any(Object),
    }),
  );
  expect(mocks.fetchTaskSession).toHaveBeenCalledExactlyOnceWith(mocks.errandId, {
    cache: "no-store",
  });
  expect(mocks.setTaskSession).toHaveBeenCalledExactlyOnceWith(
    expect.objectContaining({ id: mocks.errandId, role: "errand" }),
  );
  expect(mocks.setActiveSession).not.toHaveBeenCalled();
});
