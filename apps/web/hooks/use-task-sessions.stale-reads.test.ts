import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { createAppStore, type AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/ids";

const apiMock = vi.hoisted(() => ({
  listTaskSessions: vi.fn(),
}));
const storeProviderRef = vi.hoisted(() => ({ current: null as unknown }));

type MockTaskSessionsState = {
  taskSessions: {
    activityEpochBySession: Record<string, number>;
    readCursorEpochBySession: Record<string, number>;
  };
  taskSessionsByTask: {
    itemsByTaskId: Record<string, TaskSession[]>;
    loadingByTaskId: Record<string, boolean>;
    loadedByTaskId: Record<string, boolean>;
    errorByTaskId: Record<string, string | null>;
  };
  connection: { status: string; generation?: number };
  auth: {
    mode: string;
    authenticated: boolean;
    user: { id: string } | null;
  };
  workspaceContextGeneration: number;
  setTaskSessionsForTask: ReturnType<typeof vi.fn>;
  setTaskSessionsLoading: ReturnType<typeof vi.fn>;
  setTaskSessionsError: ReturnType<typeof vi.fn>;
};

let mockState: MockTaskSessionsState;

vi.mock("@/components/state-provider", async () => {
  const { useStore } = await import("zustand");
  const getState = () => mockState;
  const storeApi = { getState };
  const currentStore = () => storeProviderRef.current as StoreApi<MockTaskSessionsState> | null;
  return {
    useAppStore: (selector: (state: MockTaskSessionsState) => unknown) => {
      const store = currentStore();
      return store ? useStore(store, selector) : selector(mockState);
    },
    useAppStoreApi: () => currentStore() ?? storeApi,
  };
});

vi.mock("@/lib/api", () => apiMock);

import { useTaskSessions } from "./use-task-sessions";

const TASK_ID = taskId("task-1");
const SERVICE_UNAVAILABLE = "service unavailable";

function session(id: string, state: TaskSession["state"] = "RUNNING"): TaskSession {
  return {
    id: sessionId(id),
    task_id: TASK_ID,
    state,
    started_at: "2026-06-27T00:00:00Z",
    updated_at: "2026-06-27T00:00:00Z",
  };
}

function setDocumentVisibility(value: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    value,
  });
}

function resetMockState() {
  mockState = {
    taskSessions: { activityEpochBySession: {}, readCursorEpochBySession: {} },
    taskSessionsByTask: {
      itemsByTaskId: {},
      loadingByTaskId: {},
      loadedByTaskId: {},
      errorByTaskId: {},
    },
    connection: { status: "connected", generation: 0 },
    auth: { mode: "optional", authenticated: false, user: null },
    workspaceContextGeneration: 0,
    setTaskSessionsForTask: vi.fn(),
    setTaskSessionsLoading: vi.fn(),
    setTaskSessionsError: vi.fn(),
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((innerResolve, innerReject) => {
    resolve = innerResolve;
    reject = innerReject;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  apiMock.listTaskSessions.mockReset();
  resetMockState();
  setDocumentVisibility("visible");
  apiMock.listTaskSessions.mockResolvedValue({ sessions: [session("sess-1")] });
});

afterEach(() => {
  cleanup();
  storeProviderRef.current = null;
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe("useTaskSessions read retirement", () => {
  it.each(["user", "workspace"])(
    "does not merge a retired %s response or settle its successor",
    async (change) => {
      const oldResponse = deferred<{ sessions: TaskSession[] }>();
      const newResponse = deferred<{ sessions: TaskSession[] }>();
      apiMock.listTaskSessions
        .mockReturnValueOnce(oldResponse.promise)
        .mockReturnValueOnce(newResponse.promise);
      const oldHook = renderHook(() => useTaskSessions(TASK_ID));
      await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1));
      if (change === "user") mockState.auth.user = { id: "new-user" };
      else mockState.workspaceContextGeneration++;
      oldHook.unmount();
      const newHook = renderHook(() => useTaskSessions(TASK_ID));
      await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(2));
      mockState.setTaskSessionsLoading.mockClear();
      await act(async () => oldResponse.resolve({ sessions: [session("obsolete")] }));
      expect(mockState.setTaskSessionsForTask).not.toHaveBeenCalled();
      expect(mockState.setTaskSessionsError).not.toHaveBeenCalled();
      expect(mockState.setTaskSessionsLoading).not.toHaveBeenCalledWith(TASK_ID, false);
      await act(async () => newResponse.resolve({ sessions: [session("successor")] }));
      expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
        TASK_ID,
        [session("successor")],
        {},
      );
      newHook.unmount();
    },
  );

  it("releases retired loading ownership so a still-mounted hook can start in the new scope", async () => {
    const oldResponse = deferred<{ sessions: TaskSession[] }>();
    const newResponse = deferred<{ sessions: TaskSession[] }>();
    apiMock.listTaskSessions
      .mockReturnValueOnce(oldResponse.promise)
      .mockReturnValueOnce(newResponse.promise);
    mockState.setTaskSessionsLoading.mockImplementation((id: string, loading: boolean) => {
      mockState.taskSessionsByTask.loadingByTaskId[id] = loading;
    });
    const hook = renderHook(() => useTaskSessions(TASK_ID));
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1));
    expect(mockState.taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(true);
    mockState.workspaceContextGeneration++;
    hook.rerender();
    expect(mockState.taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(false);
    hook.rerender();
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(2));
    mockState.setTaskSessionsLoading.mockClear();
    await act(async () => oldResponse.resolve({ sessions: [session("retired")] }));
    expect(mockState.setTaskSessionsForTask).not.toHaveBeenCalled();
    expect(mockState.taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(true);
    expect(mockState.setTaskSessionsLoading).not.toHaveBeenCalled();
    await act(async () => newResponse.resolve({ sessions: [session("successor")] }));
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("successor")],
      {},
    );
    expect(mockState.taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(false);
    hook.unmount();
  });

  it("does not commit a response after the final consumer unmounts", async () => {
    const response = deferred<{ sessions: TaskSession[] }>();
    apiMock.listTaskSessions.mockReturnValueOnce(response.promise);
    const hook = renderHook(() => useTaskSessions(TASK_ID));
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1));
    hook.unmount();
    mockState.setTaskSessionsLoading.mockClear();
    await act(async () => response.resolve({ sessions: [session("obsolete")] }));
    expect(mockState.setTaskSessionsForTask).not.toHaveBeenCalled();
    expect(mockState.setTaskSessionsError).not.toHaveBeenCalled();
    expect(mockState.setTaskSessionsLoading).not.toHaveBeenCalled();
  });
});

describe("useTaskSessions retained and successor consumers", () => {
  it("does not publish a retired transport error or settle a successor read", async () => {
    const oldResponse = deferred<{ sessions: TaskSession[] }>();
    const newResponse = deferred<{ sessions: TaskSession[] }>();
    apiMock.listTaskSessions
      .mockReturnValueOnce(oldResponse.promise)
      .mockReturnValueOnce(newResponse.promise);
    const oldHook = renderHook(() => useTaskSessions(TASK_ID));
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1));
    mockState.workspaceContextGeneration++;
    oldHook.unmount();
    const successor = renderHook(() => useTaskSessions(TASK_ID));
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(2));
    mockState.setTaskSessionsLoading.mockClear();
    await act(async () => oldResponse.reject(new Error(SERVICE_UNAVAILABLE)));
    expect(mockState.setTaskSessionsError).not.toHaveBeenCalled();
    expect(mockState.setTaskSessionsLoading).not.toHaveBeenCalled();
    await act(async () => newResponse.resolve({ sessions: [session("successor")] }));
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("successor")],
      {},
    );
    successor.unmount();
  });

  it.each(["success", "failure"])(
    "settles a retained load after the initiator leaves (%s)",
    async (outcome) => {
      const response = deferred<{ sessions: TaskSession[] }>();
      apiMock.listTaskSessions.mockReturnValueOnce(response.promise);
      mockState.setTaskSessionsLoading.mockImplementation((id: string, loading: boolean) => {
        mockState.taskSessionsByTask.loadingByTaskId[id] = loading;
      });
      mockState.setTaskSessionsForTask.mockImplementation((id: string) => {
        mockState.taskSessionsByTask.loadedByTaskId[id] = true;
      });
      mockState.setTaskSessionsError.mockImplementation((id: string, error: string) => {
        mockState.taskSessionsByTask.errorByTaskId[id] = error;
      });
      const first = renderHook(() => useTaskSessions(TASK_ID));
      await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1));
      const survivor = renderHook(() => useTaskSessions(TASK_ID));
      first.unmount();
      expect(mockState.taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(true);
      await act(async () => {
        if (outcome === "success") response.resolve({ sessions: [session("retained")] });
        else response.reject(new Error(SERVICE_UNAVAILABLE));
      });
      expect(mockState.taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(false);
      if (outcome === "success") {
        expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
          TASK_ID,
          [session("retained")],
          {},
        );
      } else {
        expect(mockState.setTaskSessionsError).toHaveBeenCalledWith(TASK_ID, SERVICE_UNAVAILABLE);
      }
      survivor.rerender();
      await act(async () => survivor.result.current.loadSessions(true));
      expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(2);
      survivor.unmount();
    },
  );

  it("keeps the shared read live after only one consumer unmounts", async () => {
    const response = deferred<{ sessions: TaskSession[] }>();
    apiMock.listTaskSessions.mockReturnValueOnce(response.promise);
    mockState.setTaskSessionsLoading.mockImplementation((id: string, loading: boolean) => {
      mockState.taskSessionsByTask.loadingByTaskId[id] = loading;
    });
    const first = renderHook(() => useTaskSessions(TASK_ID));
    const second = renderHook(() => useTaskSessions(TASK_ID));
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1));
    first.unmount();
    await act(async () => response.resolve({ sessions: [session("retained")] }));
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledTimes(1);
    expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(
      TASK_ID,
      [session("retained")],
      {},
    );
    second.unmount();
  });
});

function contextSession(id: string, used: number | null, updatedAt: string): TaskSession {
  return {
    ...session(id, "WAITING_FOR_INPUT"),
    updated_at: updatedAt,
    metadata: {
      context_window:
        used === null
          ? null
          : {
              size: 100_000,
              used,
              remaining: 100_000 - used,
              efficiency: (used / 100_000) * 100,
              source: "acp",
            },
    },
  };
}

function createReadOwnerStore(): StoreApi<AppState> {
  const store = createAppStore();
  storeProviderRef.current = store as unknown as StoreApi<MockTaskSessionsState>;
  store.getState().setConnectionStatus("connected");
  store
    .getState()
    .setTaskSessionsForTask(
      TASK_ID,
      [contextSession("child-1", 68_000, "2026-06-27T00:00:00Z")],
      {},
    );
  return store;
}

describe("useTaskSessions connection-generation ownership", () => {
  it("refreshes a batched reconnect and fences the old list after the replacement settles", async () => {
    const store = createReadOwnerStore();
    const oldResponse = deferred<{ sessions: TaskSession[] }>();
    const newResponse = deferred<{ sessions: TaskSession[] }>();
    const replacementResponse = deferred<{ sessions: TaskSession[] }>();
    apiMock.listTaskSessions
      .mockReturnValueOnce(oldResponse.promise)
      .mockReturnValueOnce(newResponse.promise)
      .mockReturnValueOnce(replacementResponse.promise);
    const { result, unmount } = renderHook(() => useTaskSessions(TASK_ID));

    let oldLoad!: Promise<void>;
    act(() => {
      oldLoad = result.current.loadSessions(true);
    });
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1));

    const generationBeforeReconnect = store.getState().connection.generation;
    act(() => {
      store.getState().setConnectionStatus("disconnected");
      store.getState().setConnectionStatus("connected");
    });

    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(2));
    expect(store.getState().connection.generation).toBe(generationBeforeReconnect + 2);
    expect(store.getState().taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(true);

    const fresh = contextSession("child-1", 82_000, "2026-06-27T00:01:00Z");
    await act(async () => {
      newResponse.resolve({ sessions: [fresh] });
    });
    await waitFor(() =>
      expect(store.getState().contextWindow.bySessionId[fresh.id]?.used).toBe(82_000),
    );
    expect(store.getState().taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(false);

    let replacementLoad!: Promise<void>;
    act(() => {
      replacementLoad = result.current.loadSessions(true);
    });
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(3));
    expect(store.getState().taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(true);

    await act(async () => {
      oldResponse.resolve({
        sessions: [contextSession("child-1", null, "2026-06-27T00:00:30Z")],
      });
      await oldLoad;
    });
    expect(store.getState().taskSessions.items["child-1"]?.metadata?.context_window).toMatchObject({
      used: 82_000,
    });
    expect(store.getState().contextWindow.bySessionId["child-1"]?.used).toBe(82_000);
    expect(store.getState().taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(true);

    await act(async () => {
      replacementResponse.resolve({ sessions: [fresh] });
      await replacementLoad;
    });
    expect(store.getState().taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(false);
    unmount();
  });

  it("accepts a pending same-session response after an unrelated state rerender", async () => {
    const store = createReadOwnerStore();
    const response = deferred<{ sessions: TaskSession[] }>();
    apiMock.listTaskSessions.mockReturnValueOnce(response.promise);
    const { result, unmount } = renderHook(() => useTaskSessions(TASK_ID));

    let load!: Promise<void>;
    act(() => {
      load = result.current.loadSessions(true);
    });
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1));
    act(() => store.getState().setRightPanelActiveTab("child-1", "files"));

    await act(async () => {
      response.resolve({
        sessions: [contextSession("child-1", 82_000, "2026-06-27T00:01:00Z")],
      });
      await load;
    });

    expect(store.getState().contextWindow.bySessionId["child-1"]?.used).toBe(82_000);
    expect(store.getState().taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(false);
    unmount();
  });

  it("keeps a shared read live when its initiating hook releases one consumer", async () => {
    const store = createReadOwnerStore();
    const response = deferred<{ sessions: TaskSession[] }>();
    apiMock.listTaskSessions.mockReturnValueOnce(response.promise);
    const first = renderHook(() => useTaskSessions(TASK_ID));
    const second = renderHook(() => useTaskSessions(TASK_ID));

    let load!: Promise<void>;
    act(() => {
      load = first.result.current.loadSessions(true);
    });
    await waitFor(() => expect(apiMock.listTaskSessions).toHaveBeenCalledTimes(1));
    first.unmount();

    await act(async () => {
      response.resolve({
        sessions: [contextSession("child-1", 82_000, "2026-06-27T00:01:00Z")],
      });
      await load;
    });

    expect(store.getState().contextWindow.bySessionId["child-1"]?.used).toBe(82_000);
    expect(store.getState().taskSessionsByTask.loadingByTaskId[TASK_ID]).toBe(false);
    second.unmount();
  });
});
