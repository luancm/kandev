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
    getState: () => ({ setActiveSession: mocks.setActiveSession, setPrimary: mocks.setPrimary }),
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
  it("adds or focuses the session panel on the desktop workbench", () => {
    const { result } = renderHook(() => useOpenTaskSession());

    result.current(TARGET);

    expect(mocks.markIntent).toHaveBeenCalledWith("errand-1");
    expect(mocks.clearHiddenSessionPanel).toHaveBeenCalledWith(mocks.api, "errand-1");
    expect(mocks.addSessionPanel).toHaveBeenCalledWith(
      mocks.api,
      "center",
      "errand-1",
      "API audit",
    );
    expect(mocks.markIntent.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.addSessionPanel.mock.invocationCallOrder[0],
    );
    expect(mocks.setActiveSession).not.toHaveBeenCalled();
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
