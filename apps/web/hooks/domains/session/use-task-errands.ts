import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { getErrandBrowserSummary, getErrandHistoryPage } from "@/lib/api/domains/errand-api";
import {
  selectErrandBrowserHistory,
  selectErrandBrowserHistoryRows,
  selectErrandBrowserRows,
  selectErrandBrowserSummary,
} from "@/lib/state/slices/errands/selectors";
import { stateReadScopeIdentity, storeReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import {
  errandBrowserReadCoordinator,
  type ErrandBrowserReadKey,
  type ErrandBrowserReadLease,
} from "@/lib/state/slices/errands/errand-browser-read-coordinator";
import type { ErrandBrowserReadObservation } from "@/lib/state/slices/errands/types";
import type {
  ErrandBrowserCounts,
  ErrandBrowserRow,
  ErrandBrowserSummary,
} from "@/lib/types/errands";

const HISTORY_PAGE_SIZE = 25;
const EMPTY_ROWS: ErrandBrowserRow[] = [];
type AppStore = ReturnType<typeof useAppStoreApi>;

function observationFor(store: AppStore, taskId: string): ErrandBrowserReadObservation {
  const state = store.getState();
  return {
    connectionGeneration: state.connection.generation ?? 0,
    readScopeIdentity: storeReadScopeIdentity(store),
    taskResetEpoch: state.errands.resetEpochByTaskId?.[taskId] ?? 0,
    observationRevision: state.errands.browserRevisionByTaskId?.[taskId] ?? 0,
  };
}

function browserReadKey(
  kind: ErrandBrowserReadKey["kind"],
  taskId: string,
  observation: ErrandBrowserReadObservation,
  page?: { parentSessionId: string; cursor: string | null },
): ErrandBrowserReadKey {
  return {
    kind,
    taskId,
    readScopeIdentity: observation.readScopeIdentity,
    connectionGeneration: observation.connectionGeneration,
    taskResetEpoch: observation.taskResetEpoch,
    ...(page
      ? {
          parentSessionId: page.parentSessionId,
          cursor: page.cursor,
          limit: HISTORY_PAGE_SIZE,
        }
      : {}),
  };
}

function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === "AbortError";
}

function emptyCounts(): ErrandBrowserCounts {
  return {
    live: 0,
    capacity: 0,
    queued: 0,
    outstanding: 0,
    finished: 0,
    pending_results: 0,
    unread: 0,
  };
}

export type TaskErrands = {
  outstanding: ErrandBrowserRow[];
  counts: ErrandBrowserCounts;
  parentCounts: ErrandBrowserCounts;
  summary: ErrandBrowserSummary | undefined;
  summaryStatus: "idle" | "loading" | "loaded" | "error";
  summaryError: unknown;
  isSummaryLoaded: boolean;
  history: ReturnType<typeof selectErrandBrowserHistory>;
  historyRows: ErrandBrowserRow[];
  openHistory: () => void;
  closeHistory: () => void;
  loadMoreHistory: () => void;
  retryHistory: () => void;
  refreshHistory: () => void;
  startHistoryAgain: () => void;
  retrySummary: () => void;
};

function useErrandSummaryRead(args: {
  store: AppStore;
  taskId: string | null;
  browserRevision: number;
  connectionStatus: string;
  connectionGeneration: number;
  readScopeIdentity: string;
  retryRevision: number;
  taskResetEpoch: number;
}) {
  const {
    store,
    taskId,
    browserRevision,
    connectionStatus,
    connectionGeneration,
    readScopeIdentity,
    retryRevision,
    taskResetEpoch,
  } = args;
  useEffect(() => {
    if (!taskId || connectionStatus !== "connected") return;
    const observation = observationFor(store, taskId);
    const read = store.getState().errands.browserSummaryReadByTaskId[taskId];
    if (
      read?.status === "loaded" &&
      read.connectionGeneration === observation.connectionGeneration &&
      read.readScopeIdentity === observation.readScopeIdentity &&
      read.taskResetEpoch === observation.taskResetEpoch &&
      read.observationRevision === observation.observationRevision
    ) {
      return;
    }

    const lease = errandBrowserReadCoordinator(store).acquire(
      browserReadKey("summary", taskId, observation),
      observation.observationRevision,
      (signal) => getErrandBrowserSummary(taskId, signal),
    );
    store.getState().beginErrandBrowserSummaryRead(taskId, observation, lease.attemptId);
    void lease.promise
      .then((result) => {
        store.getState().setErrandBrowserSummary(taskId, result, observation, lease.attemptId);
      })
      .catch((error: unknown) => {
        if (isAbortError(error)) return;
        store.getState().failErrandBrowserSummaryRead(taskId, observation, lease.attemptId, error);
      })
      .finally(lease.release);
    return lease.release;
  }, [
    browserRevision,
    connectionGeneration,
    connectionStatus,
    readScopeIdentity,
    retryRevision,
    store,
    taskId,
    taskResetEpoch,
  ]);
}

function useErrandConnectionGeneration(
  store: AppStore,
  taskId: string | null,
  connectionGeneration: number,
) {
  const previousConnectionGeneration = useRef<number | null>(null);
  useEffect(() => {
    if (!taskId) return;
    const previous = previousConnectionGeneration.current;
    previousConnectionGeneration.current = connectionGeneration;
    if (previous !== null && previous !== connectionGeneration) {
      store.getState().markErrandHistoryStale(taskId);
    }
  }, [connectionGeneration, store, taskId]);
}

function useErrandHistoryPageReads(args: {
  store: AppStore;
  taskId: string | null;
  parentSessionId: string | null;
  ownerId: string | undefined;
}) {
  const { store, taskId, parentSessionId, ownerId } = args;
  const readScopeIdentity = useAppStore(stateReadScopeIdentity);
  const leases = useRef(new Map<string, ErrandBrowserReadLease<unknown>>());
  const requestHistoryPage = useCallback(
    (cursor: string | null) => {
      if (!taskId || !parentSessionId || !ownerId) return;
      const activeHistory = store.getState().errands.historyByOwnerId[ownerId];
      if (activeHistory?.taskId !== taskId || activeHistory.parentSessionId !== parentSessionId) {
        return;
      }
      const observation = observationFor(store, taskId);
      if (store.getState().connection.status !== "connected") return;
      const lease = errandBrowserReadCoordinator(store).acquire(
        browserReadKey("history", taskId, observation, { parentSessionId, cursor }),
        observation.observationRevision,
        (signal) =>
          getErrandHistoryPage(
            taskId,
            parentSessionId,
            { limit: HISTORY_PAGE_SIZE, ...(cursor === null ? {} : { cursor }) },
            signal,
          ),
      );
      const leaseKey = JSON.stringify([ownerId, cursor]);
      leases.current.get(leaseKey)?.release();
      leases.current.set(leaseKey, lease as ErrandBrowserReadLease<unknown>);
      store.getState().beginErrandHistoryRead(ownerId, observation, cursor, lease.attemptId);
      void lease.promise
        .then((page) => {
          store
            .getState()
            .setErrandHistoryPage(ownerId, page, observation, cursor, lease.attemptId);
        })
        .catch((error: unknown) => {
          if (!isAbortError(error))
            store.getState().failErrandHistoryRead(ownerId, lease.attemptId, error);
        })
        .finally(() => {
          lease.release();
          if (leases.current.get(leaseKey) === lease) leases.current.delete(leaseKey);
        });
    },
    [ownerId, parentSessionId, store, taskId],
  );
  const releaseHistory = useCallback(() => {
    const ownerPrefix = JSON.stringify([ownerId]).slice(0, -1);
    for (const [key, lease] of leases.current) {
      if (!key.startsWith(ownerPrefix)) continue;
      lease.release();
      leases.current.delete(key);
    }
  }, [ownerId]);

  useEffect(() => {
    const ownerLeases = leases.current;
    const ownerPrefix = JSON.stringify([ownerId]).slice(0, -1);
    return () => {
      for (const [key, lease] of ownerLeases) {
        if (!key.startsWith(ownerPrefix)) continue;
        lease.release();
        ownerLeases.delete(key);
      }
      const current = ownerId ? store.getState().errands.historyByOwnerId[ownerId] : undefined;
      if (current?.taskId === taskId && current.parentSessionId === parentSessionId && ownerId) {
        store.getState().closeErrandHistory(ownerId);
      }
    };
  }, [ownerId, parentSessionId, readScopeIdentity, store, taskId]);

  return { requestHistoryPage, releaseHistory };
}

function useErrandHistoryActions(args: {
  store: AppStore;
  taskId: string | null;
  parentSessionId: string | null;
  ownerId: string | undefined;
  history: ReturnType<typeof selectErrandBrowserHistory>;
}) {
  const { store, taskId, parentSessionId, ownerId, history } = args;
  const { requestHistoryPage, releaseHistory } = useErrandHistoryPageReads({
    store,
    taskId,
    parentSessionId,
    ownerId,
  });
  useEffect(() => {
    if (history && (history.taskId !== taskId || history.parentSessionId !== parentSessionId)) {
      releaseHistory();
    }
  }, [history, parentSessionId, releaseHistory, taskId]);
  const openHistory = useCallback(() => {
    if (!taskId || !parentSessionId || !ownerId) return;
    store.getState().openErrandHistory(ownerId, taskId, parentSessionId);
    const current = store.getState().errands.historyByOwnerId[ownerId];
    if (!current || current.pages.length === 0 || current.stale || current.status === "idle") {
      requestHistoryPage(null);
    }
  }, [ownerId, parentSessionId, requestHistoryPage, store, taskId]);
  const closeHistory = useCallback(() => {
    releaseHistory();
    const current = ownerId ? store.getState().errands.historyByOwnerId[ownerId] : undefined;
    if (current?.taskId === taskId && current.parentSessionId === parentSessionId && ownerId) {
      store.getState().closeErrandHistory(ownerId);
    }
  }, [ownerId, parentSessionId, releaseHistory, store, taskId]);
  const refreshHistory = useCallback(() => {
    if (!ownerId || !taskId || !parentSessionId) return;
    releaseHistory();
    store.getState().openErrandHistory(ownerId, taskId, parentSessionId);
    store.getState().restartErrandHistory(ownerId);
    requestHistoryPage(null);
  }, [ownerId, parentSessionId, releaseHistory, requestHistoryPage, store, taskId]);
  const loadMoreHistory = useCallback(() => {
    if (
      !history ||
      history.taskId !== taskId ||
      history.parentSessionId !== parentSessionId ||
      history.stale ||
      history.status === "loading" ||
      !history.hasMore
    ) {
      return;
    }
    if (history.nextCursor) requestHistoryPage(history.nextCursor);
  }, [history, parentSessionId, requestHistoryPage, taskId]);
  const retryHistory = useCallback(() => {
    if (
      !history ||
      history.taskId !== taskId ||
      history.parentSessionId !== parentSessionId ||
      history.status === "loading"
    ) {
      return;
    }
    requestHistoryPage(history.requestCursor ?? null);
  }, [history, parentSessionId, requestHistoryPage, taskId]);
  const startHistoryAgain = useCallback(() => {
    if (!ownerId || !taskId || !parentSessionId) return;
    releaseHistory();
    store.getState().openErrandHistory(ownerId, taskId, parentSessionId);
    store.getState().restartErrandHistory(ownerId);
    requestHistoryPage(null);
  }, [ownerId, parentSessionId, releaseHistory, requestHistoryPage, store, taskId]);
  return {
    openHistory,
    closeHistory,
    refreshHistory,
    loadMoreHistory,
    retryHistory,
    startHistoryAgain,
  };
}

/** Task-scoped compact Errand summaries; finished history is read only on explicit disclosure. */
export function useTaskErrands(
  taskId: string | null | undefined,
  options: { parentSessionId?: string | null; historyOwnerId?: string } = {},
): TaskErrands {
  const store = useAppStoreApi();
  const enabled = useFeature("errands");
  const scopedTaskId = enabled ? taskId : null;
  const parentSessionId = options.parentSessionId ?? null;
  const historyOwnerId = options.historyOwnerId;
  const summary = useAppStore((state) => selectErrandBrowserSummary(state, scopedTaskId));
  const outstanding = useAppStore((state) =>
    selectErrandBrowserRows(state, scopedTaskId, parentSessionId),
  );
  const summaryRead = useAppStore((state) =>
    scopedTaskId ? state.errands.browserSummaryReadByTaskId[scopedTaskId] : undefined,
  );
  const browserRevision = useAppStore((state) =>
    scopedTaskId ? (state.errands.browserRevisionByTaskId[scopedTaskId] ?? 0) : 0,
  );
  const taskResetEpoch = useAppStore((state) =>
    scopedTaskId ? (state.errands.resetEpochByTaskId[scopedTaskId] ?? 0) : 0,
  );
  const history = useAppStore((state) => selectErrandBrowserHistory(state, historyOwnerId));
  const historyRows = useAppStore((state) => selectErrandBrowserHistoryRows(state, historyOwnerId));
  const connectionStatus = useAppStore((state) => state.connection.status);
  const connectionGeneration = useAppStore((state) => state.connection.generation ?? 0);
  const readScopeIdentity = useAppStore(stateReadScopeIdentity);
  const [summaryRetryRevision, setSummaryRetryRevision] = useState(0);
  useErrandSummaryRead({
    store,
    taskId: scopedTaskId ?? null,
    browserRevision,
    connectionStatus,
    connectionGeneration,
    readScopeIdentity,
    retryRevision: summaryRetryRevision,
    taskResetEpoch,
  });
  useErrandConnectionGeneration(store, scopedTaskId ?? null, connectionGeneration);
  const historyActions = useErrandHistoryActions({
    store,
    taskId: scopedTaskId ?? null,
    parentSessionId,
    ownerId: historyOwnerId,
    history,
  });
  const retrySummary = useCallback(() => setSummaryRetryRevision((revision) => revision + 1), []);

  const parentCounts = useMemo(() => {
    if (!summary || !parentSessionId) return emptyCounts();
    return (
      summary.parent_counts.find((item) => item.parent_session_id === parentSessionId)?.counts ??
      emptyCounts()
    );
  }, [parentSessionId, summary]);
  const counts = summary?.counts ?? emptyCounts();
  const scopedHistory =
    history && history.taskId === scopedTaskId && history.parentSessionId === parentSessionId
      ? history
      : undefined;
  const scopedHistoryRows = scopedHistory ? historyRows : EMPTY_ROWS;

  return useMemo(
    () => ({
      outstanding: enabled ? outstanding : EMPTY_ROWS,
      counts: enabled ? counts : emptyCounts(),
      parentCounts: enabled ? parentCounts : emptyCounts(),
      summary: enabled ? summary : undefined,
      summaryStatus: enabled ? (summaryRead?.status ?? "idle") : "idle",
      summaryError: enabled ? summaryRead?.error : undefined,
      isSummaryLoaded: enabled && summaryRead?.status === "loaded",
      history: enabled ? scopedHistory : undefined,
      historyRows: enabled ? scopedHistoryRows : EMPTY_ROWS,
      ...historyActions,
      retrySummary,
    }),
    [
      counts,
      enabled,
      historyActions,
      scopedHistory,
      scopedHistoryRows,
      outstanding,
      parentCounts,
      retrySummary,
      summary,
      summaryRead?.error,
      summaryRead?.status,
    ],
  );
}
