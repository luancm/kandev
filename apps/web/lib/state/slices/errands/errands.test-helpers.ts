import type {
  Errand,
  ErrandBrowserCounts,
  ErrandBrowserRow,
  ErrandBrowserSummary,
  ErrandHistoryPage,
  ErrandInspection,
} from "@/lib/types/errands";

export const ERRAND_TASK_ID = "task-1";
export const ERRAND_PARENT_ID = "session-parent";
const FIXTURE_TIMESTAMP = "2026-10-26T10:00:00Z";

export function makeErrand(overrides: Partial<Errand> = {}): Errand {
  return {
    errand_id: "errand-1",
    task_id: ERRAND_TASK_ID,
    parent_session_id: ERRAND_PARENT_ID,
    // i18n-exempt: test fixture data, never rendered
    title: "API audit",
    objective: "Audit the public API",
    agent_profile_id: "profile-1",
    status: "running",
    result_state: "none",
    result_preview: "",
    result_truncated: false,
    created_at: FIXTURE_TIMESTAMP,
    updated_at: FIXTURE_TIMESTAMP,
    settled_at: null,
    ...overrides,
  };
}

export function makeBrowserErrandRow(overrides: Partial<ErrandBrowserRow> = {}): ErrandBrowserRow {
  return {
    errand_id: "errand-1",
    task_id: ERRAND_TASK_ID,
    parent_session_id: ERRAND_PARENT_ID,
    // i18n-exempt: test fixture data, never rendered
    title: "API audit",
    agent_profile_id: "profile-1",
    status: "done",
    result_state: "delivered",
    result_truncated: false,
    created_at: FIXTURE_TIMESTAMP,
    updated_at: FIXTURE_TIMESTAMP,
    settled_at: FIXTURE_TIMESTAMP,
    version: 1,
    live: false,
    outstanding: false,
    unread: false,
    can_send_result: false,
    tokens_in: 0,
    tokens_cached_in: 0,
    tokens_out: 0,
    ...overrides,
  };
}

export function makeBrowserCounts(
  overrides: Partial<ErrandBrowserCounts> = {},
): ErrandBrowserCounts {
  return {
    live: 0,
    capacity: 4,
    queued: 0,
    outstanding: 0,
    finished: 0,
    pending_results: 0,
    unread: 0,
    ...overrides,
  };
}

export function makeBrowserSummary(
  errands: ErrandBrowserRow[] = [],
  counts: Partial<ErrandBrowserCounts> = {},
): ErrandBrowserSummary {
  return {
    scope: { task_id: ERRAND_TASK_ID },
    errands,
    counts: makeBrowserCounts(counts),
    parent_counts: [{ parent_session_id: ERRAND_PARENT_ID, counts: makeBrowserCounts(counts) }],
  };
}

export function makeHistoryPage(
  errands: ErrandBrowserRow[],
  options: { total?: number; hasMore?: boolean; cursor?: string | null } = {},
): ErrandHistoryPage {
  return {
    scope: { task_id: ERRAND_TASK_ID, parent_session_id: ERRAND_PARENT_ID },
    errands,
    total_finished: options.total ?? errands.length,
    has_more: options.hasMore ?? false,
    next_cursor: options.cursor ?? null,
  };
}

export function makeInspection(row = makeBrowserErrandRow()): ErrandInspection {
  return {
    scope: { task_id: row.task_id, child_session_id: row.errand_id },
    errand: row,
    child_session_exists: true,
    // i18n-exempt: test fixture data, never rendered
    parent: { available: true, id: row.parent_session_id, title: "Parent", role: "primary" },
  };
}
