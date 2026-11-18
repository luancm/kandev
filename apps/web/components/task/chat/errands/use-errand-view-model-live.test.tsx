import type { ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { sessionId, taskId } from "@/lib/types/http";
import { defaultState } from "@/lib/state/default-state";
import {
  makeBrowserErrandRow,
  makeInspection,
  ERRAND_TASK_ID,
  ERRAND_PARENT_ID,
} from "@/lib/state/slices/errands/errands.test-helpers";
import type { ErrandBrowserRow } from "@/lib/types/errands";
import { useErrandViewModel } from "./use-errand-view-model";

const api = vi.hoisted(() => ({ inspectErrand: vi.fn() }));
vi.mock("@/lib/api/domains/errand-api", () => api);

const ERRAND_ID = "errand-1";

afterEach(() => {
  cleanup();
  api.inspectErrand.mockReset();
});

function ownedInspection(overrides?: Partial<ErrandBrowserRow>) {
  return {
    ...makeInspection(makeBrowserErrandRow(overrides)),
    child_session_exists: false,
    owns_conversation: true,
  };
}

function renderModel() {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <StateProvider initialState={{ features: { ...defaultState.features, errands: true } }}>
      {children}
    </StateProvider>
  );
  const view = renderHook(
    () => ({ store: useAppStoreApi(), model: useErrandViewModel(ERRAND_ID, ERRAND_TASK_ID) }),
    { wrapper },
  );
  act(() => view.result.current.store.getState().setConnectionStatus("connected"));
  return view;
}

it("refreshes an owned inspection when an Errand update advances its revision", async () => {
  const running = ownedInspection({ status: "running", live: true });
  const completed = ownedInspection({ status: "done", tokens_out: 120 });
  api.inspectErrand.mockResolvedValueOnce(running).mockResolvedValueOnce(completed);
  const { result } = renderModel();
  await waitFor(() => expect(result.current.model.errand?.status).toBe("running"));
  act(() => result.current.store.getState().advanceErrandBrowserRevision(ERRAND_TASK_ID));
  await waitFor(() =>
    expect(result.current.model.errand).toMatchObject({ status: "done", tokens_out: 120 }),
  );
  expect(api.inspectErrand).toHaveBeenCalledTimes(2);
});

it.each(["revision", "reconnect"] as const)(
  "rejects an inspection superseded by a %s",
  async (change) => {
    let resolveOld!: (value: ReturnType<typeof ownedInspection>) => void;
    api.inspectErrand
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveOld = resolve;
          }),
      )
      .mockResolvedValueOnce(ownedInspection({ title: "Current audit" }));
    const { result } = renderModel();
    await waitFor(() => expect(api.inspectErrand).toHaveBeenCalledTimes(1));
    const oldSignal = api.inspectErrand.mock.calls[0][2] as AbortSignal;
    act(() => {
      const state = result.current.store.getState();
      if (change === "revision") {
        state.advanceErrandBrowserRevision(ERRAND_TASK_ID);
      } else {
        state.setConnectionStatus("disconnected");
        state.setConnectionStatus("connected");
      }
    });
    await waitFor(() => expect(result.current.model.title).toBe("Current audit"));
    expect(oldSignal.aborted).toBe(true);
    await act(async () => resolveOld(ownedInspection({ title: "Stale audit" })));
    expect(result.current.model.title).toBe("Current audit");
    expect(api.inspectErrand).toHaveBeenCalledTimes(2);
  },
);

it("retains the owned inspection until its view unmounts", async () => {
  api.inspectErrand.mockResolvedValueOnce(ownedInspection());
  const { result, unmount } = renderModel();
  const store = result.current.store;
  await waitFor(() => expect(result.current.model.title).toBe("API audit"));
  expect(store.getState().errands.detailBySessionId[ERRAND_ID]).toBeDefined();
  expect(store.getState().taskSessions.items[ERRAND_ID]).toBeUndefined();
  unmount();
  expect(store.getState().errands.detailOwnersBySessionId[ERRAND_ID]).toBeUndefined();
  expect(store.getState().errands.detailBySessionId[ERRAND_ID]).toBeUndefined();
});

it("updates workspace context from the ordinary parent without reading the Errand again", async () => {
  api.inspectErrand.mockResolvedValueOnce(ownedInspection());
  const { result } = renderModel();
  await waitFor(() => expect(result.current.model.title).toBe("API audit"));
  act(() =>
    result.current.store.getState().setTaskSession({
      id: sessionId(ERRAND_PARENT_ID),
      task_id: taskId(ERRAND_TASK_ID),
      started_at: "2026-10-26T10:00:00Z",
      updated_at: "2026-10-26T10:00:00Z",
      name: "Parent conversation",
      workspace_path: "/workspace",
      worktree_branch: "feature/audit",
      state: "COMPLETED",
    }),
  );
  expect(result.current.model).toMatchObject({
    parentTitle: "Parent conversation",
    workspacePath: "/workspace",
    branch: "feature/audit",
  });
  expect(api.inspectErrand).toHaveBeenCalledTimes(1);
});
