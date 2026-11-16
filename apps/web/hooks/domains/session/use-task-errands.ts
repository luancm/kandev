import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import {
  selectErrandBrowserHistory,
  selectErrandBrowserHistoryRows,
  selectErrandBrowserRows,
  selectErrandBrowserSummary,
} from "@/lib/state/slices/errands/selectors";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import {
  readErrandSummary,
  type ErrandHistoryOwner,
} from "@/lib/state/slices/errands/errand-browser-read-coordinator";
import type {
  ErrandBrowserCounts,
  ErrandBrowserRow,
  ErrandBrowserSummary,
} from "@/lib/types/errands";

const EMPTY_ROWS: ErrandBrowserRow[] = [];
type AppStore = ReturnType<typeof useAppStoreApi>;

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

function historyForParent(
  history: ReturnType<typeof selectErrandBrowserHistory>,
  taskId: string | null | undefined,
  parentSessionId: string | null,
) {
  return history && history.taskId === taskId && history.parentSessionId === parentSessionId
    ? history
    : undefined;
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
    return readErrandSummary(store, taskId);
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

function useErrandHistoryActions(args: {
  taskId: string | null;
  parentSessionId: string | null;
  owner: ErrandHistoryOwner | undefined;
}) {
  const { taskId, parentSessionId, owner } = args;
  useEffect(
    () => () => {
      if (taskId && parentSessionId) owner?.close(taskId, parentSessionId);
    },
    [owner, taskId, parentSessionId],
  );
  const openHistory = useCallback(() => {
    if (taskId && parentSessionId) owner?.open(taskId, parentSessionId);
  }, [owner, taskId, parentSessionId]);
  const closeHistory = useCallback(() => {
    if (taskId && parentSessionId) owner?.close(taskId, parentSessionId);
  }, [owner, taskId, parentSessionId]);
  const refreshHistory = useCallback(() => {
    if (taskId && parentSessionId) owner?.restart(taskId, parentSessionId);
  }, [owner, taskId, parentSessionId]);
  const loadMoreHistory = useCallback(() => {
    if (taskId && parentSessionId) owner?.more(taskId, parentSessionId);
  }, [owner, taskId, parentSessionId]);
  const retryHistory = useCallback(() => {
    if (taskId && parentSessionId) owner?.retry(taskId, parentSessionId);
  }, [owner, taskId, parentSessionId]);
  return {
    openHistory,
    closeHistory,
    refreshHistory,
    loadMoreHistory,
    retryHistory,
    startHistoryAgain: refreshHistory,
  };
}

/** Task-scoped compact Errand summaries; finished history is read only on explicit disclosure. */
export function useTaskErrands(
  taskId: string | null | undefined,
  options: { parentSessionId?: string | null; historyOwner?: ErrandHistoryOwner } = {},
): TaskErrands {
  const store = useAppStoreApi();
  const enabled = useFeature("errands");
  const scopedTaskId = enabled ? taskId : null;
  const parentSessionId = options.parentSessionId ?? null;
  const historyOwner = options.historyOwner;
  const historyOwnerId = historyOwner?.id;
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
    taskId: scopedTaskId ?? null,
    parentSessionId,
    owner: historyOwner,
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
  const scopedHistory = historyForParent(history, scopedTaskId, parentSessionId);
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
