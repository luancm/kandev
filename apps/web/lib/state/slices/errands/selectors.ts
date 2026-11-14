import { stateReadScopeIdentity } from "../../shared-resource-reads";
import type { AppState } from "@/lib/state/store";
import {
  isLiveErrand,
  type Errand,
  type ErrandBrowserRow,
  type ErrandBrowserSummary,
} from "@/lib/types/errands";
import type { TaskSession } from "@/lib/types/http";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
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

const EMPTY_ERRANDS: Errand[] = [];
const EMPTY_GROUPS: Record<string, Errand[]> = {};

// Derived lists are cached by the task map's identity, which changes on every
// write, so selectors return stable references for useAppStore.
const sortedByMap = new WeakMap<ErrandMap, Errand[]>();
const groupsByMap = new WeakMap<ErrandMap, Record<string, Errand[]>>();
const historyRowsByChain = new WeakMap<ErrandHistoryChain, ErrandBrowserRow[]>();
const browserRowsBySummary = new WeakMap<
  ErrandBrowserSummary,
  Record<string, ErrandBrowserRow[]>
>();

function taskErrandMap(state: ErrandsState, taskId: TaskIdInput): ErrandMap | undefined {
  return taskId ? state.errands.byTaskId[taskId] : undefined;
}

function createdAtNanos(errand: Errand): bigint {
  return parseStrictRfc3339Timestamp(errand.created_at) ?? BigInt(0);
}

function compareByCreation(a: Errand, b: Errand): number {
  const delta = createdAtNanos(a) - createdAtNanos(b);
  if (delta !== BigInt(0)) return delta < BigInt(0) ? -1 : 1;
  return a.errand_id.localeCompare(b.errand_id);
}

/** A task's errands in creation order. */
export function selectTaskErrands(state: ErrandsState, taskId: TaskIdInput): Errand[] {
  const map = taskErrandMap(state, taskId);
  if (!map) return EMPTY_ERRANDS;
  let sorted = sortedByMap.get(map);
  if (!sorted) {
    sorted = Object.values(map).sort(compareByCreation);
    sortedByMap.set(map, sorted);
  }
  return sorted;
}

/** A task's errands grouped by parent session ID, each group in creation order. */
export function selectErrandsByParent(
  state: ErrandsState,
  taskId: TaskIdInput,
): Record<string, Errand[]> {
  const map = taskErrandMap(state, taskId);
  if (!map) return EMPTY_GROUPS;
  let groups = groupsByMap.get(map);
  if (!groups) {
    groups = {};
    for (const errand of selectTaskErrands(state, taskId)) {
      (groups[errand.parent_session_id] ??= []).push(errand);
    }
    groupsByMap.set(map, groups);
  }
  return groups;
}

/** The errands one parent session started, in creation order. */
export function selectParentErrands(
  state: ErrandsState,
  taskId: TaskIdInput,
  parentSessionId: string,
): Errand[] {
  return selectErrandsByParent(state, taskId)[parentSessionId] ?? EMPTY_ERRANDS;
}

/** How many of a task's errands are live (queued, running, or needing attention). */
export function selectLiveErrandCount(state: ErrandsState, taskId: TaskIdInput): number {
  let live = 0;
  for (const errand of selectTaskErrands(state, taskId)) {
    if (isLiveErrand(errand)) live += 1;
  }
  return live;
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

/**
 * Whether a task session is an errand: by its role, or by a known errand row
 * when the session arrived as a partial row before its role.
 */
export function isErrandSession(
  state: ErrandsState,
  taskId: string,
  session: Pick<TaskSession, "id" | "role">,
): boolean {
  return session.role === "errand" || Boolean(taskErrandMap(state, taskId)?.[session.id]);
}
