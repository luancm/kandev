import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { sessionId, taskId } from "@/lib/types/http";
import { defaultState } from "@/lib/state/default-state";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import {
  makeBrowserErrandRow,
  makeBrowserSummary,
  makeInspection,
  ERRAND_TASK_ID,
  ERRAND_PARENT_ID,
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

function renderModel({
  enabled = true,
  selectedSession = "errand-1" as string | null | undefined,
  taskHint = ERRAND_TASK_ID as string | null,
} = {}) {
  return renderHook(
    () => ({ store: useAppStoreApi(), model: useErrandViewModel(selectedSession, taskHint) }),
    {
      wrapper: ({ children }: { children: ReactNode }) =>
        createElement(StateProvider, {
          initialState: { features: { ...defaultState.features, errands: enabled } },
          children,
        }),
    },
  );
}

function addChild(store: ReturnType<typeof useAppStoreApi>) {
  store.getState().setTaskSession({
    id: sessionId("errand-1"),
    task_id: taskId(ERRAND_TASK_ID),
    started_at: timestamp,
    updated_at: timestamp,
    state: "COMPLETED",
    role: "errand",
    name: "Child conversation",
    parent_session_id: sessionId(ERRAND_PARENT_ID),
  });
}

function publishSummary(store: ReturnType<typeof useAppStoreApi>) {
  store.getState().setConnectionStatus("connected");
  const state = store.getState();
  const observation = {
    connectionGeneration: state.connection.generation ?? 0,
    readScopeIdentity: stateReadScopeIdentity(state),
    taskResetEpoch: state.errands.resetEpochByTaskId[ERRAND_TASK_ID] ?? 0,
    observationRevision: state.errands.browserRevisionByTaskId[ERRAND_TASK_ID] ?? 0,
  };
  const summary = makeBrowserSummary([
    makeBrowserErrandRow({
      title: "Summary child",
      status: "running",
      live: true,
      outstanding: true,
      settled_at: null,
    }),
  ]);
  state.beginErrandBrowserSummaryRead(ERRAND_TASK_ID, observation, "view-model-summary");
  state.setErrandBrowserSummary(ERRAND_TASK_ID, summary, observation, "view-model-summary");
  store.getState().setConnectionStatus("disconnected");
}

it.each([true, false])("keeps absent selections ordinary with errands=%s", (enabled) => {
  const { result, unmount } = renderModel({ enabled, selectedSession: null });
  expect(result.current.model).toBeNull();
  expect(result.current.store.getState().errands.detailOwnersBySessionId).toEqual({});
  expect(api.inspectErrand).not.toHaveBeenCalled();
  unmount();
});

it.each([true, false])(
  "recognizes a late role-only child and prefers its task with errands=%s",
  (enabled) => {
    const { result } = renderModel({ enabled, taskHint: "other-task" });
    expect(result.current.model).toBeNull();
    act(() => {
      result.current.store.getState().setTaskSession({
        id: sessionId(ERRAND_PARENT_ID),
        task_id: taskId(ERRAND_TASK_ID),
        started_at: timestamp,
        updated_at: timestamp,
        state: "COMPLETED",
        name: "Parent conversation",
      });
      addChild(result.current.store);
    });
    expect(result.current.model).toMatchObject({
      taskId: ERRAND_TASK_ID,
      title: "Child conversation",
      errand: null,
      parentSessionId: ERRAND_PARENT_ID,
      parentTitle: "Parent conversation",
    });
    expect(api.inspectErrand).not.toHaveBeenCalled();
  },
);

it("recognizes a late browser-only child only while enabled without changing props", () => {
  const { result } = renderModel({ enabled: false });
  act(() => publishSummary(result.current.store));
  expect(result.current.model).toBeNull();
  act(() =>
    result.current.store.getState().setFeatures({ ...defaultState.features, errands: true }),
  );
  expect(result.current.model?.title).toBe("Summary child");
  expect(
    Object.keys(result.current.store.getState().errands.detailOwnersBySessionId["errand-1"]),
  ).toHaveLength(1);
  act(() =>
    result.current.store.getState().setFeatures({ ...defaultState.features, errands: false }),
  );
  expect(result.current.model).toBeNull();
  expect(
    result.current.store.getState().errands.detailOwnersBySessionId["errand-1"],
  ).toBeUndefined();
  expect(api.inspectErrand).not.toHaveBeenCalled();
});

it.each([true, false])(
  "recognizes scoped inspection identity and drops it on scope change with errands=%s",
  (enabled) => {
    const { result } = renderModel({ enabled });
    act(() => result.current.store.getState().retainErrandInspection("opener", makeInspection()));
    expect(result.current.model?.taskId).toBe(ERRAND_TASK_ID);
    act(() => result.current.store.setState({ workspaceContextGeneration: 1 }));
    expect(result.current.model).toBeNull();
    expect(api.inspectErrand).not.toHaveBeenCalled();
  },
);

it("retains a recognized inspection without task presentation data until unmount", () => {
  const { result, unmount } = renderModel({ enabled: false, taskHint: null });
  const store = result.current.store;
  act(() => store.getState().retainErrandInspection("opener", makeInspection()));
  expect(result.current.model).toBeNull();
  expect(Object.keys(store.getState().errands.detailOwnersBySessionId["errand-1"])).toHaveLength(2);
  act(() => store.getState().releaseErrandInspection("opener"));
  expect(store.getState().errands.detailBySessionId["errand-1"]).toBeDefined();
  unmount();
  expect(store.getState().errands.detailBySessionId["errand-1"]).toBeUndefined();
  expect(store.getState().errands.detailOwnersBySessionId["errand-1"]).toBeUndefined();
});

it("updates child and parent titles without reacquiring inspection ownership", async () => {
  api.inspectErrand.mockResolvedValue(makeInspection());
  const { result } = renderModel();
  const store = result.current.store;
  const retain = vi.fn(store.getState().retainErrandInspection);
  const release = vi.fn(store.getState().releaseErrandInspection);
  act(() => {
    store.setState({ retainErrandInspection: retain, releaseErrandInspection: release });
    store.getState().setTaskSession({
      id: sessionId(ERRAND_PARENT_ID),
      task_id: taskId(ERRAND_TASK_ID),
      started_at: timestamp,
      updated_at: timestamp,
      state: "COMPLETED",
      name: "Parent conversation",
    });
    addChild(store);
    store.getState().setConnectionStatus("connected");
  });
  await waitFor(() => expect(result.current.model?.title).toBe("API audit"));
  const owner = retain.mock.calls[0][0];
  const owners = Object.keys(store.getState().errands.detailOwnersBySessionId["errand-1"]);
  act(() => {
    store
      .getState()
      .retainErrandInspection(
        owner,
        makeInspection(makeBrowserErrandRow({ title: "Retitled child" })),
      );
    store.getState().setTaskSession({
      ...store.getState().taskSessions.items[ERRAND_PARENT_ID],
      name: "Retitled parent",
    });
    retain.mockClear();
    release.mockClear();
  });
  expect(result.current.model).toMatchObject({
    title: "Retitled child",
    parentTitle: "Retitled parent",
  });
  expect(retain).not.toHaveBeenCalled();
  expect(release).not.toHaveBeenCalled();
  expect(Object.keys(store.getState().errands.detailOwnersBySessionId["errand-1"])).toEqual(owners);
  expect(api.inspectErrand).toHaveBeenCalledTimes(1);
});

it("refreshes pending inspection across a connection generation and rejects its old response", async () => {
  let resolveOld!: (value: ReturnType<typeof makeInspection>) => void;
  api.inspectErrand
    .mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveOld = resolve;
        }),
    )
    .mockResolvedValue(makeInspection(makeBrowserErrandRow({ title: "After reconnect" })));
  const { result } = renderModel();
  act(() => {
    addChild(result.current.store);
    result.current.store.getState().setConnectionStatus("connected");
  });
  await waitFor(() => expect(api.inspectErrand).toHaveBeenCalledTimes(1));
  const oldSignal = api.inspectErrand.mock.calls[0][2] as AbortSignal;
  act(() => result.current.store.getState().setConnectionStatus("disconnected"));
  expect(oldSignal.aborted).toBe(true);
  act(() => result.current.store.getState().setConnectionStatus("connected"));
  await waitFor(() => expect(result.current.model?.title).toBe("After reconnect"));
  await act(async () =>
    resolveOld(makeInspection(makeBrowserErrandRow({ title: "Old connection" }))),
  );
  expect(result.current.model?.title).toBe("After reconnect");
  expect(api.inspectErrand).toHaveBeenCalledTimes(2);
});
