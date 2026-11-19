import { beforeEach, expect, it, vi } from "vitest";

const request = vi.hoisted(() => vi.fn());
const client = vi.hoisted(() => ({ current: { request } as { request: typeof request } | null }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => client.current }));

import * as errandApi from "./errand-api";
import { getErrandBrowserSummary, getErrandHistoryPage, inspectErrand } from "./errand-api";

const TASK_ID = "task-1";
const PARENT_SESSION_ID = "parent-1";
const CHILD_SESSION_ID = "errand-1";
const BASE_DATE = "2026-10-26T10:00:00Z";

function makeBrowserRow(status: "needs_attention" | "done") {
  return {
    errand_id: CHILD_SESSION_ID,
    task_id: TASK_ID,
    parent_session_id: PARENT_SESSION_ID,
    title: "Audit",
    agent_profile_id: "profile-1",
    status,
    ...(status === "needs_attention" ? { attention_reason: "permission" } : {}),
    result_state: status === "done" ? "delivered" : "pending",
    result_truncated: false,
    created_at: BASE_DATE,
    updated_at: BASE_DATE,
    settled_at: null,
    version: status === "done" ? 1 : 2,
    live: status === "needs_attention",
    outstanding: status === "needs_attention",
    unread: status === "needs_attention",
    can_send_result: false,
  };
}

beforeEach(() => {
  request.mockReset();
  client.current = { request };
});

it("exposes additive reads while retaining the complete list API", () => {
  const api = errandApi as unknown as Record<string, unknown>;
  expect(api.getErrandBrowserSummary).toEqual(expect.any(Function));
  expect(api.listErrands).toEqual(expect.any(Function));
});

it("reads the compact summary through its additive action", async () => {
  request.mockResolvedValue({
    scope: { task_id: TASK_ID },
    errands: [makeBrowserRow("needs_attention")],
    counts: {
      live: 1,
      capacity: 5,
      queued: 0,
      outstanding: 1,
      finished: 0,
      pending_results: 1,
      unread: 1,
    },
    parent_counts: [],
  });

  const summary = await getErrandBrowserSummary(TASK_ID);

  expect(summary.errands[0].attention_reason).toBe("permission");
  expect(summary.counts.finished).toBe(0);
  expect(request).toHaveBeenCalledWith("errand.browser_summary", { task_id: TASK_ID });
  expect(request).not.toHaveBeenCalledWith("errand.list", expect.anything());
});

it("passes the shared read abort signal through without changing the request payload", async () => {
  const controller = new AbortController();
  request.mockResolvedValue({
    scope: { task_id: TASK_ID },
    errands: [],
    counts: {
      live: 0,
      capacity: 5,
      queued: 0,
      outstanding: 0,
      finished: 0,
      pending_results: 0,
      unread: 0,
    },
    parent_counts: [],
  });

  await getErrandBrowserSummary(TASK_ID, controller.signal);

  expect(request).toHaveBeenCalledWith(
    "errand.browser_summary",
    { task_id: TASK_ID },
    undefined,
    controller.signal,
  );
});

it("requests a history page only for the explicit parent and cursor", async () => {
  request.mockResolvedValue({
    scope: { task_id: TASK_ID, parent_session_id: PARENT_SESSION_ID },
    errands: [],
    total_finished: 51,
    has_more: true,
    next_cursor: "next",
  });

  await expect(
    getErrandHistoryPage(TASK_ID, PARENT_SESSION_ID, { limit: 25, cursor: "after" }),
  ).resolves.toMatchObject({ total_finished: 51, next_cursor: "next" });
  expect(request).toHaveBeenCalledWith("errand.history_page", {
    task_id: TASK_ID,
    parent_session_id: PARENT_SESSION_ID,
    limit: 25,
    cursor: "after",
  });
});

it("inspects one child without enumerating task history", async () => {
  request.mockResolvedValue({
    scope: { task_id: TASK_ID, child_session_id: CHILD_SESSION_ID },
    errand: makeBrowserRow("done"),
    owns_conversation: true,
    parent: { available: false },
  });

  await expect(inspectErrand(TASK_ID, CHILD_SESSION_ID)).resolves.toMatchObject({
    owns_conversation: true,
    parent: { available: false },
  });
  expect(request).toHaveBeenCalledWith("errand.inspect", {
    task_id: TASK_ID,
    child_session_id: CHILD_SESSION_ID,
  });
  expect(request).not.toHaveBeenCalledWith("errand.list", expect.anything());
});

it("propagates history failures without falling back to the complete list", async () => {
  request.mockRejectedValue(new Error("history unavailable"));

  await expect(getErrandHistoryPage(TASK_ID, PARENT_SESSION_ID)).rejects.toThrow(
    "history unavailable",
  );
  expect(request).toHaveBeenCalledTimes(1);
  expect(request).toHaveBeenCalledWith("errand.history_page", {
    task_id: TASK_ID,
    parent_session_id: PARENT_SESSION_ID,
  });
});
