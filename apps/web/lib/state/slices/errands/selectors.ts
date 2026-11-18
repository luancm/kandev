import { stateReadScopeIdentity } from "../../shared-resource-reads";
import type { AppState } from "@/lib/state/store";
import type { Errand, ErrandBrowserRow, ErrandBrowserSummary } from "@/lib/types/errands";
import type { ErrandHistoryChain } from "./types";

type ErrandsState = Pick<AppState, "errands"> &
  Partial<Pick<AppState, "auth" | "workspaceContextGeneration">>;

function browserScopeMatches(state: ErrandsState): boolean {
  return (
    state.errands.browserReadScopeIdentity === undefined ||
    state.errands.browserReadScopeIdentity === stateReadScopeIdentity(state)
  );
}
type ErrandMap = Record<string, Errand>;
type TaskIdInput = string | null | undefined;

const historyRowsByChain = new WeakMap<ErrandHistoryChain, ErrandBrowserRow[]>();
const browserRowsBySummary = new WeakMap<
  ErrandBrowserSummary,
  Record<string, ErrandBrowserRow[]>
>();

function taskErrandMap(state: ErrandsState, taskId: TaskIdInput): ErrandMap | undefined {
  return taskId ? state.errands.byTaskId[taskId] : undefined;
}

/** The errand backed by a session; an errand's ID is its session ID. */
export function selectErrandBySessionId(
  state: ErrandsState,
  taskId: TaskIdInput,
  sessionId: string,
): Errand | undefined {
  return taskErrandMap(state, taskId)?.[sessionId];
}

export function selectErrandBrowserSummary(
  state: ErrandsState,
  taskId: TaskIdInput,
): ErrandBrowserSummary | undefined {
  return taskId && browserScopeMatches(state)
    ? state.errands.browserSummaryByTaskId[taskId]
    : undefined;
}

export function selectErrandBrowserRows(
  state: ErrandsState,
  taskId: TaskIdInput,
  parentSessionId?: string | null,
): ErrandBrowserRow[] {
  const rows = selectErrandBrowserSummary(state, taskId)?.errands;
  if (!rows) return EMPTY_BROWSER_ROWS;
  if (!parentSessionId) return rows;
  const summary = state.errands.browserSummaryByTaskId[taskId!];
  if (!summary) return EMPTY_BROWSER_ROWS;
  let byParent = browserRowsBySummary.get(summary);
  if (!byParent) {
    byParent = {};
    for (const row of summary.errands) (byParent[row.parent_session_id] ??= []).push(row);
    browserRowsBySummary.set(summary, byParent);
  }
  return byParent[parentSessionId] ?? EMPTY_BROWSER_ROWS;
}

const EMPTY_BROWSER_ROWS: ErrandBrowserRow[] = [];

export function selectErrandBrowserHistory(
  state: ErrandsState,
  ownerId: string | null | undefined,
): ErrandHistoryChain | undefined {
  return ownerId && browserScopeMatches(state)
    ? state.errands.historyByOwnerId[ownerId]
    : undefined;
}

export function selectErrandBrowserHistoryRows(
  state: ErrandsState,
  ownerId: string | null | undefined,
): ErrandBrowserRow[] {
  const chain = selectErrandBrowserHistory(state, ownerId);
  if (!chain) return EMPTY_BROWSER_ROWS;
  let rows = historyRowsByChain.get(chain);
  if (!rows) {
    rows = chain.pages.flatMap((page) => page.errands);
    historyRowsByChain.set(chain, rows);
  }
  return rows;
}

export function selectErrandInspection(state: ErrandsState, sessionId: string | null | undefined) {
  return sessionId && browserScopeMatches(state)
    ? state.errands.detailBySessionId[sessionId]
    : undefined;
}

export function selectErrandBrowserRowBySessionId(
  state: ErrandsState,
  taskId: TaskIdInput,
  sessionId: string | null | undefined,
): ErrandBrowserRow | undefined {
  if (!taskId || !sessionId || !browserScopeMatches(state)) return undefined;
  const inspection = state.errands.detailBySessionId[sessionId];
  if (inspection?.scope.task_id === taskId) return inspection.errand;
  const outstanding = state.errands.browserSummaryByTaskId[taskId]?.errands.find(
    (row) => row.errand_id === sessionId,
  );
  if (outstanding) return outstanding;
  for (const chain of Object.values(state.errands.historyByOwnerId)) {
    if (chain.taskId !== taskId) continue;
    const row = chain.pages
      .flatMap((page) => page.errands)
      .find((item) => item.errand_id === sessionId);
    if (row) return row;
  }
  return undefined;
}
