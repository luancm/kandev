import { describe, it, expect } from "vitest";
import {
  blockedRemoteOperationResult,
  groupPathsByRepoName,
  hasComparisonForRepository,
  resolveRemoteOperationRepositoryScope,
  repositoryScopesForMutation,
} from "./use-session-git";

describe("groupPathsByRepoName", () => {
  it("splits paths by repository_name, preserving insertion order per bucket", () => {
    const lookup = new Map([
      ["src/app.tsx", "frontend"],
      ["src/api.ts", "frontend"],
      ["handlers/task.go", "backend"],
    ]);
    const out = groupPathsByRepoName(["src/app.tsx", "handlers/task.go", "src/api.ts"], lookup);
    expect(out.get("frontend")).toEqual(["src/app.tsx", "src/api.ts"]);
    expect(out.get("backend")).toEqual(["handlers/task.go"]);
    expect(out.size).toBe(2);
  });

  it("falls back to empty-string bucket for paths missing a repo", () => {
    const out = groupPathsByRepoName(["unknown.txt"], new Map());
    expect(out.get("")).toEqual(["unknown.txt"]);
  });

  it("preserves single-repo callers (everything in one bucket)", () => {
    const lookup = new Map([
      ["a.ts", "only"],
      ["b.ts", "only"],
    ]);
    const out = groupPathsByRepoName(["a.ts", "b.ts"], lookup);
    expect(out.size).toBe(1);
    expect(out.get("only")).toEqual(["a.ts", "b.ts"]);
  });
});

describe("repositoryScopesForMutation", () => {
  it("excludes a file-only root when only named scopes have trackers", () => {
    expect(
      repositoryScopesForMutation(
        [{ repository_name: undefined }, { repository_name: "vendor" }],
        ["vendor"],
      ),
    ).toEqual(["vendor"]);
  });

  it("keeps an available root and adds available ancestors", () => {
    expect(
      repositoryScopesForMutation(
        [{ repository_name: "vendor/inner" }],
        ["", "vendor", "vendor/inner"],
      ),
    ).toEqual(["vendor/inner", "vendor", ""]);
  });

  it("preserves the legacy root fallback without per-repo status", () => {
    expect(repositoryScopesForMutation([{ repository_name: undefined }], [])).toEqual([""]);
  });
});

describe("hasComparisonForRepository", () => {
  const statuses = [
    {
      repository_name: "frontend",
      comparisonEvidenceAvailable: true,
    },
    {
      repository_name: "backend",
      comparisonEvidenceAvailable: false,
    },
  ];

  it("keeps Rebase and Merge scoped to repositories with comparison evidence", () => {
    expect(hasComparisonForRepository(statuses, "frontend")).toBe(true);
    expect(hasComparisonForRepository(statuses, "backend")).toBe(false);
    expect(hasComparisonForRepository(statuses, "missing")).toBe(false);
    expect(hasComparisonForRepository(statuses, undefined)).toBe(true);
    expect(hasComparisonForRepository([], undefined, true)).toBe(true);
    expect(hasComparisonForRepository([], undefined, false)).toBe(false);
  });
});

describe("blockedRemoteOperationResult", () => {
  it("reports blocked comparison operations as failures instead of successful no-ops", () => {
    expect(blockedRemoteOperationResult("rebase")).toEqual({
      success: false,
      operation: "rebase",
      output: "",
      error_code: "remote_role_evidence_unavailable",
    });
    expect(blockedRemoteOperationResult("merge").success).toBe(false);
  });
});

describe("resolveRemoteOperationRepositoryScope", () => {
  it("keeps the sole named repository explicit when no root scope exists", () => {
    expect(resolveRemoteOperationRepositoryScope(undefined, ["frontend"])).toBe("frontend");
    expect(resolveRemoteOperationRepositoryScope("frontend", ["frontend"])).toBe("frontend");
  });

  it("keeps the legacy root scope unscoped", () => {
    expect(resolveRemoteOperationRepositoryScope(undefined, [""])).toBeUndefined();
  });
});
