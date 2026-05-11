import { describe, it, expect } from "vitest";
import {
  availableTabs,
  pickInitialTab,
  STORAGE_KEY,
  type TabId,
} from "./mobile-changes-tabs-helpers";

const empty = { uncommitted: 0, committed: 0, pr: 0 };

describe("availableTabs", () => {
  it("returns [] when nothing has content", () => {
    expect(availableTabs(empty, false)).toEqual([]);
  });

  it("returns only sources with content", () => {
    expect(availableTabs({ uncommitted: 2, committed: 0, pr: 0 }, false)).toEqual(["uncommitted"]);
    expect(availableTabs({ uncommitted: 2, committed: 1, pr: 0 }, false)).toEqual([
      "uncommitted",
      "committed",
    ]);
  });

  it("always includes pr when hasPR is true, even with zero loaded files", () => {
    expect(availableTabs({ uncommitted: 1, committed: 0, pr: 0 }, true)).toEqual([
      "uncommitted",
      "pr",
    ]);
  });

  it("excludes pr when hasPR is false even if pr count is non-zero (shouldn't happen, but be safe)", () => {
    expect(availableTabs({ uncommitted: 1, committed: 0, pr: 5 }, false)).toEqual(["uncommitted"]);
  });

  it("orders uncommitted, committed, pr", () => {
    expect(availableTabs({ uncommitted: 1, committed: 1, pr: 1 }, true)).toEqual([
      "uncommitted",
      "committed",
      "pr",
    ]);
  });
});

describe("pickInitialTab", () => {
  it("defaults to uncommitted when no saved value and source is available", () => {
    expect(pickInitialTab(null, { uncommitted: 1, committed: 0, pr: 0 }, false)).toBe(
      "uncommitted",
    );
  });

  it("returns saved value when valid", () => {
    expect(pickInitialTab("pr", { uncommitted: 0, committed: 0, pr: 1 }, true)).toBe("pr");
    expect(pickInitialTab("committed", { uncommitted: 0, committed: 1, pr: 0 }, false)).toBe(
      "committed",
    );
  });

  it("falls back to uncommitted when saved value not in available tabs", () => {
    expect(pickInitialTab("pr", { uncommitted: 1, committed: 0, pr: 0 }, false)).toBe(
      "uncommitted",
    );
  });

  it("falls back to first available when uncommitted not available", () => {
    expect(pickInitialTab(null, { uncommitted: 0, committed: 1, pr: 0 }, false)).toBe("committed");
    expect(pickInitialTab(null, { uncommitted: 0, committed: 0, pr: 1 }, true)).toBe("pr");
  });

  it("returns null when no tabs available", () => {
    expect(pickInitialTab(null, empty, false)).toBeNull();
  });

  it("ignores garbage saved values", () => {
    expect(pickInitialTab("garbage" as TabId, { uncommitted: 1, committed: 0, pr: 0 }, false)).toBe(
      "uncommitted",
    );
  });
});

describe("STORAGE_KEY", () => {
  it("is a stable global key", () => {
    expect(STORAGE_KEY).toBe("mobile-changes-source");
  });
});
