import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  unsubscribe: vi.fn(),
  subscribeSession: vi.fn(),
  setContextWindow: vi.fn(),
  state: {
    contextWindow: { bySessionId: {} },
    taskSessions: { items: {} },
    connection: { status: "connected" },
    setContextWindow: vi.fn(),
  },
}));

mocks.subscribeSession.mockReturnValue(mocks.unsubscribe);

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ subscribeSession: mocks.subscribeSession }),
}));

import { useSessionContextWindow } from "./use-session-context-window";

afterEach(() => {
  cleanup();
  mocks.unsubscribe.mockClear();
  mocks.subscribeSession.mockClear();
  mocks.state.contextWindow.bySessionId = {};
  mocks.state.taskSessions.items = {};
  mocks.state.setContextWindow.mockReset();
});

describe("useSessionContextWindow detail demand", () => {
  it("does not subscribe while its chat panel is hidden", () => {
    renderHook(() => useSessionContextWindow("session-1", false));

    expect(mocks.subscribeSession).not.toHaveBeenCalled();
  });

  it("subscribes when revealed and releases the session on hide", () => {
    const { rerender, unmount } = renderHook(
      ({ detailActive }: { detailActive: boolean }) =>
        useSessionContextWindow("session-1", detailActive),
      { initialProps: { detailActive: false } },
    );

    expect(mocks.subscribeSession).not.toHaveBeenCalled();

    rerender({ detailActive: true });
    expect(mocks.subscribeSession).toHaveBeenCalledTimes(1);
    expect(mocks.subscribeSession).toHaveBeenCalledWith("session-1");

    rerender({ detailActive: false });
    expect(mocks.unsubscribe).toHaveBeenCalledTimes(1);
    unmount();
    expect(mocks.unsubscribe).toHaveBeenCalledTimes(1);
  });

  it("restores, tracks, and clears shared context without a child detail subscription", () => {
    const contextBySession = mocks.state.contextWindow.bySessionId as Record<string, unknown>;
    const sessions = mocks.state.taskSessions.items as Record<
      string,
      { metadata?: Record<string, unknown> }
    >;
    const persistedWindow = {
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      source: "acp",
    };
    sessions["session-1"] = {
      metadata: { context_window: persistedWindow, context_compaction_count: 2 },
    };
    mocks.state.setContextWindow.mockImplementation((sessionId: string, entry: unknown) => {
      contextBySession[sessionId] = entry;
    });

    const { result, rerender } = renderHook(() => useSessionContextWindow("session-1", false));
    expect(mocks.state.setContextWindow).toHaveBeenCalledWith(
      "session-1",
      expect.objectContaining({ size: 200_000, used: 56_047, compactionCount: 2 }),
    );
    expect(mocks.subscribeSession).not.toHaveBeenCalled();

    rerender();
    expect(result.current).toMatchObject({ size: 200_000, used: 56_047, compactionCount: 2 });

    contextBySession["session-1"] = {
      size: 200_000,
      used: 170_000,
      remaining: 30_000,
      efficiency: 85,
      compactionCount: 2,
    };
    rerender();
    expect(result.current?.used).toBe(170_000);

    sessions["session-1"].metadata = { context_window: null };
    delete contextBySession["session-1"];
    rerender();
    expect(result.current).toBeUndefined();
    expect(mocks.state.setContextWindow).toHaveBeenCalledTimes(1);
    expect(mocks.subscribeSession).not.toHaveBeenCalled();
  });
});
