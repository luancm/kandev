"use client";

import { useCallback, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { PRCommitInfo, TaskPR } from "@/lib/types/github";
import type { GitRemoteRefObservation } from "@/lib/state/slices/session-runtime/types";
import { usePRCommits } from "@/hooks/domains/github/use-pr-commits";
import { usePRReviewRepositoryIdentityResolver } from "@/hooks/domains/github/use-pr-review-repository-identity";
import { useReviewPRSelection } from "@/hooks/domains/github/use-review-pr-selection";
import { useSessionGitStatusByRepo } from "./use-session-git-status";
import { hasComparisonEvidence } from "./use-session-git-derived";
import { resolveBranchScopedTaskPRs, selectBranchScopedTaskPR } from "./branch-scoped-task-pr";
import {
  classifyRemoteContribution,
  type RemoteContributionRelation,
} from "./remote-contribution-relation";

function taskPRHeadIdentity(pr: TaskPR | null): GitRemoteRefObservation["identity"] {
  if (!pr?.head_branch) return undefined;
  const hasCrossRepositoryFields =
    pr.head_host !== undefined ||
    pr.head_owner !== undefined ||
    pr.head_repo !== undefined ||
    pr.head_repo_id !== undefined ||
    pr.head_repo_node_id !== undefined;
  if (hasCrossRepositoryFields && (!pr.head_host || !pr.head_owner || !pr.head_repo)) {
    return undefined;
  }
  const host = hasCrossRepositoryFields ? pr.head_host : (pr.base_host ?? "github.com");
  const owner = hasCrossRepositoryFields ? pr.head_owner : (pr.base_owner ?? pr.owner);
  const repo = hasCrossRepositoryFields ? pr.head_repo : (pr.base_repo ?? pr.repo);
  if (!host || !owner || !repo) return undefined;
  return {
    repository: {
      provider: "github",
      host,
      repository_path: `${owner}/${repo}`,
      provider_repository_id:
        pr.head_repo_id !== undefined ? String(pr.head_repo_id) : pr.head_repo_node_id,
    },
    ref: pr.head_branch,
  };
}

export type RemoteContributionRelationState = {
  prs: TaskPR[];
  selectedPR: TaskPR | null;
  repositoryName: string | undefined;
  commits: PRCommitInfo[];
  loading: boolean;
  error: string | null;
  relation: RemoteContributionRelation;
  refreshProviderEvidence: () => Promise<string | null>;
};

export function useRemoteContributionRelation(
  sessionId: string | null | undefined,
): RemoteContributionRelationState {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const { prs, selectedKey } = useReviewPRSelection(activeTaskId);
  const statusByRepo = useSessionGitStatusByRepo(sessionId ?? null);
  const resolveRepositoryName = usePRReviewRepositoryIdentityResolver(activeTaskId, sessionId);
  const branchScopedPRs = useMemo(
    () =>
      resolveBranchScopedTaskPRs({
        prs,
        statuses: statusByRepo,
        preferredKey: selectedKey,
        resolveRepositoryName,
      }),
    [prs, statusByRepo, selectedKey, resolveRepositoryName],
  );
  const selection = useMemo(
    () => selectBranchScopedTaskPR(branchScopedPRs, selectedKey),
    [branchScopedPRs, selectedKey],
  );
  const scopedPRs = useMemo(() => branchScopedPRs.map((entry) => entry.pr), [branchScopedPRs]);
  const selectedPR = selection?.pr ?? null;
  const commitsState = usePRCommits(
    selectedPR?.owner ?? null,
    selectedPR?.repo ?? null,
    selectedPR?.pr_number ?? null,
    selectedPR?.last_synced_at ?? null,
  );
  const repositoryName = selection?.repositoryName;
  const refreshProviderEvidence = useCallback(async () => {
    const refreshed = await commitsState.refresh();
    return refreshed?.providerHead ?? null;
  }, [commitsState.refresh]);
  const gitStatus = selection?.gitStatus;

  const relation = useMemo(
    () =>
      classifyRemoteContribution({
        hasSelectedPR: Boolean(selectedPR),
        providerCommits: commitsState.commits,
        providerHead: commitsState.providerHead,
        providerCommitsComplete: commitsState.providerCommitsComplete,
        providerLoading: commitsState.loading,
        providerError: commitsState.error,
        localHead: gitStatus?.head_commit,
        upstreamHead: gitStatus?.remote_head_commit,
        remoteAhead: gitStatus?.remote_ahead ?? 0,
        remoteBehind: gitStatus?.remote_behind ?? 0,
        baseAhead: gitStatus?.ahead ?? 0,
        hasUpstream: Boolean(gitStatus?.remote_branch),
        actionHead: gitStatus?.action_head,
        trackingUpstream: gitStatus?.tracking_upstream,
        providerSourceIdentity: taskPRHeadIdentity(selectedPR),
        remoteRolesGeneration: gitStatus?.remote_roles_generation,
        comparisonEvidenceAvailable: hasComparisonEvidence(gitStatus),
      }),
    [
      selectedPR,
      commitsState.commits,
      commitsState.providerHead,
      commitsState.providerCommitsComplete,
      commitsState.loading,
      commitsState.error,
      gitStatus,
    ],
  );

  return {
    prs: scopedPRs,
    selectedPR,
    repositoryName,
    commits: commitsState.commits,
    loading: commitsState.loading,
    error: commitsState.error,
    relation,
    refreshProviderEvidence,
  };
}
