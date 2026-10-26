import type { Errand } from "@/lib/types/errands";

export const ERRAND_TASK_ID = "task-1";
export const ERRAND_PARENT_ID = "session-parent";

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
    created_at: "2026-10-26T10:00:00Z",
    updated_at: "2026-10-26T10:00:00Z",
    settled_at: null,
    ...overrides,
  };
}
