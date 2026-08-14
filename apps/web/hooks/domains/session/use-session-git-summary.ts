import { useMemo } from "react";
import type {
  FileInfo,
  GitComparisonStatus,
  GitRemoteRefObservation,
} from "@/lib/state/slices/session-runtime/types";
import { useSessionGitStatusByRepo } from "./use-session-git-status";
import { deriveRemoteRoleCapabilities, hasComparisonEvidence } from "./use-session-git-derived";

type RepositoryStatus = ReturnType<typeof useSessionGitStatusByRepo>[number];

export type RepositoryStatusSummary = {
  repository_name: string;
  branch: string | null;
  ahead: number;
  behind: number;
  remoteAhead: number;
  remoteBehind: number;
  pushAhead: number;
  pullBehind: number;
  hasUpstream: boolean;
  actionHeadState: string | null;
  trackingUpstreamState: string | null;
  actionHeadCommit: string | null;
  trackingUpstreamCommit: string | null;
  actionHead: GitRemoteRefObservation | null | undefined;
  trackingUpstream: GitRemoteRefObservation | null | undefined;
  comparison: GitComparisonStatus | null | undefined;
  actionEvidenceAvailable: boolean;
  trackingEvidenceAvailable: boolean;
  comparisonEvidenceAvailable: boolean;
  hasStaged: boolean;
  hasUnstaged: boolean;
};

function upstreamCounts(status: RepositoryStatus["status"], ahead: number) {
  const roles = deriveRemoteRoleCapabilities(status, ahead);
  return {
    hasUpstream: roles.hasUpstream,
    remoteAhead: status?.remote_ahead ?? 0,
    remoteBehind: status?.remote_behind ?? 0,
    pushAhead: roles.pushAhead,
    pullBehind: roles.pullBehind,
    actionHeadState: roles.actionHeadState,
    trackingUpstreamState: roles.trackingUpstreamState,
    actionHeadCommit: roles.actionHeadCommit,
    trackingUpstreamCommit: roles.trackingUpstreamCommit,
    actionHead: status?.action_head,
    trackingUpstream: status?.tracking_upstream,
    comparison: status?.comparison,
    actionEvidenceAvailable: roles.actionEvidenceAvailable,
    trackingEvidenceAvailable: roles.trackingEvidenceAvailable,
    comparisonEvidenceAvailable: hasComparisonEvidence(status),
  };
}

function summarizeRepositoryStatus(
  { repository_name, status }: RepositoryStatus,
  stagedByRepo: Map<string, boolean>,
  unstagedByRepo: Map<string, boolean>,
): RepositoryStatusSummary {
  const ahead = status?.ahead ?? 0;
  const behind = status?.behind ?? 0;
  const remote = upstreamCounts(status, ahead);
  return {
    repository_name,
    branch: status?.branch ?? null,
    ahead,
    behind,
    ...remote,
    hasStaged: stagedByRepo.get(repository_name) ?? false,
    hasUnstaged: unstagedByRepo.get(repository_name) ?? false,
  };
}

export function deriveMultiRepoSummary(
  statusByRepo: RepositoryStatus[],
  allFiles: FileInfo[],
  reposInFiles: string[],
) {
  const seen = new Set<string>();
  for (const { repository_name } of statusByRepo) seen.add(repository_name);
  for (const repositoryName of reposInFiles) seen.add(repositoryName);
  const all = Array.from(seen).sort((a, b) => a.localeCompare(b));
  const named = all.filter((repositoryName) => repositoryName !== "");
  const hasRootStatus = statusByRepo.some((entry) => entry.repository_name === "");
  const repoNamesForControls = hasRootStatus || named.length === 0 ? all : named;

  if (statusByRepo.length === 0) {
    return { repoNamesForControls, perRepoStatus: [] as RepositoryStatusSummary[] };
  }

  const stagedByRepo = new Map<string, boolean>();
  const unstagedByRepo = new Map<string, boolean>();
  for (const file of allFiles) {
    const repositoryName = file.repository_name ?? "";
    if (file.staged) stagedByRepo.set(repositoryName, true);
    else unstagedByRepo.set(repositoryName, true);
  }
  const hasNamed = statusByRepo.some((status) => status.repository_name !== "");
  const filtered =
    !hasRootStatus && hasNamed
      ? statusByRepo.filter((status) => status.repository_name !== "")
      : statusByRepo;
  const perRepoStatus = filtered.map((entry) =>
    summarizeRepositoryStatus(entry, stagedByRepo, unstagedByRepo),
  );
  return { repoNamesForControls, perRepoStatus };
}

/**
 * Derives the repository names and status summaries used by Changes-panel
 * repository controls. A bare multi-repo root has no real empty scope, while
 * a Git root with submodules does and must retain it alongside its children.
 */
export function useMultiRepoSummary(
  statusByRepo: ReturnType<typeof useSessionGitStatusByRepo>,
  allFiles: FileInfo[],
  reposInFiles: string[],
) {
  return useMemo(
    () => deriveMultiRepoSummary(statusByRepo, allFiles, reposInFiles),
    [statusByRepo, allFiles, reposInFiles],
  );
}
