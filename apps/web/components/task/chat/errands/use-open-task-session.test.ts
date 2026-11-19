import { renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { TaskSessionNavigationContext } from "./task-session-navigation-context";
import { useOpenTaskSession } from "./use-open-task-session";

const mocks = vi.hoisted(() => ({
  usesDesktopWorkbench: true,
  api: {} as object | null,
  addSessionPanel: vi.fn(),
  setActiveSession: vi.fn(),
  clearHiddenSessionPanel: vi.fn(),
  markIntent: vi.fn(),
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
      tasks: { activeTaskId: "task-1", activeSessionId: "initial" },
      taskSessions: { items: { parent: { id: "parent", task_id: "task-1" } } },
      setActiveSession: mocks.setActiveSession,
    }),
    subscribe: () => () => {},
  }),
}));

const target = { taskId: "task-1", sessionId: "parent", title: "Main" };

beforeEach(() => {
  vi.clearAllMocks();
  mocks.usesDesktopWorkbench = true;
  mocks.api = {};
  window.history.replaceState(
    { guard: "retained" },
    "",
    "/t/task-1?sessionId=initial&layout=wide#turn",
  );
});

afterEach(() => window.history.replaceState({}, "", "/"));

it("opens an ordinary desktop tab and preserves the route options", async () => {
  const { result } = renderHook(() => useOpenTaskSession());
  expect(await result.current(target)).toBe(true);
  expect(mocks.setActiveSession).toHaveBeenCalledExactlyOnceWith("task-1", "parent");
  expect(mocks.markIntent).toHaveBeenCalledExactlyOnceWith("parent");
  expect(mocks.clearHiddenSessionPanel).toHaveBeenCalledExactlyOnceWith(mocks.api, "parent");
  expect(mocks.addSessionPanel).toHaveBeenCalledExactlyOnceWith(
    mocks.api,
    "center",
    "parent",
    "Main",
  );
  expect(window.location.search).toBe("?sessionId=parent&layout=wide");
  expect(window.location.hash).toBe("#turn");
  expect(window.history.state).toEqual({ guard: "retained" });
});

it.each(["phone", "workbench startup"] as const)(
  "selects an ordinary session during %s",
  async (mode) => {
    mocks.usesDesktopWorkbench = mode !== "phone";
    if (mode === "workbench startup") mocks.api = null;
    const { result } = renderHook(() => useOpenTaskSession());
    expect(await result.current(target)).toBe(true);
    expect(mocks.setActiveSession).toHaveBeenCalledExactlyOnceWith("task-1", "parent");
    expect(mocks.addSessionPanel).not.toHaveBeenCalled();
  },
);

it("lets the embedded surface own an ordinary session selection", async () => {
  const open = vi.fn();
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(TaskSessionNavigationContext.Provider, { value: open }, children);
  const { result } = renderHook(() => useOpenTaskSession(), { wrapper });
  expect(await result.current(target)).toBe(true);
  expect(open).toHaveBeenCalledExactlyOnceWith(target);
  expect(mocks.setActiveSession).not.toHaveBeenCalled();
  expect(mocks.addSessionPanel).not.toHaveBeenCalled();
});
