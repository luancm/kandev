import { describe, it, expect } from "vitest";
import { prKey } from "./use-pr-key-to-tasks";

describe("prKey", () => {
  it("formats owner/repo#number", () => {
    expect(prKey("kdlbs", "kandev", 42)).toBe("kdlbs/kandev#42");
  });

  it("handles underscores and dashes in owner/repo", () => {
    expect(prKey("my-org", "my_repo", 1)).toBe("my-org/my_repo#1");
  });

  it("handles large PR numbers", () => {
    expect(prKey("foo", "bar", 99999)).toBe("foo/bar#99999");
  });

  it("handles zero PR number", () => {
    expect(prKey("o", "r", 0)).toBe("o/r#0");
  });

  it("handles single-character owner and repo", () => {
    expect(prKey("a", "b", 7)).toBe("a/b#7");
  });
});
