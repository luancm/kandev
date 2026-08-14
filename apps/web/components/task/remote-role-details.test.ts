import { describe, expect, it } from "vitest";
import { buildRemoteRoleRows, formatRemoteRepository } from "./remote-role-details-model";

describe("remote role details", () => {
  it("projects the selected repository and ref for each independent role", () => {
    const rows = buildRemoteRoleRows({
      actionHead: {
        observation_state: "present",
        identity: {
          repository: {
            provider: "github",
            host: "github.com",
            repository_path: "acme/fork",
          },
          ref: "feature/task",
        },
      },
      trackingUpstream: {
        observation_state: "absent",
        identity: {
          repository: {
            provider: "github",
            host: "github.com",
            repository_path: "acme/project",
          },
          ref: "main",
        },
      },
      comparison: {
        resolution_state: "resolved",
        target: {
          repository: {
            provider: "github",
            host: "github.com",
            repository_path: "acme/project",
          },
          ref: "main",
        },
      },
    });

    expect(rows).toEqual([
      {
        role: "action_head",
        state: "present",
        repository: "github.com/acme/fork",
        ref: "feature/task",
      },
      {
        role: "tracking_upstream",
        state: "absent",
        repository: "github.com/acme/project",
        ref: "main",
      },
      {
        role: "comparison_target",
        state: "resolved",
        repository: "github.com/acme/project",
        ref: "main",
      },
    ]);
  });

  it("falls back to the provider repository id without exposing credentials", () => {
    expect(
      formatRemoteRepository({
        provider: "azure_repos",
        host: "dev.azure.com",
        provider_repository_id: "repo-42",
      }),
    ).toBe("dev.azure.com · repo-42");
  });

  it("keeps unresolved roles visible without inventing an identity", () => {
    expect(
      buildRemoteRoleRows({
        actionHead: null,
        trackingUpstream: undefined,
        comparison: { resolution_state: "ambiguous" },
      }),
    ).toEqual([
      { role: "action_head", state: "unknown", repository: null, ref: null },
      { role: "tracking_upstream", state: "unknown", repository: null, ref: null },
      { role: "comparison_target", state: "ambiguous", repository: null, ref: null },
    ]);
  });
});
