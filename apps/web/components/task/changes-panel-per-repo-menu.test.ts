import { describe, expect, it } from "vitest";
import { repoActionAvailability } from "./changes-panel-per-repo-menu";

describe("repoActionAvailability", () => {
  it("fails closed when a repository status is missing", () => {
    expect(repoActionAvailability(undefined)).toEqual({
      trackingUnavailable: true,
      comparisonUnavailable: true,
    });
  });

  it("fails closed when role evidence is omitted", () => {
    expect(
      repoActionAvailability({
        repository_name: "frontend",
        branch: "feature",
        ahead: 0,
        behind: 0,
        hasStaged: false,
        hasUnstaged: false,
      }),
    ).toEqual({ trackingUnavailable: true, comparisonUnavailable: true });
  });
});
