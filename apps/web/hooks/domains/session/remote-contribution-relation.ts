import type {
  GitRemoteRefObservation,
  GitStatusEntry,
} from "@/lib/state/slices/session-runtime/types";
import { deriveRemoteRoleCapabilities } from "./use-session-git-derived";

export type RemoteContributionRelationKind =
  | "not_applicable"
  | "aligned"
  | "local_ahead"
  | "provider_ahead"
  | "diverged"
  | "unknown";

export type RemoteContributionPresentation = "unified" | "separate";

export type RemoteContributionAction =
  | "normal_push"
  | "provider_ahead_pull"
  | "diverged_replace"
  | "unavailable_evidence";

export type RemoteContributionRelationInput = {
  hasSelectedPR: boolean;
  providerCommits: ReadonlyArray<{ sha: string }>;
  providerHead: string | null | undefined;
  providerCommitsComplete: boolean;
  providerLoading: boolean;
  providerError: string | null;
  localHead: string | null | undefined;
  upstreamHead: string | null | undefined;
  remoteAhead: number;
  remoteBehind: number;
  baseAhead: number;
  hasUpstream: boolean;
  actionHead?: GitRemoteRefObservation | null;
  trackingUpstream?: GitRemoteRefObservation | null;
  remoteRolesGeneration?: string;
  comparisonEvidenceAvailable?: boolean;
};

export type RemoteContributionRelation = {
  kind: RemoteContributionRelationKind;
  presentation: RemoteContributionPresentation;
  action: RemoteContributionAction;
  providerHead: string | null;
  pushAhead: number;
  pullBehind: number;
  canPush: boolean;
  canPull: boolean;
  canReplaceRemote: boolean;
  canUseRemote: boolean;
  actionHeadState?: string | null;
  trackingUpstreamState?: string | null;
  actionEvidenceAvailable?: boolean;
  trackingEvidenceAvailable?: boolean;
  comparisonEvidenceAvailable?: boolean;
};

export type RemoteContributionActionPolicy = {
  action: RemoteContributionAction;
  pushDisabled: boolean;
  pullDisabled: boolean;
  replaceDisabled: boolean;
  useDisabled: boolean;
  disabledReason: "provider_evidence_unavailable" | null;
};

export type RemoteContributionActionReasonKey =
  | "task:providerUnavailable"
  | "task:providerAheadPushDisabled"
  | "task:providerAheadPullRequiresUpstream"
  | "task:divergedActionsUnavailable";

export function remoteContributionActionReasonKey(
  relation: RemoteContributionRelation,
  action: "push" | "pull",
): RemoteContributionActionReasonKey | null {
  if (relation.action === "unavailable_evidence") return "task:providerUnavailable";
  if (relation.action === "provider_ahead_pull") {
    if (action === "push") return "task:providerAheadPushDisabled";
    if (!relation.canPull) return "task:providerAheadPullRequiresUpstream";
  }
  if (relation.action === "diverged_replace") return "task:divergedActionsUnavailable";
  return null;
}

/**
 * Safety gates shared by desktop and mobile Git controls. A provider-ahead
 * checkout may pull, but must not push over the provider's newer history.
 * Diverged histories expose explicit contribution replacement or adoption.
 */
export function remoteContributionActionPolicy(
  relation: RemoteContributionRelation,
): RemoteContributionActionPolicy {
  const unavailable = relation.action === "unavailable_evidence";
  const diverged = relation.action === "diverged_replace";
  const actionEvidenceUnavailable = relation.actionEvidenceAvailable === false;
  const trackingEvidenceUnavailable =
    relation.trackingEvidenceAvailable === false || relation.trackingUpstreamState === "absent";
  return {
    action: relation.action,
    pushDisabled:
      unavailable ||
      diverged ||
      relation.action === "provider_ahead_pull" ||
      actionEvidenceUnavailable ||
      !relation.canPush,
    pullDisabled:
      unavailable ||
      diverged ||
      trackingEvidenceUnavailable ||
      (relation.action === "provider_ahead_pull" && !relation.canPull),
    replaceDisabled: !relation.canReplaceRemote,
    useDisabled: !relation.canUseRemote,
    disabledReason: unavailable ? "provider_evidence_unavailable" : null,
  };
}

export function isFullCommitSHA(value: string | null | undefined): boolean {
  return (
    (value?.length === 40 || value?.length === 64) &&
    [...(value ?? "")].every((character) => /^[0-9a-f]$/i.test(character))
  );
}

function roleCapabilities(input: RemoteContributionRelationInput) {
  const status: Pick<
    GitStatusEntry,
    | "remote_branch"
    | "remote_head_commit"
    | "remote_ahead"
    | "remote_behind"
    | "remote_roles_generation"
    | "action_head"
    | "tracking_upstream"
  > = {
    remote_branch: input.hasUpstream ? "legacy/upstream" : null,
    remote_head_commit: input.upstreamHead ?? undefined,
    remote_ahead: input.remoteAhead,
    remote_behind: input.remoteBehind,
    remote_roles_generation: input.remoteRolesGeneration,
    action_head: input.actionHead,
    tracking_upstream: input.trackingUpstream,
  };
  return deriveRemoteRoleCapabilities(status, input.baseAhead);
}

function fallbackCapabilities(input: RemoteContributionRelationInput) {
  const roles = roleCapabilities(input);
  return {
    pushAhead: roles.pushAhead,
    pullBehind: roles.pullBehind,
    canPush: roles.actionEvidenceAvailable && roles.pushAhead > 0,
    canPull: roles.trackingEvidenceAvailable && roles.pullBehind > 0,
    canReplaceRemote: false,
    canUseRemote: false,
    actionHeadState: roles.actionHeadState,
    trackingUpstreamState: roles.trackingUpstreamState,
    actionEvidenceAvailable: roles.actionEvidenceAvailable,
    trackingEvidenceAvailable: roles.trackingEvidenceAvailable,
    comparisonEvidenceAvailable: input.comparisonEvidenceAvailable !== false,
  };
}

function actionFor(
  kind: RemoteContributionRelationKind,
  providerHead: string | null,
): RemoteContributionAction {
  if (kind === "provider_ahead") return "provider_ahead_pull";
  if (kind === "diverged" && isFullCommitSHA(providerHead)) return "diverged_replace";
  if (kind === "unknown") return "unavailable_evidence";
  if (kind === "diverged") return "unavailable_evidence";
  return "normal_push";
}

function result(
  kind: RemoteContributionRelationKind,
  providerHead: string | null,
  capabilities: ReturnType<typeof fallbackCapabilities>,
): RemoteContributionRelation {
  const action = !capabilities.actionEvidenceAvailable
    ? "unavailable_evidence"
    : actionFor(kind, providerHead);
  return {
    kind,
    presentation: kind === "diverged" ? "separate" : "unified",
    action,
    providerHead,
    ...capabilities,
  };
}

function providerEvidenceUnavailable(
  input: RemoteContributionRelationInput,
  providerHead: string | null,
  roles: ReturnType<typeof roleCapabilities>,
): boolean {
  return (
    input.providerLoading ||
    Boolean(input.providerError) ||
    !input.providerCommitsComplete ||
    input.providerCommits.length === 0 ||
    !providerHead ||
    !input.localHead ||
    !roles.actionEvidenceAvailable ||
    (roles.actionHeadState === "present" && !roles.actionHeadCommit)
  );
}

function classifyProviderHeads(
  input: RemoteContributionRelationInput,
  providerHead: string,
  roles: ReturnType<typeof roleCapabilities>,
  fallback: ReturnType<typeof fallbackCapabilities>,
): RemoteContributionRelation {
  if (input.localHead === providerHead) {
    return result("aligned", providerHead, fallback);
  }

  if (input.providerCommits.some((commit) => commit.sha === input.localHead)) {
    return result("provider_ahead", providerHead, {
      ...fallback,
      canPush: false,
      canPull: fallback.canPull,
    });
  }

  const actionHead = roles.actionHeadCommit;
  if (!actionHead) return result("unknown", providerHead, fallback);

  if (actionHead === providerHead && roles.actionAhead > 0 && roles.actionBehind === 0) {
    return result("local_ahead", providerHead, {
      ...fallback,
      pushAhead: roles.actionAhead,
      canPush: true,
      canPull: fallback.canPull,
    });
  }

  const canResolve = isFullCommitSHA(providerHead);
  return result("diverged", providerHead, {
    ...fallback,
    canPush: false,
    canPull: false,
    canReplaceRemote: canResolve,
    canUseRemote: canResolve,
  });
}

export function classifyRemoteContribution(
  input: RemoteContributionRelationInput,
): RemoteContributionRelation {
  const fallback = fallbackCapabilities(input);
  if (!input.hasSelectedPR) {
    return result("not_applicable", null, fallback);
  }

  // The provider API supplies the head separately because a commit page can
  // be incomplete or ordered independently from the live branch head.
  const providerHead = input.providerHead ?? null;
  const roles = roleCapabilities(input);
  if (providerEvidenceUnavailable(input, providerHead, roles)) {
    return result("unknown", providerHead, fallback);
  }
  return classifyProviderHeads(input, providerHead!, roles, fallback);
}
