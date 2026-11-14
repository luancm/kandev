import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { sessionId, taskId } from "@/lib/types/http";
import { defaultState } from "@/lib/state/default-state";
import {
  makeBrowserErrandRow,
  makeInspection,
  ERRAND_TASK_ID,
} from "@/lib/state/slices/errands/errands.test-helpers";
const api = vi.hoisted(() => ({ inspectErrand: vi.fn() }));
vi.mock("@/lib/api/domains/errand-api", () => api);
import { useErrandViewModel } from "./use-errand-view-model";

afterEach(() => {
  cleanup();
  api.inspectErrand.mockReset();
});
const timestamp = "2026-10-26T10:00:00Z";

function Wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, {
    initialState: { features: { ...defaultState.features, errands: true } },
    children,
  });
}

it("refreshes an owned inspection on session-only changes", async () => {
  const running = makeInspection(makeBrowserErrandRow({ status: "running", live: true }));
  const completed = makeInspection(
    makeBrowserErrandRow({ status: "done", live: false, tokens_out: 120 }),
  );
  api.inspectErrand.mockResolvedValueOnce(running).mockResolvedValue(completed);
  const { result } = renderHook(
    () => ({
      store: useAppStoreApi(),
      model: useErrandViewModel("errand-1", ERRAND_TASK_ID),
    }),
    { wrapper: Wrapper },
  );
  act(() => {
    result.current.store.getState().setTaskSession({
      id: sessionId("errand-1"),
      task_id: taskId(ERRAND_TASK_ID),
      started_at: timestamp,
      updated_at: timestamp,
      role: "errand",
      state: "RUNNING",
    });
    result.current.store.getState().setConnectionStatus("connected");
  });
  await waitFor(() => expect(result.current.model?.errand?.status).toBe("running"));
  act(() =>
    result.current.store.getState().setTaskSession({
      id: sessionId("errand-1"),
      task_id: taskId(ERRAND_TASK_ID),
      started_at: timestamp,
      updated_at: timestamp,
      role: "errand",
      state: "COMPLETED",
    }),
  );
  await waitFor(() =>
    expect(result.current.model?.errand).toMatchObject({
      status: "done",
      live: false,
      tokens_out: 120,
    }),
  );
  expect(api.inspectErrand).toHaveBeenCalledTimes(2);
});

it("discards a delayed inspection after a newer session observation", async () => {
  const running = makeInspection(makeBrowserErrandRow({ status: "running", live: true }));
  const completed = makeInspection(makeBrowserErrandRow({ status: "done", live: false }));
  let resolveOld!: (value: typeof running) => void;
  api.inspectErrand
    .mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveOld = resolve;
        }),
    )
    .mockResolvedValue(completed);
  const { result } = renderHook(
    () => ({ store: useAppStoreApi(), model: useErrandViewModel("errand-1", ERRAND_TASK_ID) }),
    { wrapper: Wrapper },
  );
  const session = {
    id: sessionId("errand-1"),
    task_id: taskId(ERRAND_TASK_ID),
    started_at: timestamp,
    updated_at: timestamp,
    role: "errand" as const,
  };
  act(() => {
    result.current.store.getState().setTaskSession({ ...session, state: "RUNNING" });
    result.current.store.getState().setConnectionStatus("connected");
  });
  await waitFor(() => expect(api.inspectErrand).toHaveBeenCalledTimes(1));
  act(() => result.current.store.getState().setTaskSession({ ...session, state: "COMPLETED" }));
  await waitFor(() => expect(result.current.model?.errand?.status).toBe("done"));
  await act(async () => {
    resolveOld(running);
  });
  expect(result.current.model?.errand?.status).toBe("done");
  expect(api.inspectErrand).toHaveBeenCalledTimes(2);
});

it("holds the child cache while inspection is pending and releases it on unmount", async () => {
  let resolve!: (value: ReturnType<typeof makeInspection>) => void;
  api.inspectErrand.mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const { result, unmount } = renderHook(
    () => ({ store: useAppStoreApi(), model: useErrandViewModel("errand-1", ERRAND_TASK_ID) }),
    { wrapper: Wrapper },
  );
  const store = result.current.store;
  act(() => {
    store.getState().retainErrandDetail("opener", "errand-1");
    store.getState().setTaskSession({
      id: sessionId("errand-1"),
      task_id: taskId(ERRAND_TASK_ID),
      started_at: timestamp,
      updated_at: timestamp,
      role: "errand",
      state: "COMPLETED",
    });
    store.getState().setConnectionStatus("connected");
  });
  await waitFor(() => expect(api.inspectErrand).toHaveBeenCalledTimes(1));
  act(() => store.getState().releaseErrandInspection("opener"));
  expect(store.getState().taskSessions.items["errand-1"]).toBeDefined();
  unmount();
  expect(store.getState().taskSessions.items["errand-1"]).toBeUndefined();
  await act(async () => resolve(makeInspection()));
  expect(store.getState().errands.detailBySessionId["errand-1"]).toBeUndefined();
  expect(store.getState().taskSessions.items["errand-1"]).toBeUndefined();
});
