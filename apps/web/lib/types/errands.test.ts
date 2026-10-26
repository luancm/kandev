import { describe, expect, it } from "vitest";
import {
  canSendErrandResult,
  isLiveErrand,
  parseErrandStatus,
  type Errand,
  type ErrandStatus,
} from "./errands";

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

  it("accepts the server-computed can_send_result flag", () => {
    const errand: Errand = {
      errand_id: "errand-1",
      task_id: "task-1",
      parent_session_id: "session-parent",
      title: "API audit",
      objective: "Audit the API",
      agent_profile_id: "profile-1",
      status: "done",
      result_state: "delivered",
      result_preview: "",
      result_truncated: false,
      created_at: "2026-10-26T10:00:00Z",
      updated_at: "2026-10-26T10:05:00Z",
      settled_at: "2026-10-26T10:05:00Z",
      can_send_result: true,
    };

    expect(errand.can_send_result).toBe(true);
  });
});

describe("canSendErrandResult", () => {
  it.each(Object.keys(STATUSES) as ErrandStatus[])(
    "offers Send result only for an eligible settled errand (%s)",
    (status) => {
      const live = STATUSES[status];
      expect(canSendErrandResult({ status, can_send_result: true })).toBe(!live);
      expect(canSendErrandResult({ status, can_send_result: false })).toBe(false);
      expect(canSendErrandResult({ status })).toBe(false);
    },
  );

  it("never offers Send result while Stop applies", () => {
    for (const status of Object.keys(STATUSES) as ErrandStatus[]) {
      const errand = { status, can_send_result: true };
      expect(canSendErrandResult(errand) && isLiveErrand(errand)).toBe(false);
    }
  });
});

describe("parseErrandStatus", () => {
  it.each(Object.keys(STATUSES))("accepts %s", (status) => {
    expect(parseErrandStatus(status)).toBe(status);
  });

  it.each([undefined, null, "", "RUNNING", "pending", 3, {}])("rejects %s", (value) => {
    expect(parseErrandStatus(value)).toBeNull();
  });
});
