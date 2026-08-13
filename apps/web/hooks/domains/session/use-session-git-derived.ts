import type {
  FileInfo,
  GitRemoteRefObservation,
  GitStatusEntry,
  SessionCommit,
} from "@/lib/state/slices/session-runtime/types";

type RemoteObservation = GitRemoteRefObservation | null | undefined;
type RemoteIdentity = NonNullable<GitRemoteRefObservation["identity"]>;
type RemoteRepositoryIdentity = RemoteIdentity["repository"];

export type RemoteRoleCapabilities = {
  actionHeadState: string | null;
  trackingUpstreamState: string | null;
  actionHeadCommit: string | null;
  trackingUpstreamCommit: string | null;
  actionEvidenceAvailable: boolean;
  trackingEvidenceAvailable: boolean;
  actionAhead: number;
  actionBehind: number;
  trackingAhead: number;
  trackingBehind: number;
  pushAhead: number;
  pullBehind: number;
  hasUpstream: boolean;
};

function nonNegativeKnown(value: number | undefined): boolean {
  return value !== undefined && Number.isFinite(value) && value >= 0;
}

function hasCompleteRepositoryIdentity(repository: RemoteRepositoryIdentity | undefined): boolean {
  return Boolean(
    repository?.host && (repository.repository_path || repository.provider_repository_id),
  );
}

function hasCompleteRemoteIdentity(identity: GitRemoteRefObservation["identity"]): boolean {
  return Boolean(identity?.ref && hasCompleteRepositoryIdentity(identity.repository));
}

function hasCompleteComparisonIdentity(
  comparison: NonNullable<GitStatusEntry["comparison"]>,
): boolean {
  const target = comparison?.target;
  return Boolean(
    target?.ref &&
    target.repository?.host &&
    (target.repository.repository_path || target.repository.provider_repository_id),
  );
}

/** Comparison is a separate delivered target. Legacy statuses have no
 * structured comparison and remain compatible until the next refresh. */
export function hasComparisonEvidence(
  gitStatus:
    | Pick<
        GitStatusEntry,
        "comparison" | "remote_roles_generation" | "action_head" | "tracking_upstream"
      >
    | undefined,
): boolean {
  if (!gitStatus) return true;
  if (gitStatus.comparison === undefined) {
    return (
      gitStatus.remote_roles_generation === undefined &&
      gitStatus.action_head === undefined &&
      gitStatus.tracking_upstream === undefined
    );
  }
  const comparison = gitStatus.comparison;
  return Boolean(
    comparison &&
    comparison.resolution_state === "resolved" &&
    comparison.context_generation &&
    hasCompleteComparisonIdentity(comparison) &&
    comparison.resolved_ref &&
    comparison.base_commit &&
    nonNegativeKnown(comparison.ahead) &&
    nonNegativeKnown(comparison.behind) &&
    nonNegativeKnown(comparison.additions) &&
    nonNegativeKnown(comparison.deletions),
  );
}

function observationState(observation: RemoteObservation): string | null {
  if (observation === undefined) return null;
  return observation?.observation_state ?? "unknown";
}

function isKnownObservation(observation: RemoteObservation): boolean {
  const state = observationState(observation);
  return state === "present" || state === "absent";
}

function hasPresentHead(observation: RemoteObservation): boolean {
  return (
    observationState(observation) === "present" &&
    hasCompleteRemoteIdentity(observation?.identity) &&
    Boolean(observation?.remote_head_commit)
  );
}

function sameRemoteIdentity(
  left: GitRemoteRefObservation["identity"],
  right: GitRemoteRefObservation["identity"],
): boolean {
  if (!left || !right) return false;
  const leftRepository = left.repository;
  const rightRepository = right.repository;
  const leftHost = leftRepository.host;
  const rightHost = rightRepository.host;
  if (
    !hasCompleteRepositoryIdentity(leftRepository) ||
    !hasCompleteRepositoryIdentity(rightRepository)
  ) {
    return false;
  }
  const provider = leftRepository.provider ?? "";
  const pathMatches =
    leftRepository.repository_path && rightRepository.repository_path
      ? ["github", "gitlab", "azure_repos"].includes(provider)
        ? leftRepository.repository_path.toLowerCase() ===
          rightRepository.repository_path.toLowerCase()
        : leftRepository.repository_path === rightRepository.repository_path
      : false;
  const idMatches =
    !leftRepository.provider_repository_id || !rightRepository.provider_repository_id
      ? true
      : leftRepository.provider_repository_id === rightRepository.provider_repository_id;
  const repositoryMatches =
    pathMatches ||
    ((!leftRepository.repository_path || !rightRepository.repository_path) &&
      Boolean(
        leftRepository.provider_repository_id &&
        rightRepository.provider_repository_id &&
        leftRepository.provider_repository_id === rightRepository.provider_repository_id,
      ));
  return (
    left.ref === right.ref &&
    provider === (rightRepository.provider ?? "") &&
    leftHost?.toLowerCase() === rightHost?.toLowerCase() &&
    idMatches &&
    repositoryMatches
  );
}

function nonNegative(value: number | undefined): number {
  return value !== undefined && Number.isFinite(value) ? Math.max(0, value) : 0;
}

type RoleStatus = Pick<
  GitStatusEntry,
  | "remote_roles_generation"
  | "remote_branch"
  | "remote_head_commit"
  | "remote_ahead"
  | "remote_behind"
  | "action_head"
  | "tracking_upstream"
  | "comparison"
>;

function legacyRoleCapabilities(
  status: RoleStatus,
  comparisonAhead: number,
): RemoteRoleCapabilities {
  const hasUpstream = Boolean(status.remote_branch);
  const remoteHead = status.remote_head_commit ?? null;
  const remoteAhead = nonNegative(status.remote_ahead);
  const remoteBehind = nonNegative(status.remote_behind);
  const firstPushAhead = nonNegative(comparisonAhead);
  return {
    actionHeadState: hasUpstream ? "present" : "absent",
    trackingUpstreamState: hasUpstream ? "present" : "absent",
    actionHeadCommit: remoteHead,
    trackingUpstreamCommit: remoteHead,
    actionEvidenceAvailable: true,
    trackingEvidenceAvailable: true,
    actionAhead: hasUpstream ? remoteAhead : firstPushAhead,
    actionBehind: hasUpstream ? remoteBehind : 0,
    trackingAhead: hasUpstream ? remoteAhead : 0,
    trackingBehind: hasUpstream ? remoteBehind : 0,
    pushAhead: hasUpstream ? remoteAhead : firstPushAhead,
    pullBehind: hasUpstream ? remoteBehind : 0,
    hasUpstream,
  };
}

function actionAheadForStructuredRoles(
  action: RemoteObservation,
  tracking: RemoteObservation,
  actionState: string,
  trackingState: string,
  comparisonAhead: number,
  comparisonEvidenceAvailable: boolean,
): number {
  if (actionState === "absent") {
    return comparisonEvidenceAvailable ? nonNegative(comparisonAhead) : 0;
  }
  if (actionState !== "present") return 0;
  if (action?.ahead !== undefined) return nonNegative(action.ahead);
  const sameIdentity =
    hasPresentHead(action) &&
    hasPresentHead(tracking) &&
    sameRemoteIdentity(action?.identity, tracking?.identity);
  return sameIdentity && trackingState === "present" ? nonNegative(tracking?.ahead) : 0;
}

function roleEvidenceAvailable(state: string, observation: RemoteObservation, hasHead: boolean) {
  if (!hasCompleteRemoteIdentity(observation?.identity)) return false;
  if (state === "absent") {
    return observation?.ahead === undefined && observation?.behind === undefined;
  }
  return (
    isKnownObservation(observation) &&
    hasHead &&
    nonNegativeKnown(observation?.ahead) &&
    nonNegativeKnown(observation?.behind)
  );
}

function structuredRoleEvidence(
  action: RemoteObservation,
  tracking: RemoteObservation,
  actionState: string,
  trackingState: string,
  generationAvailable: boolean,
) {
  const actionPresent = hasPresentHead(action);
  const trackingPresent = hasPresentHead(tracking);
  return {
    actionPresent,
    trackingPresent,
    actionEvidenceAvailable:
      generationAvailable && roleEvidenceAvailable(actionState, action, actionPresent),
    trackingEvidenceAvailable:
      generationAvailable && roleEvidenceAvailable(trackingState, tracking, trackingPresent),
  };
}

function structuredRoleCounts({
  action,
  tracking,
  actionState,
  trackingState,
  comparisonAhead,
  comparisonEvidenceAvailable,
  actionEvidenceAvailable,
  trackingEvidenceAvailable,
  trackingPresent,
}: {
  action: RemoteObservation;
  tracking: RemoteObservation;
  actionState: string;
  trackingState: string;
  comparisonAhead: number;
  comparisonEvidenceAvailable: boolean;
  actionEvidenceAvailable: boolean;
  trackingEvidenceAvailable: boolean;
  trackingPresent: boolean;
}) {
  const actionAhead = actionAheadForStructuredRoles(
    action,
    tracking,
    actionState,
    trackingState,
    comparisonAhead,
    comparisonEvidenceAvailable,
  );
  const trackingBehind = nonNegative(tracking?.behind);
  return {
    actionAhead,
    actionBehind: nonNegative(action?.behind),
    trackingAhead: nonNegative(tracking?.ahead),
    trackingBehind,
    pushAhead: actionEvidenceAvailable ? actionAhead : 0,
    pullBehind: trackingEvidenceAvailable && trackingPresent ? trackingBehind : 0,
    hasUpstream: trackingEvidenceAvailable && trackingPresent,
  };
}

function structuredRoleCapabilities(
  action: RemoteObservation,
  tracking: RemoteObservation,
  comparisonAhead: number,
  generationAvailable: boolean,
  comparisonEvidenceAvailable: boolean,
): RemoteRoleCapabilities {
  const actionState = observationState(action) ?? "unknown";
  const trackingState = observationState(tracking) ?? "unknown";
  const { trackingPresent, actionEvidenceAvailable, trackingEvidenceAvailable } =
    structuredRoleEvidence(action, tracking, actionState, trackingState, generationAvailable);
  const counts = structuredRoleCounts({
    action,
    tracking,
    actionState,
    trackingState,
    comparisonAhead,
    comparisonEvidenceAvailable,
    actionEvidenceAvailable,
    trackingEvidenceAvailable,
    trackingPresent,
  });
  return {
    actionHeadState: actionState,
    trackingUpstreamState: trackingState,
    actionHeadCommit: action?.remote_head_commit ?? null,
    trackingUpstreamCommit: tracking?.remote_head_commit ?? null,
    actionEvidenceAvailable,
    trackingEvidenceAvailable,
    ...counts,
  };
}

/**
 * Derives the independent remote-role capabilities used by desktop and mobile
 * controls. Legacy statuses remain readable during rollout, but once either
 * structured observation is present, counts never fall back across roles.
 */
export function deriveRemoteRoleCapabilities(
  gitStatus: RoleStatus | undefined,
  comparisonAhead: number,
  comparisonEvidenceAvailableOverride?: boolean,
): RemoteRoleCapabilities {
  const action = gitStatus?.action_head;
  const tracking = gitStatus?.tracking_upstream;
  const hasStructuredRoles =
    action !== undefined ||
    tracking !== undefined ||
    gitStatus?.remote_roles_generation !== undefined ||
    gitStatus?.comparison !== undefined;
  if (!gitStatus || !hasStructuredRoles) {
    return legacyRoleCapabilities(gitStatus ?? { remote_branch: null }, comparisonAhead);
  }
  return structuredRoleCapabilities(
    action,
    tracking,
    comparisonAhead,
    Boolean(gitStatus.remote_roles_generation),
    comparisonEvidenceAvailableOverride ?? hasComparisonEvidence(gitStatus),
  );
}

function deriveBranchValues(gitStatus: GitStatusEntry | undefined) {
  const ahead = gitStatus?.ahead ?? 0;
  return {
    branch: gitStatus?.branch ?? null,
    remoteBranch: gitStatus?.remote_branch ?? null,
    headCommit: gitStatus?.head_commit ?? null,
    remoteHeadCommit: gitStatus?.remote_head_commit ?? null,
    ahead,
    behind: gitStatus?.behind ?? 0,
  };
}

function deriveUpstreamValues(gitStatus: GitStatusEntry | undefined, ahead: number) {
  const roles = deriveRemoteRoleCapabilities(gitStatus, ahead);
  return {
    remoteAhead: gitStatus?.remote_ahead ?? 0,
    remoteBehind: gitStatus?.remote_behind ?? 0,
    ...roles,
  };
}

function deriveChangeValues(
  unstagedFiles: FileInfo[],
  stagedFiles: FileInfo[],
  commits: SessionCommit[],
) {
  const hasUnstaged = unstagedFiles.length > 0;
  const hasStaged = stagedFiles.length > 0;
  const hasCommits = commits.length > 0;
  return { hasUnstaged, hasStaged, hasCommits, hasChanges: hasUnstaged || hasStaged };
}

export function deriveSessionGitValues(
  gitStatus: GitStatusEntry | undefined,
  hasRepositoryStatuses: boolean,
  unstagedFiles: FileInfo[],
  stagedFiles: FileInfo[],
  commits: SessionCommit[],
) {
  const branch = deriveBranchValues(gitStatus);
  const upstream = deriveUpstreamValues(gitStatus, branch.ahead);
  const status = { ...branch, ...upstream };
  const changes = deriveChangeValues(unstagedFiles, stagedFiles, commits);
  const hasAnything = changes.hasChanges || changes.hasCommits;
  return {
    ...status,
    comparisonEvidenceAvailable: hasComparisonEvidence(gitStatus),
    statusLoaded: Boolean(gitStatus || hasRepositoryStatuses),
    ...changes,
    hasAnything,
    canStageAll: changes.hasUnstaged,
    canCommit: changes.hasStaged,
    canPush: status.pushAhead > 0,
    canPull: status.pullBehind > 0,
    canCreatePR:
      changes.hasCommits &&
      Boolean(gitStatus) &&
      status.actionEvidenceAvailable &&
      hasComparisonEvidence(gitStatus),
  };
}
