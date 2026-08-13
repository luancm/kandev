import { describe, expect, it } from "vitest";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { deriveSessionGitValues, hasComparisonEvidence } from "./use-session-git-derived";

const ACTION_HEAD = "published-head";
const TRACKING_HEAD = "tracking-head";
const ROLE_IDENTITY = {
  repository: {
    provider: "github",
    host: "github.com",
    repository_path: "acme/widget",
  },
  ref: "feature",
};
const COMPARISON = {
  context_generation: "generation-1",
  target: {
    repository: { provider: "github", host: "github.com", repository_path: "acme/widget" },
    ref: "main",
  },
  resolution_state: "resolved" as const,
  resolved_ref: "origin/main",
  base_commit: "comparison-base",
  ahead: 2,
  behind: 1,
  additions: 3,
  deletions: 1,
};

function status(overrides: Partial<GitStatusEntry> = {}): GitStatusEntry {
  return {
    branch: "feature/contribution",
    remote_branch: "contributor/feature",
    modified: [],
    added: [],
    deleted: [],
    untracked: [],
    renamed: [],
    ahead: 0,
    behind: 0,
    files: {},
    timestamp: null,
    ...overrides,
  };
}

// eslint-disable-next-line max-lines-per-function -- role-specific cases stay together for the derived model.
describe("deriveSessionGitValues", () => {
  it("uses upstream-relative counts for remote actions", () => {
    const result = deriveSessionGitValues(
      status({
        ahead: 7,
        behind: 2,
        remote_ahead: 1,
        remote_behind: 3,
      }),
      false,
      [],
      [],
      [],
    );

    expect(result).toMatchObject({
      ahead: 7,
      behind: 2,
      remoteAhead: 1,
      remoteBehind: 3,
      pushAhead: 1,
      pullBehind: 3,
      canPush: true,
      canPull: true,
    });
  });

  it("falls back to base-ahead for a branch without an upstream", () => {
    const result = deriveSessionGitValues(
      status({
        remote_branch: null,
        ahead: 4,
        remote_ahead: 8,
        remote_behind: 2,
      }),
      false,
      [],
      [],
      [],
    );

    expect(result).toMatchObject({
      remoteAhead: 8,
      remoteBehind: 2,
      pushAhead: 4,
      pullBehind: 0,
      canPush: true,
      canPull: false,
    });
  });

  it("uses the writable action head for Push and explicit tracking for Pull", () => {
    const result = deriveSessionGitValues(
      status({
        remote_branch: "upstream/main",
        ahead: 99,
        behind: 4,
        remote_ahead: 0,
        remote_behind: 8,
        remote_roles_generation: "generation-1",
        action_head: {
          identity: ROLE_IDENTITY,
          observation_state: "present",
          remote_head_commit: ACTION_HEAD,
          ahead: 2,
          behind: 0,
        },
        tracking_upstream: {
          identity: ROLE_IDENTITY,
          observation_state: "present",
          remote_head_commit: TRACKING_HEAD,
          ahead: 0,
          behind: 3,
        },
      }),
      false,
      [],
      [],
      [],
    );

    expect(result).toMatchObject({
      pushAhead: 2,
      pullBehind: 3,
      canPush: true,
      canPull: true,
      actionHeadCommit: ACTION_HEAD,
      trackingUpstreamCommit: TRACKING_HEAD,
      actionEvidenceAvailable: true,
      trackingEvidenceAvailable: true,
    });
  });

  it("fails closed when the action or tracking observation is unknown", () => {
    const result = deriveSessionGitValues(
      status({
        ahead: 7,
        remote_ahead: 4,
        remote_behind: 6,
        remote_roles_generation: "generation-1",
        action_head: { observation_state: "unknown" },
        tracking_upstream: { observation_state: "unknown" },
      }),
      false,
      [],
      [],
      [],
    );

    expect(result).toMatchObject({
      pushAhead: 0,
      pullBehind: 0,
      canPush: false,
      canPull: false,
      actionEvidenceAvailable: false,
      trackingEvidenceAvailable: false,
    });
  });

  it("fails closed when structured observations have no generation", () => {
    const result = deriveSessionGitValues(
      status({
        remote_roles_generation: undefined,
        action_head: {
          observation_state: "present",
          remote_head_commit: ACTION_HEAD,
          ahead: 2,
        },
        tracking_upstream: {
          observation_state: "present",
          remote_head_commit: TRACKING_HEAD,
          behind: 3,
        },
      }),
      false,
      [],
      [],
      [],
    );

    expect(result).toMatchObject({
      pushAhead: 0,
      pullBehind: 0,
      canPush: false,
      canPull: false,
      actionEvidenceAvailable: false,
      trackingEvidenceAvailable: false,
    });
  });

  it("rejects fabricated counts on an absent remote role", () => {
    const result = deriveSessionGitValues(
      status({
        remote_roles_generation: "generation-1",
        action_head: { identity: ROLE_IDENTITY, observation_state: "absent", ahead: 0 },
        tracking_upstream: { identity: ROLE_IDENTITY, observation_state: "absent" },
      }),
      false,
      [],
      [],
      [],
    );

    expect(result.actionEvidenceAvailable).toBe(false);
    expect(result.canPush).toBe(false);
  });

  it("allows a first push only when action absence is explicit", () => {
    const result = deriveSessionGitValues(
      status({
        remote_branch: "upstream/main",
        ahead: 3,
        remote_ahead: 50,
        remote_behind: 50,
        remote_roles_generation: "generation-1",
        comparison: COMPARISON,
        action_head: { identity: ROLE_IDENTITY, observation_state: "absent" },
        tracking_upstream: {
          identity: ROLE_IDENTITY,
          observation_state: "present",
          remote_head_commit: TRACKING_HEAD,
          ahead: 0,
          behind: 0,
        },
      }),
      false,
      [],
      [],
      [],
    );

    expect(result).toMatchObject({
      pushAhead: 3,
      pullBehind: 0,
      canPush: true,
      canPull: false,
      actionEvidenceAvailable: true,
      trackingEvidenceAvailable: true,
    });
  });

  it("requires complete comparison identity and counts", () => {
    expect(hasComparisonEvidence(status({ comparison: COMPARISON }))).toBe(true);
    expect(
      hasComparisonEvidence(
        status({ comparison: { ...COMPARISON, target: undefined, additions: undefined } }),
      ),
    ).toBe(false);
    expect(
      hasComparisonEvidence(
        status({ action_head: { identity: ROLE_IDENTITY, observation_state: "absent" } }),
      ),
    ).toBe(false);
  });

  it("requires source and comparison evidence before creating a PR", () => {
    const actionHead = {
      identity: ROLE_IDENTITY,
      observation_state: "present" as const,
      remote_head_commit: ACTION_HEAD,
      ahead: 0,
      behind: 0,
    };
    const complete = deriveSessionGitValues(
      status({
        remote_roles_generation: "generation-1",
        action_head: actionHead,
        comparison: COMPARISON,
      }),
      false,
      [],
      [],
      [{} as never],
    );
    expect(complete.canCreatePR).toBe(true);

    const missingSource = deriveSessionGitValues(
      status({
        remote_roles_generation: "generation-1",
        action_head: { ...actionHead, identity: undefined },
        comparison: COMPARISON,
      }),
      false,
      [],
      [],
      [{} as never],
    );
    const missingComparison = deriveSessionGitValues(
      status({
        remote_roles_generation: "generation-1",
        action_head: actionHead,
        comparison: { ...COMPARISON, deletions: undefined },
      }),
      false,
      [],
      [],
      [{} as never],
    );
    expect(missingSource.canCreatePR).toBe(false);
    expect(missingComparison.canCreatePR).toBe(false);
  });

  it("fails closed when a generation omits both role observations", () => {
    const result = deriveSessionGitValues(
      status({
        remote_roles_generation: "generation-2",
        remote_ahead: 9,
        remote_behind: 9,
      }),
      false,
      [],
      [],
      [],
    );

    expect(result).toMatchObject({
      pushAhead: 0,
      pullBehind: 0,
      canPush: false,
      canPull: false,
      actionEvidenceAvailable: false,
      trackingEvidenceAvailable: false,
    });
  });

  it("treats an empty generation as structured unknown state", () => {
    const result = deriveSessionGitValues(
      status({ remote_roles_generation: "" }),
      false,
      [],
      [],
      [],
    );

    expect(result).toMatchObject({
      actionEvidenceAvailable: false,
      trackingEvidenceAvailable: false,
      comparisonEvidenceAvailable: false,
      canPush: false,
      canPull: false,
    });
  });
});
