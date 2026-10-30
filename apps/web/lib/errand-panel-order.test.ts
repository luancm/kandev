import { describe, expect, it } from "vitest";
import type { Errand } from "./types/errands";
import { orderErrandPanel } from "./errand-panel-order";

const item = (id: string, parent: string, status: Errand["status"], created: string) =>
  ({ errand_id: id, parent_session_id: parent, status, created_at: created }) as Errand;

describe("orderErrandPanel", () => {
  it("places live groups first and sorts rows and groups by creation time", () => {
    const rows = [
      item("old", "a", "running", "2024-01-01"),
      item("done", "b", "done", "2026-01-01"),
      item("new", "a", "queued", "2025-01-01"),
      item("attention", "b", "needs_attention", "2025-02-01"),
    ];
    const result = orderErrandPanel(rows);
    expect(result.active.map((g) => g.parentId)).toEqual(["b", "a"]);
    expect(result.active[1].errands.map((e) => e.errand_id)).toEqual(["new", "old"]);
    expect(result.finished[0].errands[0].errand_id).toBe("done");
    expect(rows[0].errand_id).toBe("old");
  });
  it("moves a settled errand to finished without sorting by updates", () => {
    const first = item("first", "a", "running", "2025-01-01");
    const second = item("second", "a", "running", "2025-02-01");
    expect(orderErrandPanel([first, second]).active[0].errands.map((e) => e.errand_id)).toEqual([
      "second",
      "first",
    ]);
    expect(
      orderErrandPanel([{ ...first, status: "done" }, second]).finished[0].errands[0].errand_id,
    ).toBe("first");
  });
  it("uses stable ID ordering for ties and invalid timestamps", () => {
    const result = orderErrandPanel([
      item("b", "a", "done", "bad"),
      item("a", "a", "failed", "bad"),
    ]);
    expect(result.finished[0].errands.map((e) => e.errand_id)).toEqual(["a", "b"]);
  });
});
