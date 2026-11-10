import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  unsubscribe: vi.fn(),
  subscribeSession: vi.fn(),
  setContextWindow: vi.fn(),
  state: {
    contextWindow: { bySessionId: {} },
    taskSessions: { items: {}, contextWindowEpochBySession: {} as Record<string, number> },
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
  mocks.state.taskSessions.contextWindowEpochBySession = {};
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

  it("reads retained and cached context without writing or subscribing while inactive", () => {
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
    expect(result.current).toMatchObject({ size: 200_000, used: 56_047, compactionCount: 2 });
    expect(mocks.state.setContextWindow).not.toHaveBeenCalled();
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
    expect(mocks.state.setContextWindow).not.toHaveBeenCalled();
    expect(mocks.subscribeSession).not.toHaveBeenCalled();
  });

  it("does not restore retained metadata after a live invalidation removed the cache", () => {
    const sessions = mocks.state.taskSessions.items as Record<
      string,
      { metadata?: Record<string, unknown> }
    >;
    sessions["session-1"] = {
      metadata: {
        context_window: {
          size: 200_000,
          used: 56_047,
          remaining: 143_953,
          efficiency: 28,
          source: "acp",
        },
        context_compaction_count: 2,
      },
    };
    mocks.state.taskSessions.contextWindowEpochBySession = { "session-1": 1 };

    const { result } = renderHook(() => useSessionContextWindow("session-1", false));

    expect(result.current).toBeUndefined();
    expect(mocks.state.setContextWindow).not.toHaveBeenCalled();
    expect(mocks.subscribeSession).not.toHaveBeenCalled();
  });
});

describe("useSessionContextWindow hydration", () => {
  it("carries persisted native reporting and the lifetime count", () => {
    mocks.state.contextWindow.bySessionId = {};
    mocks.state.taskSessions.items = {
      "session-2": {
        id: "session-2",
        metadata: {
          context_window: {
            size: 272_000,
            used: 9_695,
            remaining: 262_305,
            efficiency: 3.6,
            source: "acp",
          },
          context_compaction_count: 4,
          context_compaction_reporting: {
            source: "copilot_native",
            availability: "available",
            coverage: "partial",
            revision: 5,
          },
        },
      },
    };
    mocks.state.setContextWindow = mocks.setContextWindow;

    renderHook(() => useSessionContextWindow("session-2", true));

    expect(mocks.setContextWindow).toHaveBeenCalledWith(
      "session-2",
      expect.objectContaining({
        compactionCount: 4,
        compactionReporting: expect.objectContaining({ coverage: "partial", revision: 5 }),
      }),
    );
  });
});

describe("useSessionContextWindow retained metadata reconciliation", () => {
  it("merges a newer authoritative metadata count into an existing gauge", () => {
    mocks.setContextWindow = vi.fn();
    mocks.state.setContextWindow = mocks.setContextWindow;
    mocks.state.contextWindow.bySessionId = {
      "session-3": {
        size: 272_000,
        used: 9_695,
        remaining: 262_305,
        efficiency: 4,
        compactionCount: 2,
        compactionReporting: {
          source: "copilot_native",
          availability: "available",
          coverage: "observed",
          revision: 2,
        },
      },
    };
    mocks.state.taskSessions.items = {
      "session-3": {
        id: "session-3",
        metadata: {
          context_window: {
            size: 272_000,
            used: 9_695,
            remaining: 262_305,
            efficiency: 4,
            source: "acp",
          },
          context_compaction_count: 5,
          context_compaction_reporting: {
            source: "copilot_native",
            availability: "available",
            coverage: "partial",
            revision: 4,
          },
        },
      },
    };

    renderHook(() => useSessionContextWindow("session-3", true));

    expect(mocks.setContextWindow).toHaveBeenCalledWith(
      "session-3",
      expect.objectContaining({
        compactionCount: 5,
        compactionReporting: expect.objectContaining({ revision: 4, coverage: "partial" }),
      }),
    );
  });

  it("hydrates retained metadata after the live cache is cleared", () => {
    mocks.setContextWindow = vi.fn();
    mocks.state.setContextWindow = mocks.setContextWindow;
    mocks.state.contextWindow.bySessionId = {};
    mocks.state.taskSessions.items = {
      "session-4": {
        id: "session-4",
        metadata: {
          context_window: {
            size: 272_000,
            used: 9_695,
            remaining: 262_305,
            efficiency: 4,
            source: "acp",
          },
          context_compaction_count: 3,
        },
      },
    };

    renderHook(() => useSessionContextWindow("session-4", true));

    expect(mocks.setContextWindow).toHaveBeenCalledWith(
      "session-4",
      expect.objectContaining({ compactionCount: 3, size: 272_000 }),
    );
  });
});
