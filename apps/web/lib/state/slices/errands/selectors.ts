import type { AppState } from "@/lib/state/store";
import { isLiveErrand, type Errand } from "@/lib/types/errands";
import type { TaskSession } from "@/lib/types/http";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

type ErrandsState = Pick<AppState, "errands">;
type ErrandMap = Record<string, Errand>;
type TaskIdInput = string | null | undefined;

const EMPTY_ERRANDS: Errand[] = [];
const EMPTY_GROUPS: Record<string, Errand[]> = {};

// Derived lists are cached by the task map's identity, which changes on every
// write, so selectors return stable references for useAppStore.
const sortedByMap = new WeakMap<ErrandMap, Errand[]>();
const groupsByMap = new WeakMap<ErrandMap, Record<string, Errand[]>>();

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
