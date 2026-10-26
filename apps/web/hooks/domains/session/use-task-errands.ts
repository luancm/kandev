import { useEffect, useMemo, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { listErrands } from "@/lib/api/domains/errand-api";
import {
  selectErrandsByParent,
  selectLiveErrandCount,
  selectTaskErrands,
} from "@/lib/state/slices/errands/selectors";
import type { Errand } from "@/lib/types/errands";

type AppStore = ReturnType<typeof useAppStoreApi>;
const loadsByStore = new WeakMap<AppStore, Map<string, Promise<void>>>();

function storeLoads(store: AppStore) {
  let loads = loadsByStore.get(store);
  if (!loads) {
    loads = new Map();
    loadsByStore.set(store, loads);
  }
  return loads;
}

/** Applies `errand.list` for a task, sharing one in-flight request across consumers. */
function loadTaskErrands(store: AppStore, taskId: string, force: boolean): Promise<void> {
  const state = store.getState();
  if (!force && state.errands.loadedByTaskId[taskId]) return Promise.resolve();
  const loads = storeLoads(store);
  const current = loads.get(taskId);
  if (current) return current;

  const requestRevision = state.errands.liveRevisionByTaskId[taskId] ?? 0;
  const request = listErrands(taskId)
    .then((errands) => store.getState().setTaskErrands(taskId, errands, requestRevision))
    .catch(() => {
      // Live errand.updated events still populate the store; the next connect retries.
    })
    .finally(() => loads.delete(taskId));
  loads.set(taskId, request);
  return request;
}

export type TaskErrands = {
  /** The task's errands in creation order. */
  errands: Errand[];
  /** The same errands grouped by parent session ID. */
  errandsByParent: Record<string, Errand[]>;
  /** Queued, running, and needs-attention errands. */
  liveCount: number;
  /** Whether an `errand.list` snapshot has been applied for the task. */
  isLoaded: boolean;
};

/**
 * A task's errands. While the `errands` flag is on, loads `errand.list` once
 * per task and connection (live rows arrive through `errand.updated`); while
 * it is off, reports no errands and sends nothing.
 */
export function useTaskErrands(taskId: string | null | undefined): TaskErrands {
  const store = useAppStoreApi();
  const enabled = useFeature("errands");
  const scopedTaskId = enabled ? taskId : null;
  const errands = useAppStore((state) => selectTaskErrands(state, scopedTaskId));
  const errandsByParent = useAppStore((state) => selectErrandsByParent(state, scopedTaskId));
  const liveCount = useAppStore((state) => selectLiveErrandCount(state, scopedTaskId));
  const isLoaded = useAppStore((state) =>
    scopedTaskId ? Boolean(state.errands.loadedByTaskId[scopedTaskId]) : false,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const previousConnection = useRef(connectionStatus);

  useEffect(() => {
    const previous = previousConnection.current;
    previousConnection.current = connectionStatus;
    if (!scopedTaskId || connectionStatus !== "connected") return;
    void loadTaskErrands(store, scopedTaskId, previous !== "connected");
  }, [connectionStatus, store, scopedTaskId]);

  return useMemo(
    () => ({ errands, errandsByParent, liveCount, isLoaded }),
    [errands, errandsByParent, liveCount, isLoaded],
  );
}
