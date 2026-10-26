import type { BackendMessage } from "./backend-message";

/** `role` value of an errand session; ordinary sessions omit `role`. */
export type TaskSessionRole = "errand";

export type ErrandStatus =
  | "queued"
  | "running"
  | "needs_attention"
  | "done"
  | "failed"
  | "cancelled";

/** Delivery state of an errand's current result to its parent session. */
export type ErrandResultState = "none" | "pending" | "delivered" | "superseded" | "discarded";

/** Browser projection of one errand, mirroring the backend `ErrandDTO`. */
export type Errand = {
  /** Equals the errand's session ID; a queued errand has no session row yet. */
  errand_id: string;
  task_id: string;
  parent_session_id: string;
  title: string;
  objective: string;
  agent_profile_id: string;
  status: ErrandStatus;
  result_state: ErrandResultState;
  /** At most the first 280 characters of the current result; full text stays with the parent. */
  result_preview: string;
  result_truncated: boolean;
  created_at: string;
  updated_at: string;
  settled_at: string | null;
  /**
   * Server-computed eligibility for **Send result to parent**: the errand is
   * done or failed and its latest settled turn is not yet a result.
   */
  can_send_result?: boolean;
};

/** Response of the `errand.list` request. */
export type ErrandListResponse = { errands: Errand[] };

/** Response of the `errand.cancel_all` request. */
export type ErrandCancelAllResponse = { cancelled: number };

export type ErrandBackendMessageMap = {
  "errand.updated": BackendMessage<"errand.updated", Errand>;
};

const LIVE_ERRAND_STATUSES: ReadonlySet<ErrandStatus> = new Set([
  "queued",
  "running",
  "needs_attention",
]);

const ERRAND_STATUSES: ReadonlySet<string> = new Set<ErrandStatus>([
  ...LIVE_ERRAND_STATUSES,
  "done",
  "failed",
  "cancelled",
]);

/** A live errand is queued, running, or needs attention. */
export function isLiveErrand(errand: Pick<Errand, "status">): boolean {
  return LIVE_ERRAND_STATUSES.has(errand.status);
}

/**
 * Whether **Send result to parent** applies. It never applies to a live
 * errand, so it never appears together with **Stop**.
 */
export function canSendErrandResult(errand: Pick<Errand, "status" | "can_send_result">): boolean {
  return errand.can_send_result === true && !isLiveErrand(errand);
}

/** Narrows an untrusted wire value to an errand status. */
export function parseErrandStatus(value: unknown): ErrandStatus | null {
  return typeof value === "string" && ERRAND_STATUSES.has(value) ? (value as ErrandStatus) : null;
}
