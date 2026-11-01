import { useEffect, useMemo } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { listErrands } from "@/lib/api/domains/errand-api";
import {
  selectErrandsByParent,
  selectLiveErrandCount,
  selectTaskErrands,
} from "@/lib/state/slices/errands/selectors";
import { stateReadScopeIdentity, storeReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import type { ErrandsListObservation } from "@/lib/state/slices/errands/types";
import type { Errand } from "@/lib/types/errands";

type AppStore = ReturnType<typeof useAppStoreApi>;
type LoadAttempt = Readonly<
  ErrandsListObservation & {
    promise: Promise<void>;
    requestRevision: number;
  }
>;
const loadsByStore = new WeakMap<AppStore, Map<string, LoadAttempt>>();

function storeLoads(store: AppStore) {
  let loads = loadsByStore.get(store);
  if (!loads) {
    loads = new Map();
    loadsByStore.set(store, loads);
  }
  return loads;
}

/** Applies `errand.list` for a task, sharing one in-flight request across consumers. */
function loadTaskErrands(store: AppStore, taskId: string): Promise<void> {
  const state = store.getState();
  const connectionGeneration = state.connection.generation ?? 0;
  const readScopeIdentity = storeReadScopeIdentity(store);
  const taskResetEpoch = state.errands.resetEpochByTaskId?.[taskId] ?? 0;
  if (
    state.errands.loadedByTaskId[taskId] &&
    state.errands.loadedConnectionGenerationByTaskId?.[taskId] === connectionGeneration &&
    state.errands.loadedReadScopeByTaskId?.[taskId] === readScopeIdentity
  )
    return Promise.resolve();
  const loads = storeLoads(store);
  const current = loads.get(taskId);
  if (
    current &&
    current.connectionGeneration === connectionGeneration &&
    current.readScopeIdentity === readScopeIdentity &&
    current.taskResetEpoch === taskResetEpoch
  ) {
    return current.promise;
  }

  const requestRevision = state.errands.liveRevisionByTaskId[taskId] ?? 0;
  const promise = listErrands(taskId)
    .then((errands) => {
      if (loads.get(taskId) !== attempt) return;
      store.getState().setTaskErrands(taskId, errands, attempt.requestRevision, attempt);
    })
    .catch(() => {
      // Keep cached rows; the next mount or connection retries the failed snapshot.
    })
    .finally(() => {
      if (loads.get(taskId) === attempt) loads.delete(taskId);
    });
  const attempt: LoadAttempt = {
    connectionGeneration,
    readScopeIdentity,
    taskResetEpoch,
    requestRevision,
    promise,
  };
  loads.set(taskId, attempt);
  return attempt.promise;
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
  const connectionGeneration = useAppStore((state) => state.connection.generation ?? 0);
  const readScopeIdentity = useAppStore(stateReadScopeIdentity);

  useEffect(() => {
    if (!scopedTaskId || connectionStatus !== "connected") return;
    void loadTaskErrands(store, scopedTaskId);
  }, [connectionStatus, connectionGeneration, readScopeIdentity, store, scopedTaskId]);

  return useMemo(
    () => ({ errands, errandsByParent, liveCount, isLoaded }),
    [errands, errandsByParent, liveCount, isLoaded],
  );
}
