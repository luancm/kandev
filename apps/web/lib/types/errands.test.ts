import { describe, expect, it } from "vitest";
import {
  canSendErrandResult,
  isLiveErrand,
  normalizeErrandAttention,
  normalizeErrandBrowserAttention,
  parseErrandAttentionReason,
  parseErrandStatus,
  type Errand,
  type ErrandBrowserRow,
  type ErrandDeliveryState,
  type ErrandStatus,
} from "./errands";

const BASE_DATE = "2026-10-26T10:00:00Z";
const RECOVERY_UNRESOLVED = "recovery_unresolved";
const TASK_ID = "task-1";
const PARENT_SESSION_ID = "session-parent";
const ERRAND_ID = "errand-1";

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
      errand_id: ERRAND_ID,
      task_id: TASK_ID,
      parent_session_id: PARENT_SESSION_ID,
      title: "API audit",
      objective: "Audit the API",
      agent_profile_id: "profile-1",
      status: "running",
      result_state: "none",
      result_preview: "",
      result_truncated: false,
      created_at: BASE_DATE,
      updated_at: BASE_DATE,
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
      errand_id: ERRAND_ID,
      task_id: TASK_ID,
      parent_session_id: PARENT_SESSION_ID,
      title: "API audit",
      objective: "Audit the API",
      agent_profile_id: "profile-1",
      status: "done",
      result_state: "delivered",
      result_preview: "",
      result_truncated: false,
      created_at: BASE_DATE,
      updated_at: "2026-10-26T10:05:00Z",
      settled_at: "2026-10-26T10:05:00Z",
      can_send_result: true,
    };

    expect(errand.can_send_result).toBe(true);
  });

  it.each(["received", "uncertain"] as const)("accepts the current delivery state %s", (state) => {
    const deliveryState: ErrandDeliveryState = state;
    const errand: Pick<Errand, "delivery_state"> = { delivery_state: deliveryState };

    expect(errand.delivery_state).toBe(state);
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

describe("Errand attention reason", () => {
  it.each(["permission", RECOVERY_UNRESOLVED] as const)("accepts %s", (reason) => {
    expect(parseErrandAttentionReason(reason)).toBe(reason);
  });

  it.each([undefined, null, "unknown", "uncertain", 3, {}])("rejects %s", (reason) => {
    expect(parseErrandAttentionReason(reason)).toBeNull();
  });

  it("keeps only a recognized reason on needs_attention snapshots", () => {
    const attention = normalizeErrandAttention({
      errand_id: ERRAND_ID,
      task_id: TASK_ID,
      parent_session_id: PARENT_SESSION_ID,
      title: "API audit",
      objective: "Audit the API",
      agent_profile_id: "profile-1",
      status: "needs_attention",
      attention_reason: RECOVERY_UNRESOLVED,
      result_state: "none",
      result_preview: "",
      result_truncated: false,
      created_at: BASE_DATE,
      updated_at: BASE_DATE,
      settled_at: null,
    });
    expect(attention.attention_reason).toBe(RECOVERY_UNRESOLVED);

    const oldBackend = normalizeErrandAttention({ ...attention, attention_reason: undefined });
    expect(oldBackend).not.toHaveProperty("attention_reason");

    const invalid = normalizeErrandAttention({
      ...attention,
      attention_reason: "unknown",
    } as unknown as Errand);
    expect(invalid).not.toHaveProperty("attention_reason");

    const nonAttention = normalizeErrandAttention({ ...attention, status: "running" });
    expect(nonAttention).not.toHaveProperty("attention_reason");
  });
});

describe("compact Errand browser rows", () => {
  const row: ErrandBrowserRow = {
    errand_id: ERRAND_ID,
    task_id: TASK_ID,
    parent_session_id: PARENT_SESSION_ID,
    title: "API audit",
    agent_profile_id: "profile-1",
    status: "needs_attention",
    result_state: "pending",
    result_truncated: false,
    created_at: BASE_DATE,
    updated_at: BASE_DATE,
    settled_at: null,
    version: 2,
    live: true,
    outstanding: true,
    unread: false,
    can_send_result: false,
  };

  it("preserves a recognized reason and never exposes full result fields", () => {
    const normalized = normalizeErrandBrowserAttention({ ...row, attention_reason: "permission" });
    expect(normalized.attention_reason).toBe("permission");
    expect(normalized).not.toHaveProperty("objective");
    expect(normalized).not.toHaveProperty("result_preview");
  });

  it("drops invalid and stale attention reasons", () => {
    expect(
      normalizeErrandBrowserAttention({
        ...row,
        attention_reason: "future",
      } as unknown as ErrandBrowserRow),
    ).not.toHaveProperty("attention_reason");
    expect(
      normalizeErrandBrowserAttention({
        ...row,
        status: "running",
        attention_reason: "permission",
      }),
    ).not.toHaveProperty("attention_reason");
  });
});
