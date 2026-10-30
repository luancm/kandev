import { describe, expect, it } from "vitest";
import {
  ERRAND_PREVIEW_MAX_CHARS,
  parseAwaitErrands,
  parseCancelErrand,
  parseErrandDelivery,
  parseStartErrand,
} from "./errand-parse";

const ERRAND_ID = "errand-1";

describe("parseStartErrand", () => {
  it("reads the start arguments and result", () => {
    const view = parseStartErrand(
      { title: "API audit", prompt: "Audit /api/v1", agent_profile: "Codex" },
      {
        errand_id: ERRAND_ID,
        session_id: ERRAND_ID,
        status: "running",
        agent_profile_id: "profile-1",
        agent_profile_name: "Codex",
      },
      "complete",
    );

    expect(view).toEqual({
      title: "API audit",
      prompt: "Audit /api/v1",
      errandId: ERRAND_ID,
      status: "running",
      profileId: "profile-1",
      profileName: "Codex",
      errorText: null,
    });
  });

  it("tolerates a pending call with arguments only", () => {
    const view = parseStartErrand({ title: "API audit", prompt: "Audit" }, null, "running");

    expect(view.errandId).toBeNull();
    expect(view.status).toBeNull();
    expect(view.title).toBe("API audit");
    expect(view.errorText).toBeNull();
  });

  it("falls back to session_id and ignores malformed fields", () => {
    const view = parseStartErrand(
      { title: 42, prompt: ["x"] },
      { session_id: ERRAND_ID, status: "RUNNING", agent_profile_name: 7 },
      "complete",
    );

    expect(view).toMatchObject({
      title: null,
      prompt: null,
      errandId: ERRAND_ID,
      status: null,
      profileName: null,
    });
  });

  it("reports an error result as text, never as an errand", () => {
    const fromString = parseStartErrand({ title: "API audit" }, "errand_limit_reached", "error");
    const fromObject = parseStartErrand(
      { title: "API audit" },
      { error: "Profile not found", details: { reason: "errand_profile_not_found" } },
      "complete",
    );

    expect(fromString).toMatchObject({ errandId: null, errorText: "errand_limit_reached" });
    expect(fromObject).toMatchObject({ errandId: null, errorText: "Profile not found" });
  });

  it("keeps an error status without readable text as an error", () => {
    expect(parseStartErrand(undefined, undefined, "error").errorText).toBe("");
  });
});

describe("parseAwaitErrands", () => {
  it("reads actual duration and outcome without guessing legacy values", () => {
    const measured = parseAwaitErrands(
      {},
      { duration_ms: 12000, end_reason: "timeout", errands: [] },
      "complete",
    );
    expect(measured.durationMs).toBe(12000);
    expect(measured.endReason).toBe("timeout");
    const legacy = parseAwaitErrands(
      {},
      { duration_ms: -1, end_reason: "other", errands: [] },
      "complete",
    );
    expect(legacy.durationMs).toBeNull();
    expect(legacy.endReason).toBeNull();
    expect(parseAwaitErrands({}, { duration_ms: 0 }, "complete").durationMs).toBe(0);
  });
  it("orders rows by the requested IDs and merges result statuses", () => {
    const view = parseAwaitErrands(
      { errand_ids: ["e-1", "e-2"], mode: "any", max_wait_seconds: 30 },
      {
        errands: [
          { errand_id: "e-2", status: "pending" },
          { errand_id: "e-1", status: "done", result: "All good", truncated: false },
          { errand_id: "e-3", status: "failed", delivered_via: "turn_context" },
        ],
      },
      "complete",
    );

    expect(view.mode).toBe("any");
    expect(view.rows).toEqual([
      { errandId: "e-1", status: "done", pending: false, resultReceived: true },
      { errandId: "e-2", status: null, pending: true, resultReceived: false },
      { errandId: "e-3", status: "failed", pending: false, resultReceived: false },
    ]);
    expect(view.errorText).toBeNull();
  });

  it("lists requested errands while the await is still running", () => {
    const view = parseAwaitErrands({ errand_ids: ["e-1", 5, "", "e-1"] }, null, "running");

    expect(view.mode).toBe("all");
    expect(view.rows).toEqual([
      { errandId: "e-1", status: null, pending: false, resultReceived: false },
    ]);
  });

  it("reports an error result", () => {
    const view = parseAwaitErrands({ errand_ids: ["e-1"] }, "errand_not_found", "error");

    expect(view.errorText).toBe("errand_not_found");
    expect(view.rows.map((row) => row.errandId)).toEqual(["e-1"]);
  });
});

describe("parseCancelErrand", () => {
  it("reads the errand ID from arguments, then the result", () => {
    expect(parseCancelErrand({ errand_id: "e-1" }, { status: "cancelled" }, "complete")).toEqual({
      errandId: "e-1",
      status: "cancelled",
      errorText: null,
    });
    expect(parseCancelErrand(undefined, { errand_id: "e-2" }, "complete").errandId).toBe("e-2");
  });
});

describe("parseErrandDelivery", () => {
  it("returns one item per valid delivered result", () => {
    const items = parseErrandDelivery({
      errand_delivery: {
        token: "tok",
        route: "notification",
        items: [
          { errand_id: "e-1", title: "Schema review", status: "done", preview: "No breaks." },
          { errand_id: "e-2", title: "Test plan", status: "cancelled" },
        ],
      },
    });

    expect(items).toEqual([
      { errandId: "e-1", title: "Schema review", status: "done", preview: "No breaks." },
      { errandId: "e-2", title: "Test plan", status: "cancelled", preview: "" },
    ]);
  });

  it.each([
    undefined,
    null,
    {},
    { errand_delivery: "x" },
    { errand_delivery: { items: "x" } },
    { errand_delivery: { items: [] } },
  ])("returns nothing for %s", (metadata) => {
    expect(parseErrandDelivery(metadata)).toEqual([]);
  });

  it("drops items without an errand ID and caps untrusted previews", () => {
    const long = "x".repeat(ERRAND_PREVIEW_MAX_CHARS + 50);
    const items = parseErrandDelivery({
      errand_delivery: {
        items: [
          null,
          { title: "No id" },
          { errand_id: 9 },
          { errand_id: "e-1", title: 3, status: "weird", preview: long },
        ],
      },
    });

    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({ errandId: "e-1", title: "", status: null });
    expect(Array.from(items[0].preview)).toHaveLength(ERRAND_PREVIEW_MAX_CHARS);
  });

  it("cuts a capped preview at a character boundary", () => {
    const emoji = "\u{1F600}".repeat(ERRAND_PREVIEW_MAX_CHARS + 1);
    const [item] = parseErrandDelivery({
      errand_delivery: { items: [{ errand_id: "e-1", preview: emoji }] },
    });

    expect(item.preview).toBe("\u{1F600}".repeat(ERRAND_PREVIEW_MAX_CHARS));
  });
});
