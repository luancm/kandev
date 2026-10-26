import { describe, expect, it } from "vitest";
import { isLiveErrand, type Errand, type ErrandStatus } from "./errands";

const STATUSES: Record<ErrandStatus, boolean> = {
  queued: true,
  running: true,
  needs_attention: true,
  done: false,
  failed: false,
  cancelled: false,
};

describe("isLiveErrand", () => {
  it.each(Object.entries(STATUSES))("treats %s as live: %s", (status, live) => {
    expect(isLiveErrand({ status: status as ErrandStatus })).toBe(live);
  });
});

describe("Errand", () => {
  it("mirrors the backend ErrandDTO keys, with settled_at sent as null", () => {
    const errand: Errand = {
      errand_id: "errand-1",
      task_id: "task-1",
      parent_session_id: "session-parent",
      title: "API audit",
      objective: "Audit the API",
      agent_profile_id: "profile-1",
      status: "running",
      result_state: "none",
      result_preview: "",
      result_truncated: false,
      created_at: "2026-10-26T10:00:00Z",
      updated_at: "2026-10-26T10:00:00Z",
      settled_at: null,
    };

    expect(Object.keys(errand).sort()).toEqual([
      "agent_profile_id",
      "created_at",
      "errand_id",
      "objective",
      "parent_session_id",
      "result_preview",
      "result_state",
      "result_truncated",
      "settled_at",
      "status",
      "task_id",
      "title",
      "updated_at",
    ]);
  });
});
