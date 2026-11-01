import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  usesDesktopWorkbench: true,
  api: { id: "dockview-api" } as unknown,
  addSessionPanel: vi.fn(),
  clearHiddenSessionPanel: vi.fn(),
  markIntent: vi.fn(),
  setActiveSession: vi.fn(),
  setPrimary: vi.fn(),
  taskSessions: {
    items: {
      "errand-1": {
        id: "errand-1",
        task_id: "task-1",
        role: "errand",
        parent_session_id: "parent-1",
      },
      "parent-1": { id: "parent-1", task_id: "task-1" },
    },
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
vi.mock("../../dockview-hidden-session-panels", () => ({
  clearHiddenSessionPanel: mocks.clearHiddenSessionPanel,
}));
vi.mock("../../session-tab-activation-intent", () => ({
  markSessionTabUserActivationIntent: mocks.markIntent,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({
      setActiveSession: mocks.setActiveSession,
      setPrimary: mocks.setPrimary,
      taskSessions: mocks.taskSessions,
      taskSessionsByTask: { itemsByTaskId: { "task-1": Object.values(mocks.taskSessions.items) } },
      errands: { byTaskId: {} },
    }),
  }),
}));

import { useOpenTaskSession } from "./use-open-task-session";

const TARGET = { taskId: "task-1", sessionId: "errand-1", title: "API audit" };

beforeEach(() => {
  vi.clearAllMocks();
  mocks.usesDesktopWorkbench = true;
  mocks.api = { id: "dockview-api" };
});

describe("useOpenTaskSession", () => {
  it("opens a child conversation in its parent surface without adding a child tab", () => {
    const { result } = renderHook(() => useOpenTaskSession());
    result.current(TARGET);
    expect(mocks.setActiveSession).toHaveBeenCalledWith("task-1", "errand-1");
    expect(mocks.addSessionPanel).not.toHaveBeenCalledWith(
      expect.anything(),
      expect.anything(),
      "errand-1",
      expect.anything(),
    );
    expect(mocks.setPrimary).not.toHaveBeenCalled();
  });

  it("makes the session active on phone and tablet layouts", () => {
    mocks.usesDesktopWorkbench = false;
    const { result } = renderHook(() => useOpenTaskSession());

    result.current(TARGET);

    expect(mocks.setActiveSession).toHaveBeenCalledWith("task-1", "errand-1");
    expect(mocks.addSessionPanel).not.toHaveBeenCalled();
  });

  it("falls back to the active session while the workbench has no dockview API", () => {
    mocks.api = null;
    const { result } = renderHook(() => useOpenTaskSession());

    result.current(TARGET);

    expect(mocks.setActiveSession).toHaveBeenCalledWith("task-1", "errand-1");
    expect(mocks.addSessionPanel).not.toHaveBeenCalled();
  });
});

it("preserves the selected child URL for reload without changing task route options", () => {
  window.history.replaceState(
    { guard: "retained" },
    "",
    "/t/task-1?sessionId=parent-1&layout=wide#turn",
  );
  const { result } = renderHook(() => useOpenTaskSession());
  result.current(TARGET);
  expect(new URL(window.location.href).searchParams.get("sessionId")).toBe("errand-1");
  expect(new URL(window.location.href).searchParams.get("layout")).toBe("wide");
  expect(window.location.hash).toBe("#turn");
  expect(window.history.state).toEqual({ guard: "retained" });
  window.history.replaceState({}, "", "/");
});

it("selects the parent once when returning from a child on phone", () => {
  mocks.usesDesktopWorkbench = false;
  const { result } = renderHook(() => useOpenTaskSession());
  result.current({ taskId: "task-1", sessionId: "parent-1", title: "Main" });
  expect(mocks.setActiveSession).toHaveBeenCalledExactlyOnceWith("task-1", "parent-1");
});
