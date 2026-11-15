import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useRetainedReaderPosition } from "./use-retained-reader-position";

const scheduled = vi.hoisted(() => [] as { run: () => void; cancelled: boolean }[]);
const resizeCallbacks = new Set<() => void>();
vi.mock("./transcript-auto-scroll", () => ({
  scheduleAfterPanelRestore: (run: () => void) => {
    const entry = { run, cancelled: false };
    scheduled.push(entry);
    return () => {
      entry.cancelled = true;
    };
  },
}));

function flushRestores() {
  act(() => {
    for (const entry of scheduled.splice(0)) if (!entry.cancelled) entry.run();
  });
}

function fixture(inactive = false) {
  const panel = document.createElement("div");
  const scroller = document.createElement("div");
  scroller.className = "chat-message-list";
  const anchor = document.createElement("div");
  anchor.id = "msg-history";
  scroller.append(anchor);
  panel.append(scroller);
  document.body.append(panel);
  const geometry = { top: 200, content: 1000, anchorTop: 150 };
  Object.defineProperties(scroller, {
    scrollTop: {
      configurable: true,
      get: () => geometry.top,
      set: (top: number) => {
        geometry.top = top;
      },
    },
    scrollHeight: { configurable: true, get: () => geometry.content },
    clientHeight: { configurable: true, get: () => 200 },
  });
  scroller.getBoundingClientRect = () => ({ top: 100, bottom: 300 }) as DOMRect;
  anchor.getBoundingClientRect = () =>
    ({
      top: 100 + geometry.anchorTop - geometry.top,
      bottom: 200 + geometry.anchorTop - geometry.top,
    }) as DOMRect;
  const panelRef = { current: panel };
  const props = {
    inactive,
    sessionId: "parent",
    generation: 0,
    hasTarget: false,
    hasLocalTarget: false,
    visible: true,
  };
  const view = renderHook((input) => useRetainedReaderPosition({ panelRef, ...input }), {
    initialProps: props,
  });
  return { ...view, props, geometry, scroller, anchor };
}

beforeEach(() => {
  scheduled.length = 0;
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(private run: () => void) {
        resizeCallbacks.add(run);
      }
      observe() {}
      unobserve() {}
      disconnect() {
        resizeCallbacks.delete(this.run);
      }
    },
  );
});
afterEach(() => {
  cleanup();
  resizeCallbacks.clear();
  vi.unstubAllGlobals();
  document.body.replaceChildren();
});

describe("retained parent reader ownership", () => {
  it("restores a scrolled-up parent's visual anchor after a native reactivation shift", () => {
    const f = fixture();
    f.rerender({ ...f.props, inactive: true });
    f.geometry.top = 283;
    f.rerender(f.props);
    flushRestores();
    expect(f.geometry.top).toBe(200);
  });

  it("preserves the same reading anchor when older rows prepend while covered", () => {
    const f = fixture();
    f.rerender({ ...f.props, inactive: true });
    f.geometry.content += 300;
    f.geometry.anchorTop += 300;
    f.geometry.top = 583;
    f.rerender(f.props);
    flushRestores();
    expect(f.geometry.top).toBe(500);
  });

  it("lets an existing bottom follower catch up instead of restoring an old edge", () => {
    const f = fixture();
    f.geometry.top = 800;
    f.rerender({ ...f.props, inactive: true });
    f.geometry.content = 1200;
    f.geometry.top = 1000;
    f.rerender(f.props);
    flushRestores();
    expect(f.geometry.top).toBe(1000);
  });

  it("does not claim an unseen parent first mounted behind a direct child URL", () => {
    const f = fixture(true);
    f.geometry.top = 600;
    f.rerender({ ...f.props, inactive: false });
    flushRestores();
    expect(f.geometry.top).toBe(600);
  });

  it("yields to a newer explicit navigation target or history reset", () => {
    for (const change of [{ hasTarget: true }, { hasLocalTarget: true }, { generation: 1 }]) {
      const f = fixture();
      f.rerender({ ...f.props, inactive: true });
      f.geometry.top = 300;
      f.rerender({ ...f.props, ...change });
      flushRestores();
      expect(f.geometry.top).toBe(300);
      f.unmount();
    }
  });

  it("yields to user input after Back and cancels pending restoration on unmount", () => {
    const f = fixture();
    f.rerender({ ...f.props, inactive: true });
    f.geometry.top = 283;
    f.rerender(f.props);
    f.scroller.dispatchEvent(new WheelEvent("wheel", { deltaY: 80, bubbles: true }));
    f.geometry.top = 363;
    flushRestores();
    expect(f.geometry.top).toBe(363);
    f.rerender({ ...f.props, inactive: true });
    f.rerender(f.props);
    f.unmount();
    flushRestores();
    expect(f.geometry.top).toBe(363);
  });

  it("does not guess an offset when its anchor disappears and history changes", () => {
    const f = fixture();
    f.rerender({ ...f.props, inactive: true });
    f.anchor.remove();
    f.geometry.content = 2000;
    f.geometry.top = 400;
    f.rerender(f.props);
    flushRestores();
    expect(f.geometry.top).toBe(400);
  });

  it("keeps the original reading anchor through rapid Back and another child Open", () => {
    const f = fixture();
    f.rerender({ ...f.props, inactive: true });
    f.geometry.top = 283;
    f.rerender(f.props);
    f.rerender({ ...f.props, inactive: true });
    f.rerender(f.props);
    flushRestores();
    expect(f.geometry.top).toBe(200);
  });
});

describe("retained parent after reactivation", () => {
  it("releases ownership when the normal conversation leaves the visible surface", () => {
    const f = fixture();
    f.rerender({ ...f.props, inactive: true });
    f.rerender(f.props);
    flushRestores();
    f.rerender({ ...f.props, visible: false });
    expect(f.scroller.style.overflowAnchor).toBe("");
    f.geometry.top = 400;
    f.rerender(f.props);
    flushRestores();
    expect(f.geometry.top).toBe(400);
  });

  it("yields to input before a child control can stop event propagation", () => {
    const f = fixture();
    f.rerender({ ...f.props, inactive: true });
    f.rerender(f.props);
    flushRestores();
    f.anchor.addEventListener("keydown", (event) => event.stopPropagation());
    f.anchor.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }));
    expect(f.scroller.style.overflowAnchor).toBe("");
    f.geometry.top = 400;
    f.scroller.dispatchEvent(new Event("scroll"));
    flushRestores();
    expect(f.geometry.top).toBe(400);
  });
  it("owns the reading anchor through late browser scroll correction until user input", () => {
    const f = fixture();
    f.rerender({ ...f.props, inactive: true });
    f.rerender(f.props);
    flushRestores();
    f.geometry.top = 283;
    f.scroller.dispatchEvent(new Event("scroll"));
    flushRestores();
    expect(f.geometry.top).toBe(200);
    expect(f.scroller.style.overflowAnchor).toBe("none");
    f.scroller.dispatchEvent(new WheelEvent("wheel", { bubbles: true, deltaY: 20 }));
    expect(f.scroller.style.overflowAnchor).toBe("");
  });

  it("preserves the visual anchor through late content reflow after returning", () => {
    const f = fixture();
    f.rerender({ ...f.props, inactive: true });
    f.rerender(f.props);
    flushRestores();
    f.geometry.content += 300;
    f.geometry.anchorTop += 300;
    for (const callback of resizeCallbacks) callback();
    flushRestores();
    expect(f.geometry.top).toBe(500);
  });
});
