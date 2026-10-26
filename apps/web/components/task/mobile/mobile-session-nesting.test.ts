import { describe, expect, it } from "vitest";
import { nestSessionRows } from "./mobile-session-nesting";

function row(id: string, parentSessionId: string | null = null, nested = false) {
  return { id, nested, parentSessionId };
}

describe("nestSessionRows", () => {
  it("returns top-level rows unchanged when nothing is nested", () => {
    const rows = [row("a"), row("b")];
    expect(nestSessionRows(rows)).toEqual([
      { row: rows[0], children: [] },
      { row: rows[1], children: [] },
    ]);
  });

  it("groups nested rows under their parent in the given order", () => {
    const rows = [row("a"), row("e1", "a", true), row("b"), row("e2", "a", true)];
    const groups = nestSessionRows(rows);
    expect(groups.map((group) => group.row.id)).toEqual(["a", "b"]);
    expect(groups[0].children.map((child) => child.id)).toEqual(["e1", "e2"]);
    expect(groups[1].children).toEqual([]);
  });

  it("keeps a nested row without a listed parent at the top level", () => {
    const groups = nestSessionRows([row("a"), row("e1", "gone", true), row("e2", null, true)]);
    expect(groups.map((group) => group.row.id)).toEqual(["a", "e1", "e2"]);
  });

  it("does not nest a row that is not marked nested", () => {
    const groups = nestSessionRows([row("a"), row("b", "a", false)]);
    expect(groups.map((group) => group.row.id)).toEqual(["a", "b"]);
  });
});
