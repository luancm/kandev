import { forgetErrandInspection } from "./errand-inspection-state";
import type { StateCreator } from "zustand";
import { stateReadScopeIdentity } from "../../shared-resource-reads";
import type { AppState } from "../../app-state-types";
import {
  isLiveErrand,
  normalizeErrandAttention,
  type Errand,
  type ErrandBrowserRow,
  type ErrandHistoryPage,
} from "@/lib/types/errands";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import type { ErrandBrowserReadObservation, ErrandsSlice, ErrandsSliceState } from "./types";

export const defaultErrandsState: ErrandsSliceState = {
  errands: {
    byTaskId: {},
    resetEpochByTaskId: {},
    removedSessionIdsByTaskId: {},
    browserSummaryByTaskId: {},
    browserSummaryReadByTaskId: {},
    browserRevisionByTaskId: {},
    unreadSignatureByTaskId: {},
    historyByOwnerId: {},
    detailBySessionId: {},
    detailOwnersBySessionId: {},
  },
};

export function mergeErrandsHydrationState(
  initialErrands: ErrandsSliceState["errands"] | undefined,
): ErrandsSliceState["errands"] {
  return {
    ...defaultErrandsState.errands,
    ...initialErrands,
    browserReadScopeIdentity: undefined,
    detailObservationRevisionBySessionId: undefined,
    resetEpochByTaskId: {},
    browserSummaryByTaskId: {},
    browserSummaryReadByTaskId: {},
    browserRevisionByTaskId: {},
    unreadSignatureByTaskId: {},
    historyByOwnerId: {},
    detailBySessionId: {},
    detailOwnersBySessionId: {},
  };
}

type ImmerSet = Parameters<StateCreator<AppState, [["zustand/immer", never]], [], ErrandsSlice>>[0];

/** Unparseable timestamps never count as older, so a malformed row cannot freeze an errand. */
function isOlderErrand(
  incoming: Pick<Errand, "updated_at">,
  existing: Pick<Errand, "updated_at"> | undefined,
): boolean {
  if (!existing) return false;
  const incomingAt = parseStrictRfc3339Timestamp(incoming.updated_at);
  const existingAt = parseStrictRfc3339Timestamp(existing.updated_at);
  return incomingAt !== null && existingAt !== null && incomingAt < existingAt;
}

function browserObservationIsCurrent(
  state: AppState,
  taskId: string,
  observation: ErrandBrowserReadObservation,
): boolean {
  return (
    state.connection.status === "connected" &&
    (state.connection.generation ?? 0) === observation.connectionGeneration &&
    stateReadScopeIdentity(state) === observation.readScopeIdentity &&
    (state.errands.resetEpochByTaskId?.[taskId] ?? 0) === observation.taskResetEpoch &&
    (state.errands.browserRevisionByTaskId?.[taskId] ?? 0) === observation.observationRevision
  );
}

function markHistoryChainsStale(
  historyByOwnerId: ErrandsSliceState["errands"]["historyByOwnerId"],
  taskId: string,
  parentSessionId?: string,
): void {
  for (const chain of Object.values(historyByOwnerId)) {
    if (
      chain.taskId === taskId &&
      (!parentSessionId || chain.parentSessionId === parentSessionId)
    ) {
      chain.stale = true;
    }
  }
}

function nextBrowserRevision(state: AppState, taskId: string, contextOnly = false): number {
  const previous = state.errands.browserRevisionByTaskId[taskId] ?? 0;
  const next = previous + 1;
  state.errands.browserRevisionByTaskId[taskId] = next;
  if (contextOnly) {
    const read = state.errands.browserSummaryReadByTaskId[taskId];
    if (read?.status === "loaded" && read.observationRevision === previous) {
      read.observationRevision = next;
    }
  } else {
    markHistoryChainsStale(state.errands.historyByOwnerId, taskId);
  }
  return next;
}

function compactFromEvent(existing: ErrandBrowserRow, incoming: Errand): ErrandBrowserRow {
  const metadata = incoming.metadata && { ...existing.metadata, ...incoming.metadata };
  if (incoming.context_only) return metadata ? { ...existing, metadata } : existing;
  return {
    ...existing,
    ...(metadata && { metadata }),
    title: incoming.title,
    agent_profile_id: incoming.agent_profile_id,
    status: incoming.status,
    result_state: incoming.result_state,
    delivery_state: incoming.delivery_state,
    result_truncated: incoming.result_truncated,
    updated_at: incoming.updated_at,
    settled_at: incoming.settled_at,
    attention_reason: incoming.attention_reason,
    live: isLiveErrand(incoming),
    outstanding:
      isLiveErrand(incoming) ||
      incoming.result_state === "pending" ||
      incoming.can_send_result === true,
    can_send_result: incoming.can_send_result ?? existing.can_send_result,
  };
}

function updateCompactRow(
  rows: ErrandBrowserRow[],
  incoming: Errand,
  section: "summary" | "history",
): void {
  const index = rows.findIndex((row) => row.errand_id === incoming.errand_id);
  if (index < 0) return;
  const updated = compactFromEvent(rows[index], incoming);
  const belongs =
    section === "summary" ? updated.outstanding : !updated.live && !updated.outstanding;
  if (incoming.context_only || belongs) rows[index] = updated;
  else rows.splice(index, 1);
}

function updateKnownBrowserRows(state: AppState, incoming: Errand): void {
  const inspection = state.errands.detailBySessionId[incoming.errand_id];
  if (inspection?.scope.task_id === incoming.task_id) {
    inspection.errand = compactFromEvent(inspection.errand, incoming);
  }
  const summary = state.errands.browserSummaryByTaskId[incoming.task_id];
  if (summary) updateCompactRow(summary.errands, incoming, "summary");
  for (const chain of Object.values(state.errands.historyByOwnerId)) {
    if (chain.taskId !== incoming.task_id) continue;
    for (const page of chain.pages) {
      updateCompactRow(page.errands, incoming, "history");
    }
  }
}

function rowIsTombstoned(state: AppState, taskId: string, row: ErrandBrowserRow): boolean {
  return Boolean(state.errands.removedSessionIdsByTaskId[taskId]?.[row.errand_id]);
}

function filterPageTombstones(
  state: AppState,
  taskId: string,
  page: ErrandHistoryPage,
): ErrandHistoryPage {
  return {
    ...page,
    errands: page.errands.filter((row) => !rowIsTombstoned(state, taskId, row)),
  };
}

function newestKnownErrand(state: AppState, taskId: string, id: string) {
  let newest: Pick<Errand, "updated_at"> | undefined;
  const accept = (row: Pick<Errand, "updated_at"> | undefined) => {
    if (row && (!newest || isOlderErrand(newest, row))) newest = row;
  };
  accept(state.errands.byTaskId[taskId]?.[id]);
  accept(state.errands.detailBySessionId[id]?.errand);
  accept(state.errands.browserSummaryByTaskId[taskId]?.errands.find((row) => row.errand_id === id));
  for (const chain of Object.values(state.errands.historyByOwnerId)) {
    if (chain.taskId !== taskId) continue;
    for (const page of chain.pages) accept(page.errands.find((row) => row.errand_id === id));
  }
  return newest;
}

function pruneOperationalRows(state: AppState, taskId: string, rows: ErrandBrowserRow[]): void {
  const liveIds = new Set(rows.filter((row) => row.live).map((row) => row.errand_id));
  const operational = state.errands.byTaskId[taskId];
  for (const id of Object.keys(operational ?? {})) {
    if (liveIds.has(id)) continue;
    delete operational[id];
  }
}

function applyOperationalErrand(state: AppState, snapshot: Errand): boolean {
  const taskId = snapshot.task_id;
  const errands = state.errands.byTaskId[taskId];
  const existing = newestKnownErrand(state, taskId, snapshot.errand_id);
  if (isOlderErrand(snapshot, existing)) return false;
  if (snapshot.context_only) {
    if (!snapshot.metadata) return false;
    const current = errands?.[snapshot.errand_id];
    if (current) current.metadata = { ...current.metadata, ...snapshot.metadata };
    return true;
  }
  if (isLiveErrand(snapshot))
    (state.errands.byTaskId[taskId] ??= {})[snapshot.errand_id] = snapshot;
  else if (errands) delete errands[snapshot.errand_id];
  return true;
}

function createOperationalErrandActions(
  set: ImmerSet,
): Pick<ErrandsSlice, "upsertErrand" | "removeTaskErrands"> {
  return {
    upsertErrand: (errand) =>
      set((draft) => {
        const snapshot = normalizeErrandAttention(errand);
        const taskId = snapshot.task_id;
        if (draft.errands.removedSessionIdsByTaskId[taskId]?.[snapshot.errand_id]) return;
        if (!applyOperationalErrand(draft, snapshot)) return;
        updateKnownBrowserRows(draft, snapshot);
        const browserRevision = nextBrowserRevision(draft, taskId, snapshot.context_only);
        if (draft.errands.detailBySessionId[snapshot.errand_id]) {
          (draft.errands.detailObservationRevisionBySessionId ??= {})[snapshot.errand_id] =
            browserRevision;
        }
      }),

    removeTaskErrands: (taskId) =>
      set((draft) => {
        (draft.errands.resetEpochByTaskId ??= {})[taskId] =
          (draft.errands.resetEpochByTaskId?.[taskId] ?? 0) + 1;
        delete draft.errands.byTaskId[taskId];
        delete draft.errands.removedSessionIdsByTaskId[taskId];
        delete draft.errands.browserSummaryByTaskId[taskId];
        delete draft.errands.browserSummaryReadByTaskId[taskId];
        delete draft.errands.browserRevisionByTaskId[taskId];
        delete draft.errands.unreadSignatureByTaskId[taskId];
        for (const [ownerId, chain] of Object.entries(draft.errands.historyByOwnerId)) {
          if (chain.taskId === taskId) delete draft.errands.historyByOwnerId[ownerId];
        }
        for (const [sessionId, inspection] of Object.entries(draft.errands.detailBySessionId)) {
          if (inspection.scope.task_id !== taskId) continue;
          forgetErrandInspection(draft.errands, sessionId);
        }
      }),
  };
}

function createBrowserSummaryActions(
  set: ImmerSet,
): Pick<
  ErrandsSlice,
  "beginErrandBrowserSummaryRead" | "setErrandBrowserSummary" | "failErrandBrowserSummaryRead"
> {
  return {
    beginErrandBrowserSummaryRead: (taskId, observation, attemptId) =>
      set((draft) => {
        if (!browserObservationIsCurrent(draft, taskId, observation)) return;
        if (draft.errands.browserSummaryReadByTaskId[taskId]?.attemptId === attemptId) return;
        draft.errands.browserReadScopeIdentity = observation.readScopeIdentity;
        draft.errands.browserSummaryReadByTaskId[taskId] = {
          ...observation,
          status: "loading",
          attemptId,
        };
      }),

    setErrandBrowserSummary: (taskId, summary, observation, attemptId) =>
      set((draft) => {
        const read = draft.errands.browserSummaryReadByTaskId[taskId];
        if (
          !read ||
          read.status !== "loading" ||
          read.attemptId !== attemptId ||
          !browserObservationIsCurrent(draft, taskId, observation) ||
          summary.scope.task_id !== taskId
        ) {
          return;
        }
        const removed = draft.errands.removedSessionIdsByTaskId[taskId] ?? {};
        draft.errands.browserSummaryByTaskId[taskId] = {
          ...summary,
          errands: summary.errands.filter((row) => !removed[row.errand_id]),
        };
        pruneOperationalRows(draft, taskId, draft.errands.browserSummaryByTaskId[taskId].errands);
        const outstandingIds = new Set(
          draft.errands.browserSummaryByTaskId[taskId].errands.map((row) => row.errand_id),
        );
        for (const chain of Object.values(draft.errands.historyByOwnerId)) {
          if (chain.taskId !== taskId) continue;
          let removedHistoryRow = false;
          for (const page of chain.pages) {
            const filtered = page.errands.filter((row) => !outstandingIds.has(row.errand_id));
            if (filtered.length !== page.errands.length) {
              page.errands = filtered;
              removedHistoryRow = true;
            }
          }
          if (removedHistoryRow) chain.stale = true;
        }
        read.status = "loaded";
        delete read.error;
      }),

    failErrandBrowserSummaryRead: (taskId, observation, attemptId, error) =>
      set((draft) => {
        const read = draft.errands.browserSummaryReadByTaskId[taskId];
        if (
          !read ||
          read.status !== "loading" ||
          read.attemptId !== attemptId ||
          !browserObservationIsCurrent(draft, taskId, observation)
        ) {
          return;
        }
        read.status = "error";
        read.error = error;
      }),
  };
}

function createErrandHistoryActions(
  set: ImmerSet,
): Pick<
  ErrandsSlice,
  | "openErrandHistory"
  | "restartErrandHistory"
  | "beginErrandHistoryRead"
  | "failErrandHistoryRead"
  | "closeErrandHistory"
  | "markErrandHistoryStale"
> {
  return {
    openErrandHistory: (ownerId, taskId, parentSessionId) =>
      set((draft) => {
        draft.errands.browserReadScopeIdentity = stateReadScopeIdentity(draft);
        const current = draft.errands.historyByOwnerId[ownerId];
        if (current?.taskId === taskId && current.parentSessionId === parentSessionId) return;
        draft.errands.historyByOwnerId[ownerId] = {
          taskId,
          parentSessionId,
          pages: [],
          totalFinished: 0,
          hasMore: false,
          nextCursor: null,
          rangeStart: 0,
          status: "idle",
          stale: false,
        };
      }),

    restartErrandHistory: (ownerId) =>
      set((draft) => {
        const chain = draft.errands.historyByOwnerId[ownerId];
        if (!chain) return;
        chain.pages = [];
        chain.totalFinished = 0;
        chain.hasMore = false;
        chain.nextCursor = null;
        chain.rangeStart = 0;
        chain.status = "idle";
        chain.stale = false;
        delete chain.error;
        delete chain.attemptId;
      }),

    beginErrandHistoryRead: (ownerId, observation, cursor, attemptId) =>
      set((draft) => {
        const chain = draft.errands.historyByOwnerId[ownerId];
        if (
          !chain ||
          !browserObservationIsCurrent(draft, chain.taskId, observation) ||
          (cursor !== null && cursor !== chain.nextCursor)
        ) {
          return;
        }
        chain.status = "loading";
        chain.error = undefined;
        chain.requestCursor = cursor;
        chain.attemptId = attemptId;
        chain.observation = observation;
      }),

    failErrandHistoryRead: (ownerId, attemptId, error) =>
      set((draft) => {
        const chain = draft.errands.historyByOwnerId[ownerId];
        if (!chain || chain.attemptId !== attemptId) return;
        chain.status = "error";
        chain.error = error;
      }),

    closeErrandHistory: (ownerId) =>
      set((draft) => {
        delete draft.errands.historyByOwnerId[ownerId];
      }),

    markErrandHistoryStale: (taskId, parentSessionId) =>
      set((draft) => {
        markHistoryChainsStale(draft.errands.historyByOwnerId, taskId, parentSessionId);
      }),
  };
}

function createErrandHistoryPageAction(set: ImmerSet): Pick<ErrandsSlice, "setErrandHistoryPage"> {
  return {
    setErrandHistoryPage: (ownerId, page, observation, cursor, attemptId) =>
      set((draft) => {
        const chain = draft.errands.historyByOwnerId[ownerId];
        if (
          !chain ||
          chain.attemptId !== attemptId ||
          chain.requestCursor !== cursor ||
          page.scope.task_id !== chain.taskId ||
          page.scope.parent_session_id !== chain.parentSessionId
        ) {
          return;
        }
        if (!browserObservationIsCurrent(draft, chain.taskId, observation)) {
          chain.status = chain.pages.length > 0 ? "loaded" : "idle";
          chain.stale = true;
          return;
        }
        const filtered = filterPageTombstones(draft, chain.taskId, page);
        if (cursor === null) {
          chain.pages = [filtered];
          chain.rangeStart = 0;
        } else {
          chain.pages.push(filtered);
          if (chain.pages.length > 4) {
            const removed = chain.pages.shift();
            chain.rangeStart += removed?.errands.length ?? 0;
          }
        }
        chain.totalFinished = page.total_finished;
        chain.hasMore = page.has_more;
        chain.nextCursor = page.next_cursor;
        chain.status = "loaded";
        chain.stale = false;
        delete chain.error;
      }),
  };
}

function createErrandBrowserInvalidationActions(
  set: ImmerSet,
): Pick<
  ErrandsSlice,
  "markErrandBrowserUnread" | "advanceErrandBrowserRevision" | "removeErrandBrowserSession"
> {
  return {
    markErrandBrowserUnread: (taskId, unreadSessionIds) =>
      set((draft) => {
        const signature = unreadSessionIds.slice().sort().join("\u0000");
        if (draft.errands.unreadSignatureByTaskId[taskId] === signature) return;
        draft.errands.unreadSignatureByTaskId[taskId] = signature;
        const unreadIds = new Set(unreadSessionIds);
        const summary = draft.errands.browserSummaryByTaskId[taskId];
        if (summary) {
          for (const row of summary.errands) row.unread = unreadIds.has(row.errand_id);
        }
        for (const chain of Object.values(draft.errands.historyByOwnerId)) {
          if (chain.taskId !== taskId) continue;
          for (const page of chain.pages) {
            for (const row of page.errands) row.unread = unreadIds.has(row.errand_id);
          }
        }
        nextBrowserRevision(draft, taskId);
      }),

    advanceErrandBrowserRevision: (taskId) =>
      set((draft) => {
        nextBrowserRevision(draft, taskId);
      }),

    removeErrandBrowserSession: (taskId, sessionId) =>
      set((draft) => {
        (draft.errands.removedSessionIdsByTaskId[taskId] ??= {})[sessionId] = true;
        delete draft.errands.byTaskId[taskId]?.[sessionId];
        const summary = draft.errands.browserSummaryByTaskId[taskId];
        if (summary) summary.errands = summary.errands.filter((row) => row.errand_id !== sessionId);
        for (const chain of Object.values(draft.errands.historyByOwnerId)) {
          if (chain.taskId !== taskId) continue;
          for (const page of chain.pages) {
            page.errands = page.errands.filter((row) => row.errand_id !== sessionId);
          }
        }
        forgetErrandInspection(draft.errands, sessionId);
        nextBrowserRevision(draft, taskId);
      }),
  };
}

function createErrandInspectionActions(
  set: ImmerSet,
): Pick<ErrandsSlice, "retainErrandDetail" | "retainErrandInspection" | "releaseErrandInspection"> {
  return {
    retainErrandDetail: (ownerId, sessionId) =>
      set((draft) => {
        (draft.errands.detailOwnersBySessionId[sessionId] ??= {})[ownerId] = true;
      }),
    retainErrandInspection: (ownerId, inspection) =>
      set((draft) => {
        draft.errands.browserReadScopeIdentity = stateReadScopeIdentity(draft);
        for (const [sessionId, owners] of Object.entries(draft.errands.detailOwnersBySessionId)) {
          if (!owners[ownerId] || sessionId === inspection.errand.errand_id) continue;
          delete owners[ownerId];
          if (Object.keys(owners).length === 0) {
            forgetErrandInspection(draft.errands, sessionId);
          }
        }
        const sessionId = inspection.errand.errand_id;
        draft.errands.detailBySessionId[sessionId] = inspection;
        (draft.errands.detailObservationRevisionBySessionId ??= {})[sessionId] =
          draft.errands.browserRevisionByTaskId[inspection.scope.task_id] ?? 0;
        (draft.errands.detailOwnersBySessionId[sessionId] ??= {})[ownerId] = true;
      }),

    releaseErrandInspection: (ownerId) =>
      set((draft) => {
        for (const [sessionId, owners] of Object.entries(draft.errands.detailOwnersBySessionId)) {
          if (!owners[ownerId]) continue;
          delete owners[ownerId];
          if (Object.keys(owners).length === 0) {
            forgetErrandInspection(draft.errands, sessionId);
          }
        }
      }),
  };
}

export const createErrandsSlice = (set: ImmerSet): ErrandsSlice => ({
  ...defaultErrandsState,
  resetErrandReadScope: () =>
    set((draft) => {
      const taskIds = new Set([
        ...Object.keys(draft.errands.resetEpochByTaskId),
        ...Object.keys(draft.errands.byTaskId),
        ...Object.keys(draft.errands.browserSummaryReadByTaskId),
        ...Object.values(draft.errands.historyByOwnerId).map((chain) => chain.taskId),
        ...Object.values(draft.errands.detailBySessionId).map((detail) => detail.scope.task_id),
      ]);
      const epochs = Object.fromEntries(
        [...taskIds].map((taskId) => [taskId, (draft.errands.resetEpochByTaskId[taskId] ?? 0) + 1]),
      );
      draft.errands = {
        ...defaultErrandsState.errands,
        resetEpochByTaskId: epochs,
        scopeResetEpoch: (draft.errands.scopeResetEpoch ?? 0) + 1,
      };
    }),
  ...createOperationalErrandActions(set),
  ...createBrowserSummaryActions(set),
  ...createErrandHistoryActions(set),
  ...createErrandHistoryPageAction(set),
  ...createErrandBrowserInvalidationActions(set),
  ...createErrandInspectionActions(set),
});
