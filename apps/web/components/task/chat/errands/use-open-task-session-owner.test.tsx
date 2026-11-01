import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { expect, it, vi } from "vitest";
import {
  TaskSessionNavigationContext,
  useScopedTaskSessionNavigation,
} from "./task-session-navigation-context";
import { useOpenTaskSession } from "./use-open-task-session";
const mocks = vi.hoisted(() => ({ setActive: vi.fn(), addPanel: vi.fn() }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ usesDesktopWorkbench: true }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => ({ setActiveSession: mocks.setActive }) }),
}));
vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: { getState: () => ({ api: {} }) },
}));
vi.mock("@/lib/state/dockview-panel-actions", () => ({ addSessionPanel: mocks.addPanel }));
vi.mock("../../dockview-hidden-session-panels", () => ({ clearHiddenSessionPanel: vi.fn() }));
vi.mock("../../session-tab-activation-intent", () => ({
  markSessionTabUserActivationIntent: vi.fn(),
}));
// @covers AC-TASKS-ERRANDS-010.2 AC-TASKS-ERRANDS-011.1
it("uses the embedded conversation owner for Open and Back", () => {
  const open = vi.fn();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <TaskSessionNavigationContext.Provider value={open}>
      {children}
    </TaskSessionNavigationContext.Provider>
  );
  const { result } = renderHook(() => useOpenTaskSession(), { wrapper });
  const child = { taskId: "task", sessionId: "child", title: "Audit" };
  result.current(child);
  result.current({ taskId: "task", sessionId: "parent", title: "Main" });
  expect(open).toHaveBeenNthCalledWith(1, child);
  expect(open).toHaveBeenNthCalledWith(2, { taskId: "task", sessionId: "parent", title: "Main" });
  expect(mocks.addPanel).not.toHaveBeenCalled();
  expect(mocks.setActive).not.toHaveBeenCalled();
});

it("keeps a scoped selector inert for foreign tasks and absent controlled callbacks", () => {
  const selected = vi.fn();
  const { result, rerender } = renderHook(
    ({ taskId, callback }) => useScopedTaskSessionNavigation(taskId, callback),
    {
      initialProps: { taskId: "task-a", callback: selected as ((id: string) => void) | undefined },
    },
  );
  result.current({ taskId: "task-b", sessionId: "child", title: "Audit" });
  expect(selected).not.toHaveBeenCalled();
  result.current({ taskId: "task-a", sessionId: "child", title: "Audit" });
  expect(selected).toHaveBeenCalledExactlyOnceWith("child");
  rerender({ taskId: "task-b", callback: undefined });
  result.current({ taskId: "task-a", sessionId: "child", title: "Audit" });
  result.current({ taskId: "task-b", sessionId: "parent", title: "Main" });
  expect(selected).toHaveBeenCalledTimes(1);
});
