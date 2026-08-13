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
  providerSourceIdentity?: GitRemoteRefObservation["identity"];
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
  providerEvidenceAvailable?: boolean;
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
  if (relation.action === "unavailable_evidence" && relation.providerEvidenceAvailable === false) {
    return "task:providerUnavailable";
  }
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
  const providerUnavailable = relation.providerEvidenceAvailable === false;
  const diverged = relation.action === "diverged_replace";
  const actionEvidenceUnavailable = relation.actionEvidenceAvailable !== true;
  const trackingEvidenceUnavailable =
    relation.trackingEvidenceAvailable !== true || relation.trackingUpstreamState !== "present";
  return {
    action: relation.action,
    pushDisabled:
      providerUnavailable ||
      diverged ||
      relation.action === "provider_ahead_pull" ||
      actionEvidenceUnavailable ||
      !relation.canPush,
    pullDisabled:
      providerUnavailable ||
      diverged ||
      trackingEvidenceUnavailable ||
      (relation.action === "provider_ahead_pull" && !relation.canPull),
    replaceDisabled: !relation.canReplaceRemote,
    useDisabled: !relation.canUseRemote,
    disabledReason: providerUnavailable ? "provider_evidence_unavailable" : null,
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
  return deriveRemoteRoleCapabilities(status, input.baseAhead, input.comparisonEvidenceAvailable);
}

function sameRemoteIdentity(
  left: GitRemoteRefObservation["identity"],
  right: GitRemoteRefObservation["identity"],
): boolean {
  if (!left || !right || !left.ref || left.ref !== right.ref) return false;
  const leftRepository = left.repository;
  const rightRepository = right.repository;
  if (
    !leftRepository.host ||
    !rightRepository.host ||
    (leftRepository.provider ?? "") !== (rightRepository.provider ?? "") ||
    leftRepository.host.toLowerCase() !== rightRepository.host.toLowerCase()
  ) {
    return false;
  }
  if (
    leftRepository.provider_repository_id &&
    rightRepository.provider_repository_id &&
    leftRepository.provider_repository_id !== rightRepository.provider_repository_id
  ) {
    return false;
  }
  if (leftRepository.repository_path && rightRepository.repository_path) {
    const provider = leftRepository.provider ?? "";
    return ["github", "gitlab", "azure_repos"].includes(provider)
      ? leftRepository.repository_path.toLowerCase() ===
          rightRepository.repository_path.toLowerCase()
      : leftRepository.repository_path === rightRepository.repository_path;
  }
  return Boolean(
    leftRepository.provider_repository_id &&
    rightRepository.provider_repository_id &&
    leftRepository.provider_repository_id === rightRepository.provider_repository_id,
  );
}

function provesProviderAheadAncestry(
  input: RemoteContributionRelationInput,
  providerHead: string,
): boolean {
  if (!input.providerCommitsComplete || !input.localHead) return false;
  const localIndex = input.providerCommits.findIndex((commit) => commit.sha === input.localHead);
  const providerIndex = input.providerCommits.findIndex((commit) => commit.sha === providerHead);
  return localIndex >= 0 && providerIndex > localIndex;
}

function canPullProviderAhead(
  input: RemoteContributionRelationInput,
  providerHead: string,
): boolean {
  const tracking = input.trackingUpstream;
  return Boolean(
    Boolean(input.remoteRolesGeneration) &&
    provesProviderAheadAncestry(input, providerHead) &&
    input.providerSourceIdentity &&
    tracking?.observation_state === "present" &&
    tracking.remote_head_commit === providerHead &&
    tracking.ahead !== undefined &&
    Number.isFinite(tracking.ahead) &&
    tracking.ahead >= 0 &&
    tracking.behind !== undefined &&
    Number.isFinite(tracking.behind) &&
    tracking.behind > 0 &&
    sameRemoteIdentity(tracking.identity, input.providerSourceIdentity),
  );
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
    comparisonEvidenceAvailable:
      input.comparisonEvidenceAvailable ?? input.remoteRolesGeneration === undefined,
    providerEvidenceAvailable: true,
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
  providerEvidenceAvailable = true,
): RemoteContributionRelation {
  const action = actionFor(kind, providerHead);
  return {
    kind,
    presentation: kind === "diverged" ? "separate" : "unified",
    action,
    providerHead,
    ...capabilities,
    providerEvidenceAvailable,
  };
}

function providerEvidenceUnavailable(
  input: RemoteContributionRelationInput,
  providerHead: string | null,
): boolean {
  return (
    input.providerLoading ||
    Boolean(input.providerError) ||
    !input.providerCommitsComplete ||
    input.providerCommits.length === 0 ||
    !providerHead ||
    !input.localHead
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

  if (provesProviderAheadAncestry(input, providerHead)) {
    return result("provider_ahead", providerHead, {
      ...fallback,
      canPush: false,
      canPull: canPullProviderAhead(input, providerHead),
    });
  }

  const actionHead = roles.actionHeadCommit;
  if (!actionHead) return result("unknown", providerHead, fallback);

  const actionMatchesProvider =
    !input.providerSourceIdentity ||
    sameRemoteIdentity(input.actionHead?.identity, input.providerSourceIdentity);
  if (
    actionMatchesProvider &&
    actionHead === providerHead &&
    roles.actionAhead > 0 &&
    roles.actionBehind === 0
  ) {
    return result("local_ahead", providerHead, {
      ...fallback,
      pushAhead: roles.actionAhead,
      canPush: fallback.canPush,
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
  if (providerEvidenceUnavailable(input, providerHead)) {
    return result("unknown", providerHead, fallback, false);
  }
  return classifyProviderHeads(input, providerHead!, roles, fallback);
}
