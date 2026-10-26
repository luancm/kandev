import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";

const api = vi.hoisted(() => ({
  cancelErrand: vi.fn(),
  cancelAllErrands: vi.fn(),
  sendErrandResult: vi.fn(),
}));
vi.mock("@/lib/api/domains/errand-api", () => api);

import { useErrandActions } from "./use-errand-actions";

const ERRAND_SESSION_ID = "session-errand";

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

function deferred<T>() {
  let resolve: (value: T) => void = () => {};
  let reject: (error: Error) => void = () => {};
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function useTwoPanels() {
  return { chip: useErrandActions(), header: useErrandActions() };
}

afterEach(cleanup);

describe("useErrandActions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.cancelErrand.mockResolvedValue(undefined);
    api.cancelAllErrands.mockResolvedValue(2);
    api.sendErrandResult.mockResolvedValue(undefined);
  });

  it("sends each errand action with its target", async () => {
    const { result } = renderHook(() => useErrandActions(), { wrapper });

    await act(async () => {
      await result.current.cancelErrand(ERRAND_SESSION_ID);
      await expect(result.current.cancelAllErrands("task-1")).resolves.toBe(2);
      await result.current.sendErrandResult(ERRAND_SESSION_ID);
    });

    expect(api.cancelErrand).toHaveBeenCalledExactlyOnceWith(ERRAND_SESSION_ID);
    expect(api.cancelAllErrands).toHaveBeenCalledExactlyOnceWith("task-1");
    expect(api.sendErrandResult).toHaveBeenCalledExactlyOnceWith(ERRAND_SESSION_ID);
  });

  it("shares one in-flight request for the same action and target", async () => {
    const pending = deferred<void>();
    api.cancelErrand.mockReturnValue(pending.promise);
    const { result } = renderHook(useTwoPanels, { wrapper });

    let first: Promise<void> | undefined;
    let second: Promise<void> | undefined;
    act(() => {
      first = result.current.chip.cancelErrand(ERRAND_SESSION_ID);
      second = result.current.header.cancelErrand(ERRAND_SESSION_ID);
    });
    expect(api.cancelErrand).toHaveBeenCalledTimes(1);

    await act(async () => {
      pending.resolve();
      await Promise.all([first, second]);
    });
    await act(() => result.current.chip.cancelErrand(ERRAND_SESSION_ID));
    expect(api.cancelErrand).toHaveBeenCalledTimes(2);
  });

  it("reports an action as pending until it settles", async () => {
    const pending = deferred<void>();
    api.sendErrandResult.mockReturnValue(pending.promise);
    const { result } = renderHook(() => useErrandActions(), { wrapper });

    let request: Promise<void> | undefined;
    act(() => {
      request = result.current.sendErrandResult(ERRAND_SESSION_ID);
    });
    expect(result.current.isPending("send_result", ERRAND_SESSION_ID)).toBe(true);
    expect(result.current.isPending("cancel", ERRAND_SESSION_ID)).toBe(false);

    await act(async () => {
      pending.resolve();
      await request;
    });
    expect(result.current.isPending("send_result", ERRAND_SESSION_ID)).toBe(false);
  });

  it("rejects with the backend error and clears the pending state", async () => {
    api.cancelAllErrands.mockRejectedValue(new Error("feature_disabled"));
    const { result } = renderHook(() => useErrandActions(), { wrapper });

    await act(async () => {
      await expect(result.current.cancelAllErrands("task-1")).rejects.toThrow("feature_disabled");
    });
    expect(result.current.isPending("cancel_all", "task-1")).toBe(false);
  });
});
